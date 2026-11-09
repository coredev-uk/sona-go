//go:build linux || (cgo && (darwin || windows))

package cdmhost

import (
	"bufio"
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/cdm"
	"github.com/coredev-uk/sona-go/internal/playback/equalizer"
)

// Options configures Start.
type Options struct {
	// CDMPath is the Widevine CDM library the child loads (the parent's
	// choice: internal/playback/cdm). Its directory stays readable in the
	// child's sandbox.
	CDMPath string
	// Clear starts a host that loads no CDM: it plays plain audio only
	// (song previews, library uploads: LoadFile), so it needs no CDM
	// downloaded. CDMPath is ignored. The sandbox is the same.
	Clear bool
	// Sink overrides the GStreamer sink (e.g. "fakesink" in tests).
	Sink string
	// Device is the output to play to: a PipeWire sink's node.name on
	// Linux, a Core Audio device UID on macOS; "" follows the system
	// default.
	Device string
	// Equalizer is applied from the first pipeline on.
	Equalizer equalizer.Curve
	// Logger receives the child's log records; nil discards them.
	Logger *slog.Logger
	// Exe is the executable to run as the child; default os.Executable().
	// It must call ChildMain first thing in main.
	Exe string
	// OnVideoFrame receives each decoded music video picture as it falls
	// due (VideoFrame), from the goroutine reading the child: it must not
	// block. Pictures that are not valid I420 of at most MaxVideoSize are
	// dropped before it.
	OnVideoFrame func(VideoFrame)
	// cdmOnly starts a child that loads only the CDM (envCDMOnly).
	cdmOnly bool
	// realtime replaces how the child's audio threads are raised (tests).
	realtime *rtEnv
}

// Host is the parent's handle on one child process. Its methods are safe for
// concurrent use; calls that block in the child (Feed) do not delay others.
type Host struct {
	log   *slog.Logger
	cmd   *exec.Cmd
	stdin io.WriteCloser
	info  Info
	// parent is the parent's part of the sandbox (sandbox_<os>.go).
	parent  parentSandbox
	onFrame func(VideoFrame)

	encMu sync.Mutex
	enc   *gob.Encoder

	mu      sync.Mutex
	nextID  uint64
	pending map[uint64]chan reply
	err     error

	evMu   sync.Mutex
	evq    []Event
	evWake chan struct{}
	events chan Event

	// closing is set by Close and Kill: the exit that follows is expected.
	closing atomic.Bool
	// Raw (non-JSON) stderr: the last lines, and a Go crash report (panic
	// or fatal signal, then the goroutine stacks) from its first line.
	// stderrDone closes when stderr reaches EOF.
	tailMu     sync.Mutex
	tail       []string
	report     []string
	stderrOnce sync.Once
	stderrDone chan struct{}

	// Realtime scheduling of the child's audio threads (realtime.go):
	// rtMu guards rt and the threads asked for; rtReq serialises requests.
	rtEnv   rtEnv
	rtMu    sync.Mutex
	rt      Realtime
	rtAsked map[int]bool
	rtReq   sync.Mutex

	done chan struct{}
}

// Bounds on the stderr kept for a crash log.
const (
	stderrTailLines   = 40
	stderrReportLines = 120
	stderrLineMax     = 512
	// stderrDrain is how long read waits for the rest of stderr after
	// stdout closes (a grandchild could hold it open).
	stderrDrain = 2 * time.Second
)

// childEnvDropped matches environment variables the child must not get:
// anything that names a credential, and the addresses of the session bus,
// the display server, the agents, the session manager and MyGo's automation
// socket (a probe build's child would replace the parent's socket), which
// would let code in the child act outside its sandbox (sandbox.go hides their
// sockets too).
var childEnvDropped = regexp.MustCompile(`(?i)TOKEN|SECRET|PASSW|CREDENTIAL|COOKIE|API_?KEY|PRIVATE_?KEY|AUTH|` +
	`^(DBUS_|SSH_|GPG_|GNOME_KEYRING|AT_SPI_|WAYLAND_|KRB5)|^(DISPLAY|XAUTHORITY|SESSION_MANAGER|MYGO_AUTOMATION)$`)

