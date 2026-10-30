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
	"strings"
	"sync"
	"time"
)

// Options configures Start.
type Options struct {
	// CDMPath overrides CDM discovery in the child (widevine.FindCDM).
	CDMPath string
	// Sink overrides the GStreamer sink (e.g. "fakesink" in tests).
	Sink string
	// Logger receives the child's log records; nil discards them.
	Logger *slog.Logger
	// Exe is the executable to run as the child; default os.Executable().
	// It must call ChildMain first thing in main.
	Exe string
}

// Host is the parent's handle on one child process. Its methods are safe for
// concurrent use; calls that block in the child (Feed) do not delay others.
type Host struct {
	log   *slog.Logger
	cmd   *exec.Cmd
	stdin io.WriteCloser
	info  Info

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

	done chan struct{}
}

// childEnv is the parent environment minus credentials, plus the child marker.
func childEnv(log *slog.Logger) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if k == EnvChild || k == envLogLevel || strings.Contains(strings.ToUpper(k), "TOKEN") {
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

// Start spawns a child, loads the CDM and builds the audio pipeline. The
// child exits when the Host is closed or the parent dies.
func Start(ctx context.Context, o Options) (*Host, error) {
	h, err := spawn(o)
	if err != nil {
		return nil, err
	}
	r, err := h.call(ctx, request{Op: opInit, CDMPath: o.CDMPath, Sink: o.Sink})
	if r.Info != nil {
		h.info = *r.Info
	}
	h.info.PID = h.cmd.Process.Pid
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
	cmd := exec.Command(exe)
	cmd.Env = childEnv(log)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cdmhost: start %s: %w", exe, err)
	}
	h := &Host{
		log:     log.With("component", "cdmhost", "pid", cmd.Process.Pid),
		cmd:     cmd,
		stdin:   stdin,
		enc:     gob.NewEncoder(stdin),
		pending: map[uint64]chan reply{},
		evWake:  make(chan struct{}, 1),
		events:  make(chan Event, 16),
		done:    make(chan struct{}),
	}
	go h.relayLogs(stderr)
	go h.pump()
	go h.read(stdout)
	return h, nil
}

// Info describes the child.
func (h *Host) Info() Info { return h.info }

// Events delivers child events in order until the child exits. Events are
// queued without bound, so a slow receiver never stalls replies.
func (h *Host) Events() <-chan Event { return h.events }

// Done is closed when the child has exited; Err then says why.
func (h *Host) Done() <-chan struct{} { return h.done }

// Err returns the reason the child exited, or nil while it runs.
func (h *Host) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

func (h *Host) read(stdout io.Reader) {
	dec := gob.NewDecoder(bufio.NewReaderSize(stdout, 64<<10))
	var readErr error
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
			h.evMu.Lock()
			h.evq = append(h.evq, *f.Event)
			h.evMu.Unlock()
			select {
			case h.evWake <- struct{}{}:
			default:
			}
		}
	}
	waitErr := h.cmd.Wait()
	err := ErrExited
	switch {
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
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		var rec map[string]any
		if json.Unmarshal(line, &rec) != nil {
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

// Load makes track the current generation and flushes playback; the first
// fragment fed for it starts at start (earlier samples are clipped). init is
// the fMP4 init segment, or nil to keep the previous one (a seek).
func (h *Host) Load(ctx context.Context, track uint64, init []byte, start time.Duration) error {
	_, err := h.call(ctx, request{Op: opLoad, Track: track, Data: init, Start: start})
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

// Close ends the child: closing stdin makes it exit; it is killed if it has
// not exited within two seconds.
func (h *Host) Close() error {
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
	err := h.cmd.Process.Kill()
	<-h.done
	return err
}

// Probe spawns a child to report the CDM and audio stack for Diagnostics:
// CDM path and version, whether it initialised (err nil), GStreamer version,
// decoder and sink. The returned Info is filled as far as the child got.
func Probe(ctx context.Context, o Options) (Info, error) {
	h, err := spawn(o)
	if err != nil {
		return Info{}, err
	}
	defer h.Close()
	r, err := h.call(ctx, request{Op: opInit, CDMPath: o.CDMPath, Sink: o.Sink})
	var info Info
	if r.Info != nil {
		info = *r.Info
	}
	info.PID = h.cmd.Process.Pid
	return info, err
}