// childEnv is the parent environment minus credentials and session
// addresses (childEnvDropped), plus the child marker.
func childEnv(log *slog.Logger) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k == EnvChild || k == envLogLevel || k == envCDMDir || k == envCDMOnly || slices.Contains(sandboxEnv, k) || childEnvDropped.MatchString(k) {
			continue
		}
		env = append(env, kv)
	}
	level := slog.LevelInfo
	for _, l := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn} {
		if log.Enabled(context.Background(), l) {
			level = l
			break
		}
	}
	return append(env, EnvChild+"=1", envLogLevel+"="+level.String())
}

// Start spawns a child, loads the CDM (unless Clear) and builds the audio
// pipeline. The child exits when the Host is closed or the parent dies.
func Start(ctx context.Context, o Options) (*Host, error) {
	if o.Clear {
		o.CDMPath = ""
	}
	h, err := spawn(o)
	if err != nil {
		return nil, err
	}
	eq := o.Equalizer
	r, err := h.call(ctx, request{Op: opInit, CDMPath: o.CDMPath, Clear: o.Clear, Sink: o.Sink, Device: o.Device, EQ: &eq})
	if r.Info != nil {
		h.info = *r.Info
	}
	h.info.PID = h.cmd.Process.Pid
	h.info.Sandbox = h.withSandbox(h.info.Sandbox)
	if err != nil {
		h.Close()
		return nil, err
	}
	h.log.Debug("cdmhost: started", "pid", h.info.PID, "cdm", h.info.CDMVersion, "interface", h.info.CDMInterface,
		"sink", h.info.Sink, "decoder", h.info.Decoder)
	return h, nil
}

func spawn(o Options) (*Host, error) {
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	exe := o.Exe
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return nil, fmt.Errorf("cdmhost: %w", err)
		}
	}
	env := childEnv(log)
	if o.CDMPath != "" {
		env = append(env, envCDMDir+"="+filepath.Dir(o.CDMPath))
	}
	if o.cdmOnly {
		env = append(env, envCDMOnly+"=1")
	}
	p, parent, err := startSandboxed(exe, env, log)
	if err != nil {
		return nil, fmt.Errorf("cdmhost: start %s: %w", exe, err)
	}
	h := &Host{
		log:     log.With("component", "cdmhost", "pid", p.cmd.Process.Pid),
		cmd:     p.cmd,
		parent:  parent,
		stdin:   p.stdin,
		enc:     gob.NewEncoder(p.stdin),
		pending: map[uint64]chan reply{},
		evWake:  make(chan struct{}, 1),
		events:  make(chan Event, 16),
		done:    make(chan struct{}),
		rtEnv:   defaultRTEnv,
		rt:      realtimeInitial,
		rtAsked: map[int]bool{},
		onFrame: o.OnVideoFrame,
	}
	if o.realtime != nil {
		h.rtEnv = *o.realtime
	}
	go h.relayLogs(p.stderr)
	go h.pump()
	go h.read(p.stdout)
	return h, nil
}

// proc is a started child with its pipes.
type proc struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

// startChild starts argv with env and attr, wired to fresh pipes, in
// childDir.
func startChild(argv, env []string, attr *syscall.SysProcAttr) (proc, error) {
	p := proc{cmd: exec.Command(argv[0], argv[1:]...)}
	p.cmd.Env = env
	p.cmd.Dir = childDir()
	p.cmd.SysProcAttr = attr
	var err error
	if p.stdin, err = p.cmd.StdinPipe(); err != nil {
		return proc{}, err
	}
	if p.stdout, err = p.cmd.StdoutPipe(); err != nil {
		return proc{}, err
	}
	if p.stderr, err = p.cmd.StderrPipe(); err != nil {
		return proc{}, err
	}
	if err := p.cmd.Start(); err != nil {
		return proc{}, err
	}
	return p, nil
}

// withSandbox completes the child's report of its sandbox with the
// parent's part.
func (h *Host) withSandbox(sb Sandbox) Sandbox { return sb.withParent(h.parent) }

// Info describes the child.
func (h *Host) Info() Info {
	info := h.info
	info.Realtime = h.realtime()
	return info
}

// Events delivers child events in order until the child exits. Up to
// maxQueuedEvents are queued, so a slow receiver never stalls replies;
// past that, position samples are dropped, and any other event ends the
// child.
func (h *Host) Events() <-chan Event { return h.events }

// Done is closed when the child has exited; Err then says why.
func (h *Host) Done() <-chan struct{} { return h.done }

// Err returns the reason the child exited, or nil while it runs.
func (h *Host) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

// Bounds on what the child can make the parent hold. The largest
// legitimate child→parent frame is a licence request or renewal message
// of a few KiB (init segments and fragments only go to the child); events
// queue only while the receiver is busy.
const (
	maxChildFrame   = 16 << 20
	maxQueuedEvents = 1024
)

// errMisbehaved wraps the reasons the parent stops reading a child and
// kills it.
var errMisbehaved = errors.New("cdmhost: host process misbehaved")

// gobFrames passes a gob stream through, failing a message longer than max
// before gob allocates for it. Every gob message starts with its length as
// a gob unsigned integer: one byte below 0x80, else a byte holding the
// negated count of the big-endian bytes that follow.
type gobFrames struct {
	r    *bufio.Reader
	max  uint64
	left uint64 // bytes of the current message still to pass on
	hdr  []byte // the current message's length prefix, not yet passed on
}

func (g *gobFrames) Read(p []byte) (int, error) {
	if len(g.hdr) == 0 && g.left == 0 {
		b, err := g.r.ReadByte()
		if err != nil {
			return 0, err
		}
		g.hdr = append(g.hdr[:0], b)
		n := uint64(b)
		if b >= 0x80 {
			count := -int(int8(b))
			if count < 1 || count > 8 {
				return 0, fmt.Errorf("%w: frame length prefix %#x", errMisbehaved, b)
			}
			n = 0
			for range count {
				c, err := g.r.ReadByte()
				if err != nil {
					return 0, io.ErrUnexpectedEOF
				}
				g.hdr = append(g.hdr, c)
				n = n<<8 | uint64(c)
			}
		}
		if n > g.max {
			return 0, fmt.Errorf("%w: a %d-byte frame (limit %d)", errMisbehaved, n, g.max)
		}
		g.left = n
	}
	if len(g.hdr) > 0 {
		k := copy(p, g.hdr)
		g.hdr = g.hdr[k:]
		return k, nil
	}
	if uint64(len(p)) > g.left {
		p = p[:g.left]
	}
	k, err := g.r.Read(p)
	g.left -= uint64(k)
	return k, err
}

// queue adds ev for pump; false when the queue is full and ev is not a
// position sample, which can be dropped.
func (h *Host) queue(ev Event) bool {
	h.evMu.Lock()
	defer h.evMu.Unlock()
	if len(h.evq) >= maxQueuedEvents {
		return ev.Kind == EventPosition
	}
	h.evq = append(h.evq, ev)
	return true
}

func (h *Host) read(stdout io.Reader) {
	dec := gob.NewDecoder(&gobFrames{r: bufio.NewReaderSize(stdout, 64<<10), max: maxChildFrame})
	var readErr error
loop:
	for {
		var f frame
		if err := dec.Decode(&f); err != nil {
			readErr = err
			break
		}
		switch {
		case f.Reply != nil:
			h.mu.Lock()
			ch := h.pending[f.Reply.ID]
			delete(h.pending, f.Reply.ID)
			h.mu.Unlock()
			if ch != nil {
				ch <- *f.Reply
			}
		case f.Event != nil:
			if !h.queue(*f.Event) {
				readErr = fmt.Errorf("%w: more than %d events queued", errMisbehaved, maxQueuedEvents)
				break loop
			}
			select {
			case h.evWake <- struct{}{}:
			default:
			}
		case f.Threads != nil:
			h.audioThreads(f.Threads.TIDs)
		case f.Video != nil:
			if h.onFrame != nil && f.Video.valid() {
				h.onFrame(*f.Video)
			}
		}
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		// Nothing reads its output any more: it would block, or keep
		// running out of sync.
		_ = h.cmd.Process.Kill()
	}
	// Wait closes the pipes: let relayLogs read the child's last words
	// (a panic trace) first.
	select {
	case <-h.stderrClosed():
	case <-time.After(stderrDrain):
	}
	waitErr := h.cmd.Wait()
	if !h.closing.Load() {
		h.logCrash()
	}
	err := ErrExited
	switch {
	case errors.Is(readErr, errMisbehaved):
		err = fmt.Errorf("%w: %w", ErrExited, readErr)
	case waitErr != nil:
		err = fmt.Errorf("%w: %v", ErrExited, waitErr)
	case readErr != nil && !errors.Is(readErr, io.EOF):
		err = fmt.Errorf("%w: %v", ErrExited, readErr)
	}
	h.mu.Lock()
	h.err = err
	pending := h.pending
	h.pending = nil
	h.mu.Unlock()
	for _, ch := range pending {
		ch <- reply{Err: err.Error(), Code: "exited"}
	}
	close(h.done)
}

// pump moves queued events to the Events channel.
func (h *Host) pump() {
	defer close(h.events)
	for {
		h.evMu.Lock()
		q := h.evq
		h.evq = nil
		h.evMu.Unlock()
		for _, ev := range q {
			select {
			case h.events <- ev:
			case <-h.done:
				return
			}
		}
		select {
		case <-h.evWake:
		case <-h.done:
			// Deliver what arrived before exit, then stop.
			h.evMu.Lock()
			q := h.evq
			h.evq = nil
			h.evMu.Unlock()
			for _, ev := range q {
				select {
				case h.events <- ev:
				case <-time.After(time.Second):
					return
				}
			}
			return
		}
	}
}

// relayLogs re-logs the child's JSON records (and any raw stderr output from
// GStreamer or the CDM, at debug level) through the parent's logger.
func (h *Host) relayLogs(r io.Reader) {
	defer close(h.stderrClosed())
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		var rec map[string]any
		if json.Unmarshal(line, &rec) != nil {
			h.keepTail(string(line))
			h.log.Debug("cdmhost stderr", "line", string(line))
			continue
		}
		msg, _ := rec["msg"].(string)
		var level slog.Level
		if l, ok := rec["level"].(string); ok {
			_ = level.UnmarshalText([]byte(l))
		}
		attrs := make([]any, 0, 2*len(rec))
		for k, v := range rec {
			if k != "msg" && k != "level" && k != "time" {
				attrs = append(attrs, k, v)
			}
		}
		h.log.Log(context.Background(), level, msg, attrs...)
	}
}

func (h *Host) stderrClosed() chan struct{} {
	h.stderrOnce.Do(func() { h.stderrDone = make(chan struct{}) })
	return h.stderrDone
}

// crashStart matches the first line of a Go runtime crash report: a panic,
// a fatal error, or a fatal signal (also one raised inside the CDM).
var crashStart = regexp.MustCompile(`^(panic: |fatal error: |SIG[A-Z]+: |unexpected fault address )`)

func (h *Host) keepTail(line string) {
	if len(line) > stderrLineMax {
		line = line[:stderrLineMax] + "…"
	}
	h.tailMu.Lock()
	defer h.tailMu.Unlock()
	if len(h.report) == 0 && crashStart.MatchString(line) || len(h.report) > 0 && len(h.report) < stderrReportLines {
		h.report = append(h.report, line)
	}
	if len(h.tail) == stderrTailLines {
		h.tail = append(h.tail[:0], h.tail[1:]...)
	}
	h.tail = append(h.tail, line)
}

// logCrash logs an exit nobody asked for: the exit code or signal and the
// child's crash report (Go's panic or signal report with the stacks), or
// else its last raw stderr lines (CDM or GStreamer output). Called after
// Wait.
func (h *Host) logCrash() {
	ps := h.cmd.ProcessState
	if ps == nil {
		return
	}
	attrs := []any{"exit_code", ps.ExitCode()}
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		attrs = append(attrs, "signal", ws.Signal().String())
	}
	h.tailMu.Lock()
	out := h.report
	if len(out) == 0 {
		out = h.tail
	}
	stderr := strings.Join(out, "\n")
	h.tailMu.Unlock()
	h.log.Error("cdmhost: child exited unexpectedly", append(attrs, "stderr", stderr)...)
}

// call sends r and waits for its reply.
func (h *Host) call(ctx context.Context, r request) (reply, error) {
	ch := make(chan reply, 1)
	h.mu.Lock()
	if h.pending == nil {
		err := h.err
		h.mu.Unlock()
		return reply{}, err
	}
	h.nextID++
	r.ID = h.nextID
	h.pending[r.ID] = ch
	h.mu.Unlock()

	h.encMu.Lock()
	err := h.enc.Encode(r)
	h.encMu.Unlock()
	if err != nil {
		h.forget(r.ID)
		return reply{}, fmt.Errorf("%w: %v", ErrExited, err)
	}
	select {
	case rep := <-ch:
		return rep, replyErr(rep)
	case <-ctx.Done():
		h.forget(r.ID)
		return reply{}, ctx.Err()
	}
}

func (h *Host) forget(id uint64) {
	h.mu.Lock()
	if h.pending != nil {
		delete(h.pending, id)
	}
	h.mu.Unlock()
}

func replyErr(r reply) error {
	if r.Err == "" {
		return nil
	}
	switch r.Code {
	case codeStale:
		return fmt.Errorf("%w: %s", ErrStale, r.Err)
	case codeNoUsableKey:
		return fmt.Errorf("%w: %s", ErrNoUsableKey, r.Err)
	case codeEnded:
		return fmt.Errorf("%w: %s", ErrEnded, r.Err)
	case codeSandbox:
		// r.Err starts with ErrSandbox's text; keep only the reason.
		return fmt.Errorf("%w%s", ErrSandbox, strings.TrimPrefix(r.Err, ErrSandbox.Error()))
	case codeCDMLoad:
		return fmt.Errorf("%w%s", ErrCDMLoad, strings.TrimPrefix(r.Err, ErrCDMLoad.Error()))
	case "exited":
		return fmt.Errorf("%w: %s", ErrExited, r.Err)
	}
	return errors.New(r.Err)
}

// SetServerCertificate installs the licence server's service certificate.
func (h *Host) SetServerCertificate(ctx context.Context, cert []byte) error {
	_, err := h.call(ctx, request{Op: opCert, Data: cert})
	return err
}

// CreateSession opens a CDM session for key ID kid (16 bytes) or for a
// whole Widevine 'pssh' box, and returns the licence request to send to
// the licence server.
func (h *Host) CreateSession(ctx context.Context, kid []byte) (session string, challenge []byte, err error) {
	r, err := h.call(ctx, request{Op: opSession, KID: kid})
	return r.Session, r.Challenge, err
}

// UpdateSession passes a licence (or renewal) response to session.
func (h *Host) UpdateSession(ctx context.Context, session string, license []byte) error {
	_, err := h.call(ctx, request{Op: opUpdate, Session: session, Data: license})
	return err
}

// CloseSession closes a CDM session.
func (h *Host) CloseSession(ctx context.Context, session string) error {
	_, err := h.call(ctx, request{Op: opCloseSession, Session: session})
	return err
}

// Load makes track the current generation and flushes playback (and a
// track queued with Next); the first fragment fed for it starts at start
// (earlier samples are clipped). A seek reloads with m.Init nil, keeping
// the track's init segment, duration and gain.
func (h *Host) Load(ctx context.Context, track uint64, m Media, start time.Duration) error {
	_, err := h.call(ctx, request{Op: opLoad, Track: track, Data: m.Init, Start: start, Duration: m.Duration, Gain: m.Gain})
	return err
}

// Next queues track to follow the current one without a gap, crossfading
// over fade (0: sample-exact). Its fragments are fed like the current
// track's; EventAdvance reports when it is heard. A later Next replaces
// it, track 0 drops it. ErrEnded means the current track has already
// played out.
func (h *Host) Next(ctx context.Context, track uint64, m Media, fade time.Duration) error {
	_, err := h.call(ctx, request{Op: opNext, Track: track, Data: m.Init, Duration: m.Duration, Gain: m.Gain, Fade: fade})
	return err
}

// Feed sends one encrypted fragment (moof + mdat) of track. It returns once
// the child has decrypted and queued it, which blocks while the child's
// queue is full. ErrStale means a later Load superseded track.
func (h *Host) Feed(ctx context.Context, track uint64, fragment []byte) error {
	_, err := h.call(ctx, request{Op: opFeed, Track: track, Data: fragment})
	return err
}

// StreamFragment is one fragment of a stream (live radio, episodes):
// Init, when set, is a new init segment that takes effect from this
// fragment on (same audio configuration); Shift is subtracted from the
// fragment's timestamps to place it on the track timeline; KID, when set,
// is the key ID to decrypt with (Apple's radio init segments carry an
// all-zero default key ID; the real one is in the playlist).
type StreamFragment struct {
	Init  []byte
	Data  []byte
	Shift time.Duration
	KID   []byte
}

// FeedStream is Feed for a stream fragment.
func (h *Host) FeedStream(ctx context.Context, track uint64, f StreamFragment) error {
	_, err := h.call(ctx, request{Op: opFeed, Track: track, Data: f.Data, Init: f.Init, Shift: f.Shift, KID: f.KID})
	return err
}

// EndOfTrack marks the last fragment of track; EventEnded follows when it
// has played out.
func (h *Host) EndOfTrack(ctx context.Context, track uint64) error {
	_, err := h.call(ctx, request{Op: opEnd, Track: track})
	return err
}

// Play starts or resumes output; EventStarted follows with the exact position.
func (h *Host) Play(ctx context.Context) error {
	_, err := h.call(ctx, request{Op: opPlay})
	return err
}

// Pause pauses output and returns the position it paused at.
func (h *Host) Pause(ctx context.Context) (time.Duration, bool, error) {
	r, err := h.call(ctx, request{Op: opPause})
	return r.Position, r.HasPos, err
}

// Stop flushes playback into the empty generation track and pauses.
func (h *Host) Stop(ctx context.Context, track uint64) error {
	_, err := h.call(ctx, request{Op: opStop, Track: track})
	return err
}

// SetVolume sets the linear output volume.
func (h *Host) SetVolume(ctx context.Context, v float64) error {
	_, err := h.call(ctx, request{Op: opVolume, Volume: v})
	return err
}

// Position queries the current playback position.
func (h *Host) Position(ctx context.Context) (time.Duration, bool, error) {
	r, err := h.call(ctx, request{Op: opPosition})
	return r.Position, r.HasPos, err
}

// SetEqualizer changes the equaliser of the playing audio at once.
func (h *Host) SetEqualizer(ctx context.Context, eq equalizer.Curve) error {
	_, err := h.call(ctx, request{Op: opEqualizer, EQ: &eq})
	return err
}

// SetOutputDevice moves output to device (Options.Device; "" follows the
// default), keeping the position and what is queued.
func (h *Host) SetOutputDevice(ctx context.Context, device string) error {
	_, err := h.call(ctx, request{Op: opDevice, Device: device})
	return err
}

// LoadFile makes track the current generation as a plain (unencrypted)
// audio file of size bytes, such as a library upload, played from start.
// Its bytes follow with FeedFile, in order, and EndOfFile; the child
// decodes them itself (MP3, AAC, ALAC) without the CDM. A later Load with
// no init segment seeks within the file. Size gst.Live loads a live
// stream (a broadcaster's MP3 or ADTS AAC): no length, no seeking, start
// 0; FeedFile then fails while too much of it is unplayed.
func (h *Host) LoadFile(ctx context.Context, track uint64, size int64, start time.Duration, gain float64) error {
	_, err := h.call(ctx, request{Op: opLoadFile, Track: track, Size: size, Start: start, Gain: gain})
	return err
}

// SetTrackGain turns Sound Check on or off: tracks play with the gain
// they were loaded with, or at unity. It applies to the playing track at
// once.
func (h *Host) SetTrackGain(ctx context.Context, on bool) error {
	_, err := h.call(ctx, request{Op: opTrackGain, Enabled: on})
	return err
}

// FeedFile passes the file bytes at offset of the file loaded as track
// (the generation LoadFile was given; seeks do not change it).
func (h *Host) FeedFile(ctx context.Context, track uint64, offset int64, data []byte) error {
	_, err := h.call(ctx, request{Op: opFileData, Track: track, Offset: offset, Data: data})
	return err
}

// EndOfFile reports that every byte of the file loaded as track was fed.
func (h *Host) EndOfFile(ctx context.Context, track uint64) error {
	_, err := h.call(ctx, request{Op: opFileEnd, Track: track})
	return err
}

// Close ends the child: closing stdin makes it exit; it is killed if it has
// not exited within two seconds.
func (h *Host) Close() error {
	h.closing.Store(true)
	h.encMu.Lock()
	h.stdin.Close()
	h.encMu.Unlock()
	select {
	case <-h.done:
	case <-time.After(2 * time.Second):
		_ = h.cmd.Process.Kill()
		<-h.done
	}
	return nil
}

// Kill terminates the child immediately (tests, crash recovery).
func (h *Host) Kill() error {
	h.closing.Store(true)
	err := h.cmd.Process.Kill()
	<-h.done
	return err
}

// Probe spawns a child to report the CDM and audio stack for Diagnostics:
// CDM path and version, whether it initialised (err nil), GStreamer version,
// decoder and sink; with o.Clear, the audio stack alone. The returned Info
// is filled as far as the child got.
func Probe(ctx context.Context, o Options) (Info, error) {
	if o.Clear {
		o.CDMPath = ""
	}
	h, err := spawn(o)
	if err != nil {
		return Info{}, err
	}
	defer h.Close()
	r, err := h.call(ctx, request{Op: opInit, CDMPath: o.CDMPath, Clear: o.Clear, Sink: o.Sink})
	var info Info
	if r.Info != nil {
		info = *r.Info
	}
	info.PID = h.cmd.Process.Pid
	info.Sandbox = h.withSandbox(info.Sandbox)
	return info, err
}

// LoadCheck returns the check internal/playback/cdm runs on a newly
// downloaded CDM before it becomes current: a probe child loads and
// initialises the CDM, as a playback host would, and nothing else (no
// GStreamer: see envCDMOnly). The CDM failing to load or initialise, or
// the child dying while it loads, wraps cdm.ErrNotLoadable and rejects
// the version; a check that runs out of time or whose child cannot start
// says nothing about the CDM and leaves the version undecided. Without
// its sandbox the child loads no CDM at all, so the check can't tell and
// passes, and playback reports the sandbox itself.
func LoadCheck(log *slog.Logger) func(ctx context.Context, lib string) error {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return func(ctx context.Context, lib string) error {
		info, err := Probe(ctx, Options{CDMPath: lib, Logger: log, cdmOnly: true})
		switch {
		case info.CDMInterface != 0:
			return nil
		case errors.Is(err, ErrSandbox):
			log.Warn("cdmhost: load check skipped: the playback sandbox is unavailable", "err", err)
			return nil
		case ctx.Err() != nil:
			return fmt.Errorf("cdmhost: load check: %w", ctx.Err())
		case err == nil:
			return fmt.Errorf("%w: the probe reported no CDM interface", cdm.ErrNotLoadable)
		case errors.Is(err, ErrCDMLoad), errors.Is(err, ErrExited):
			return fmt.Errorf("%w: %w", cdm.ErrNotLoadable, err)
		}
		return err
	}
}

// PrepareAudio readies GStreamer for the playback hosts that follow: a
// child that loads no CDM initialises GStreamer, which rebuilds its plugin
// registry when that is missing or stale (a computer's first run, or
// changed plugins), and reports the GStreamer version. A host started
// afterwards reads the registry instead of scanning every plugin first,
// which takes seconds on Linux and can take over a minute on a fresh Mac.
func PrepareAudio(ctx context.Context, o Options) (Info, error) {
	o.CDMPath, o.cdmOnly = "", false
	h, err := spawn(o)
	if err != nil {
		return Info{}, err
	}
	defer func() { _ = h.Close() }()
	r, err := h.call(ctx, request{Op: opAudio})
	var info Info
	if r.Info != nil {
		info = *r.Info
	}
	info.PID = h.cmd.Process.Pid
	info.Sandbox = h.withSandbox(info.Sandbox)
	return info, err
}
