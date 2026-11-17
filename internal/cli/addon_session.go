package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"sync"
	"time"
)

// AddonHost is what Sona offers one add-on session: the v1 commands, as
// the running Sona answers them, and the topics' values. The host decides
// what an add-on may know (a private listen, a preview) and answers as if
// there were nothing: the session only enforces the protocol, the
// permissions and the limits.
type AddonHost interface {
	// Call runs a command that passed the table's checks and returns one of
	// the types in schema.go. An *Error keeps its code (its message never
	// reaches the add-on).
	Call(ctx context.Context, req Request) (any, error)
	// Watch delivers topic's value at once and then each change, latest
	// wins, until cancel. It fails for a topic that doesn't exist.
	Watch(topic string) (updates <-chan any, cancel func(), err error)
}

// AddonOwn is what one add-on's session serves of its own data.
type AddonOwn interface {
	// WatchSettings delivers the SettingsValues now and at each change,
	// latest wins, until cancel.
	WatchSettings() (updates <-chan any, cancel func())
	// Secret is the value the user saved in the add-on's own secret row key.
	Secret(key string) (string, bool)
}

// AddonConfig configures a Session.
type AddonConfig struct {
	// ID is the add-on's ID as the supervisor started it; hello must name it.
	ID   string
	Host AddonHost
	// Own serves what is the add-on's alone: its settings rows' values and
	// its secrets. Nil: the settings topic and the secret method don't exist
	// for this session.
	Own AddonOwn
	// Allowed reports whether the add-on holds a permission right now. It is
	// asked at every call and every event, never cached: a revoked
	// permission refuses the very next call.
	Allowed func(permission string) bool
	// Limits' zero fields take their defaults.
	Limits AddonLimits
	// Rates replaces defaultRates (by permission; "" is the rest).
	Rates map[string]Rate
	Log   *slog.Logger
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Credentials are sent in the hello answer: the build credentials of one
	// of Sona's own add-ons, never of another. The session logs none of it.
	Credentials map[string]string
	// Owner serves the person's own program (`sona connect`), not an
	// add-on: hello may name any ID (ID is unset), and private listening
	// may be switched, as the person may from the command line.
	Owner bool
}

const (
	// flushGrace is how long a session that is ending waits for its last
	// frames to be written before closing the channel anyway.
	flushGrace = 2 * time.Second
	// DefaultInvokeDeadline and MaxInvokeDeadline bound the time an add-on
	// has to answer an invoke.
	DefaultInvokeDeadline = 10 * time.Second
	MaxInvokeDeadline     = 30 * time.Second
)

// Session is Sona's end of one add-on's channel: newline-delimited JSON
// frames, protocol v2. Create one with NewSession and run it with Serve.
type Session struct {
	cfg    AddonConfig
	limits AddonLimits
	now    func() time.Time
	log    *slog.Logger

	mu       sync.Mutex
	wake     *sync.Cond
	conn     io.Closer
	helloed  bool
	closing  bool // no more frames are accepted; the writer ends once drained
	dead     bool // the channel is closed
	replies  [][]byte
	latest   map[string][]byte // the newest event of each topic, not yet written
	order    []string          // topics in latest, oldest first
	subs     map[string]func() // topic → cancel
	calls    int
	rates    map[string]*bucket
	invokes  map[int64]chan invokeAnswer
	nextID   int64
	done     chan struct{}
	helloCh  chan struct{} // closed once hello is accepted
	started  bool
	wg       sync.WaitGroup
	callCtx  context.Context
	callStop context.CancelFunc
}

type invokeAnswer struct {
	result json.RawMessage
	err    *Error
}

// NewSession returns a session for cfg. Allowed and Host are required.
func NewSession(cfg AddonConfig) *Session {
	s := &Session{
		cfg: cfg, limits: cfg.Limits.withDefaults(), now: cfg.Now, log: cfg.Log,
		latest: map[string][]byte{}, subs: map[string]func(){}, rates: map[string]*bucket{},
		invokes: map[int64]chan invokeAnswer{}, done: make(chan struct{}), helloCh: make(chan struct{}),
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	s.wake = sync.NewCond(&s.mu)
	return s
}

// Done is closed when the session has ended.
func (s *Session) Done() <-chan struct{} { return s.done }

// Helloed is closed when the add-on's hello was accepted. A supervisor that
// sees no hello in time stops the add-on.
func (s *Session) Helloed() <-chan struct{} { return s.helloCh }

// Serve runs the session over conn until the add-on hangs up, breaks the
// protocol, doesn't read its frames, or ctx or Close ends it. It closes
// conn. It returns nil for an add-on that hung up or was told to stop.
func (s *Session) Serve(ctx context.Context, conn io.ReadWriteCloser) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("cli: session served twice")
	}
	s.started, s.conn = true, conn
	s.callCtx, s.callStop = context.WithCancel(ctx)
	s.mu.Unlock()
	defer close(s.done)
	stop := context.AfterFunc(ctx, func() { s.abort() })
	defer stop()

	wdone := make(chan struct{})
	go func() {
		defer close(wdone)
		s.write(conn)
	}()
	err := s.read(conn)
	if s.isClosing() {
		// A violation or Close: the frames queued for the add-on go out first
		// (the writer ends the session, or the grace closes the channel).
		<-wdone
	}
	s.abort()
	<-wdone
	s.wg.Wait()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// Close tells the add-on to stop (the shutdown event) and ends the session
// once that is written, or after a short grace when the add-on doesn't read.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return
	}
	if s.helloed {
		s.replies = append(s.replies, mustFrame(Frame{Event: EventShutdown}))
	}
	s.closeLocked()
}

// closeLocked stops accepting frames and lets the writer drain.
func (s *Session) closeLocked() {
	if s.closing {
		return
	}
	s.closing = true
	if c := s.conn; c != nil {
		time.AfterFunc(flushGrace, func() { _ = c.Close() })
	}
	s.wake.Broadcast()
}

// abort closes the channel now.
func (s *Session) abort() {
	s.mu.Lock()
	s.closing, s.dead = true, true
	conn := s.conn
	subs := s.subs
	s.subs = map[string]func(){}
	for id, ch := range s.invokes {
		delete(s.invokes, id)
		ch <- invokeAnswer{err: addonError(AddonErrNotRunning)}
	}
	if s.callStop != nil {
		s.callStop()
	}
	s.wake.Broadcast()
	s.mu.Unlock()
	for _, cancel := range subs {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
}

func mustFrame(f Frame) []byte {
	b, err := json.Marshal(f)
	if err != nil {
		// Only a RawMessage that isn't JSON can fail; every one is made here.
		b, _ = json.Marshal(Frame{Error: addonError(AddonErrFailed)})
	}
	return append(b, '\n')
}

// write is the one writer of the channel: replies and requests in order,
// then the newest event of each topic. A slow add-on makes it wait; it
// never makes anything else wait.
func (s *Session) write(w io.Writer) {
	for {
		s.mu.Lock()
		for !s.dead && len(s.replies) == 0 && len(s.order) == 0 && !s.closing {
			s.wake.Wait()
		}
		var line []byte
		switch {
		case s.dead:
			s.mu.Unlock()
			return
		case len(s.replies) > 0:
			line = s.replies[0]
			s.replies[0] = nil
			s.replies = s.replies[1:]
		case len(s.order) > 0 && !s.closing:
			t := s.order[0]
			s.order = s.order[1:]
			line = s.latest[t]
			delete(s.latest, t)
		default: // closing and drained
			s.mu.Unlock()
			s.abort()
			return
		}
		s.mu.Unlock()
		if _, err := w.Write(line); err != nil {
			s.abort()
			return
		}
	}
}

// reply queues a frame ahead of the events. An add-on that lets more than
// Limits.Queue of them pile up isn't reading: the session ends.
func (s *Session) reply(f Frame) {
	b := mustFrame(f)
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return
	}
	if len(s.replies) >= s.limits.Queue {
		s.mu.Unlock()
		s.log.Warn("addon: not reading its frames; ending the session", "addon", s.cfg.ID)
		s.abort()
		return
	}
	s.replies = append(s.replies, b)
	s.wake.Signal()
	s.mu.Unlock()
}

func (s *Session) fail(id int64, code string) {
	s.reply(Frame{ID: id, Error: addonError(code)})
}

// failAndClose answers a violation and ends the session.
func (s *Session) failAndClose(id int64, code string) {
	b := mustFrame(Frame{ID: id, Error: addonError(code)})
	s.mu.Lock()
	if !s.closing && len(s.replies) < s.limits.Queue {
		s.replies = append(s.replies, b)
	}
	s.closeLocked()
	s.mu.Unlock()
}

// event queues topic's newest event, replacing one not yet written: a slow
// reader gets the latest and the publisher never waits.
func (s *Session) event(topic string, b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, subscribed := s.subs[topic]; s.closing || !subscribed {
		return
	}
	if _, ok := s.latest[topic]; !ok {
		s.order = append(s.order, topic)
	}
	s.latest[topic] = b
	s.wake.Signal()
}

// errFrameTooLarge is readFrame's answer to a line longer than the limit.
var errFrameTooLarge = errors.New("cli: frame too large")

// readFrame reads one line of at most limit bytes (without the newline).
func readFrame(r *bufio.Reader, limit int) ([]byte, error) {
	var line []byte
	for {
		part, err := r.ReadSlice('\n')
		line = append(line, part...)
		if len(line) > limit+1 { // +1 for the newline
			return nil, errFrameTooLarge
		}
		switch {
		case err == nil:
			return line[:len(line)-1], nil
		case errors.Is(err, bufio.ErrBufferFull):
		case errors.Is(err, io.EOF) && len(line) > 0:
			return line, nil
		default:
			return nil, err
		}
	}
}

// read handles the add-on's frames until it hangs up or the session ends.
func (s *Session) read(r io.Reader) error {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := readFrame(br, s.limits.MaxFrame)
		switch {
		case errors.Is(err, errFrameTooLarge):
			s.failAndClose(0, AddonErrFrameTooLarge)
			return err
		case errors.Is(err, io.EOF), err != nil && s.isClosing():
			return nil
		case err != nil:
			return err
		}
		if len(line) == 0 {
			continue
		}
		if !s.handle(line) {
			return nil
		}
	}
}

func (s *Session) isClosing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closing
}

// handle processes one frame; false ends the session.
func (s *Session) handle(line []byte) bool {
	var f Frame
	err := json.Unmarshal(line, &f)
	s.mu.Lock()
	helloed, closing := s.helloed, s.closing
	s.mu.Unlock()
	if closing {
		return false
	}
	if !helloed {
		// Before hello nothing but hello is answered: a client that skips it,
		// or sends a v1 command, gets an error and no more.
		if err != nil || f.Method != AddonMethodHello || f.ID < 1 {
			s.failAndClose(f.ID, AddonErrHelloRequired)
			return false
		}
		return s.hello(f)
	}
	switch {
	case err != nil:
		s.fail(0, AddonErrBadFrame)
	case f.Method != "":
		if f.ID < 1 {
			s.fail(0, AddonErrBadFrame)
			break
		}
		s.request(f)
	case f.ID > 0 && (f.Result != nil || f.Error != nil):
		s.answer(f)
	default:
		s.fail(0, AddonErrBadFrame)
	}
	return true
}

// hello accepts or refuses the add-on's hello; false ends the session.
func (s *Session) hello(f Frame) bool {
	var p HelloParams
	if err := json.Unmarshal(f.Params, &p); err != nil || p.ID == "" || (!s.cfg.Owner && p.ID != s.cfg.ID) || len(p.Permissions) > len(Permissions) {
		s.failAndClose(f.ID, AddonErrBadHello)
		return false
	}
	for _, perm := range p.Permissions {
		if !KnownPermission(perm) {
			s.failAndClose(f.ID, AddonErrBadHello)
			return false
		}
	}
	ver, ok := NegotiateVersion(p.Protocol)
	if !ok {
		s.failAndClose(f.ID, AddonErrVersion)
		return false
	}
	granted := []string{}
	for _, perm := range Permissions {
		if slices.Contains(p.Permissions, perm) && s.cfg.Allowed(perm) {
			granted = append(granted, perm)
		}
	}
	res, _ := json.Marshal(HelloResult{ID: p.ID, Protocol: ver, Permissions: granted, Limits: s.limits, Credentials: s.cfg.Credentials})
	s.mu.Lock()
	s.helloed = true
	close(s.helloCh) // hello is accepted once: a second one never gets here
	s.mu.Unlock()
	s.reply(Frame{ID: f.ID, Result: res})
	return true
}

func (s *Session) request(f Frame) {
	switch f.Method {
	case AddonMethodHello:
		s.fail(f.ID, AddonErrAlreadyHello)
	case AddonMethodSubscribe:
		s.subscribe(f)
	case AddonMethodUnsubscribe:
		s.unsubscribe(f)
	case AddonMethodCall:
		s.call(f)
	case AddonMethodSecret:
		s.secret(f)
	default:
		s.fail(f.ID, AddonErrUnknownMethod)
	}
}

// topicsOf decodes a subscribe or unsubscribe's topics, checked against
// the limit; "" is the code of what is wrong.
func (s *Session) topicsOf(f Frame) ([]string, string) {
	var p TopicsParams
	if err := json.Unmarshal(f.Params, &p); err != nil {
		return nil, AddonErrBadArguments
	}
	if len(p.Topics) > s.limits.Subscriptions {
		return nil, AddonErrTooMany
	}
	out := make([]string, 0, len(p.Topics))
	for _, t := range p.Topics {
		if _, ok := TopicPermission(t); !ok || (t == TopicSettings && s.cfg.Own == nil) {
			return nil, AddonErrUnknownTopic
		}
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out, ""
}

func (s *Session) subscribe(f Frame) {
	topics, code := s.topicsOf(f)
	if code != "" {
		s.fail(f.ID, code)
		return
	}
	for _, t := range topics {
		if perm, _ := TopicPermission(t); perm != "" && !s.cfg.Allowed(perm) {
			s.fail(f.ID, AddonErrPermission)
			return
		}
	}
	s.mu.Lock()
	var fresh []string
	for _, t := range topics {
		if _, ok := s.subs[t]; !ok {
			fresh = append(fresh, t)
		}
	}
	if s.dead || len(s.subs)+len(fresh) > s.limits.Subscriptions {
		s.mu.Unlock()
		s.fail(f.ID, AddonErrTooMany)
		return
	}
	type watch struct {
		topic string
		ch    <-chan any
	}
	var started []watch
	for _, t := range fresh {
		var (
			ch     <-chan any
			cancel func()
			err    error
		)
		if t == TopicSettings {
			ch, cancel = s.cfg.Own.WatchSettings()
		} else {
			ch, cancel, err = s.cfg.Host.Watch(t)
		}
		if err != nil {
			s.mu.Unlock()
			s.fail(f.ID, AddonErrUnknownTopic)
			return
		}
		s.subs[t] = cancel
		started = append(started, watch{t, ch})
	}
	active := s.activeLocked()
	s.mu.Unlock()
	res, _ := json.Marshal(TopicsParams{Topics: active})
	s.reply(Frame{ID: f.ID, Result: res})
	for _, w := range started {
		s.wg.Add(1)
		go s.pump(w.topic, w.ch)
	}
}

func (s *Session) activeLocked() []string {
	out := make([]string, 0, len(s.subs))
	for _, t := range Topics {
		if _, ok := s.subs[t]; ok {
			out = append(out, t)
		}
	}
	return out
}

func (s *Session) unsubscribe(f Frame) {
	topics, code := s.topicsOf(f)
	if code != "" {
		s.fail(f.ID, code)
		return
	}
	s.mu.Lock()
	var cancels []func()
	for _, t := range topics {
		if c, ok := s.subs[t]; ok {
			cancels = append(cancels, c)
			delete(s.subs, t)
			delete(s.latest, t)
			s.order = slices.DeleteFunc(s.order, func(o string) bool { return o == t })
		}
	}
	active := s.activeLocked()
	s.mu.Unlock()
	for _, c := range cancels {
		c()
	}
	res, _ := json.Marshal(TopicsParams{Topics: active})
	s.reply(Frame{ID: f.ID, Result: res})
}

// pump turns the host's values of topic into event frames. The permission
// is asked for each: a revoked one stops the events at the next change.
func (s *Session) pump(topic string, ch <-chan any) {
	defer s.wg.Done()
	perm, _ := TopicPermission(topic)
	for v := range ch {
		if perm != "" && !s.cfg.Allowed(perm) {
			continue
		}
		data, err := json.Marshal(forAddon(v))
		if err != nil {
			s.log.Warn("addon: encode event", "addon", s.cfg.ID, "topic", topic, "err", err)
			continue
		}
		b := mustFrame(Frame{Event: topic, Data: data})
		if len(b) > s.limits.MaxFrame+1 {
			s.log.Warn("addon: event over the frame limit dropped", "addon", s.cfg.ID, "topic", topic)
			continue
		}
		s.event(topic, b)
	}
}

// secretKeyExpr is a secret row's ID.
var secretKeyExpr = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// secret answers an add-on's own secret row. It is the add-on's alone: the
// host looks the key up under this add-on's ID, so no key reaches another's.
func (s *Session) secret(f Frame) {
	if s.cfg.Own == nil {
		s.fail(f.ID, AddonErrUnknownMethod)
		return
	}
	var p SecretParams
	if err := json.Unmarshal(f.Params, &p); err != nil || !secretKeyExpr.MatchString(p.Key) {
		s.fail(f.ID, AddonErrBadArguments)
		return
	}
	if !s.allowRate("") {
		s.fail(f.ID, AddonErrRateLimited)
		return
	}
	s.mu.Lock()
	if s.calls >= s.limits.Calls || s.dead {
		s.mu.Unlock()
		s.fail(f.ID, AddonErrBusy)
		return
	}
	s.calls++
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			s.calls--
			s.mu.Unlock()
		}()
		v, ok := s.cfg.Own.Secret(p.Key)
		if !ok {
			s.fail(f.ID, "not_found")
			return
		}
		b, _ := json.Marshal(SecretResult{Value: v})
		s.reply(Frame{ID: f.ID, Result: b})
	}()
}

// call runs a v1 command for the add-on.
func (s *Session) call(f Frame) {
	var req Request
	if err := json.Unmarshal(f.Params, &req); err != nil {
		s.fail(f.ID, AddonErrBadArguments)
		return
	}
	perm, ok := RequestPermission(req)
	if !ok {
		s.fail(f.ID, CodeUnknownCommand)
		return
	}
	// Private listening is the user's alone to switch: an add-on may read it.
	if !s.cfg.Allowed(perm) || (!s.cfg.Owner && req.Command == "private" && (len(req.Args) > 1 || (len(req.Args) == 1 && req.Args[0] != "status"))) {
		s.fail(f.ID, AddonErrPermission)
		return
	}
	if len(req.Args) > s.limits.Args {
		s.fail(f.ID, AddonErrBadArguments)
		return
	}
	for _, a := range req.Args {
		if len(a) > s.limits.ArgBytes {
			s.fail(f.ID, AddonErrBadArguments)
			return
		}
	}
	if err := checkRequest(req); err != nil {
		s.reply(Frame{ID: f.ID, Error: addonCode(err)})
		return
	}
	if !s.allowRate(perm) {
		s.fail(f.ID, AddonErrRateLimited)
		return
	}
	s.mu.Lock()
	if s.calls >= s.limits.Calls || s.dead {
		s.mu.Unlock()
		s.fail(f.ID, AddonErrBusy)
		return
	}
	s.calls++
	s.wg.Add(1)
	ctx := s.callCtx
	s.mu.Unlock()
	go s.runCall(ctx, f.ID, req)
}

func (s *Session) runCall(ctx context.Context, id int64, req Request) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		s.calls--
		s.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(ctx, lookup(req.Command).roundTrip())
	defer cancel()
	res, err := s.cfg.Host.Call(ctx, req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			err = &Error{Code: AddonErrTimeout}
		}
		s.reply(Frame{ID: id, Error: addonCode(err)})
		return
	}
	b, err := json.Marshal(forAddon(res))
	if err != nil {
		s.reply(Frame{ID: id, Error: addonError(AddonErrFailed)})
		return
	}
	f := Frame{ID: id, Result: b}
	if len(mustFrame(f)) > s.limits.MaxFrame+1 {
		f = Frame{ID: id, Error: addonError(AddonErrResultTooLarge)}
	}
	s.reply(f)
}

// allowRate takes a token from the permission's bucket.
func (s *Session) allowRate(perm string) bool {
	rates := s.cfg.Rates
	if rates == nil {
		rates = defaultRates
	}
	key := perm
	r, ok := rates[key]
	if !ok {
		key, r = "", rates[""]
	}
	if r.Burst <= 0 {
		return true
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.rates[key]
	if b == nil {
		b = newBucket(r, now)
		s.rates[key] = b
	}
	return b.take(now)
}

// answer routes an add-on's response to the invoke it answers; one that
// answers nothing (late, or never asked) is dropped.
func (s *Session) answer(f Frame) {
	s.mu.Lock()
	ch, ok := s.invokes[f.ID]
	delete(s.invokes, f.ID)
	s.mu.Unlock()
	if !ok {
		return
	}
	ans := invokeAnswer{result: f.Result}
	if f.Error != nil {
		ans.err = addonCode(f.Error)
	}
	ch <- ans // buffered
}

// Invoke asks the add-on to run one of its contributions and waits for its
// answer, at most until ctx ends or MaxInvokeDeadline passes (the add-on is
// told the time it has left). It never waits on anything but the add-on: a
// hung add-on costs the caller the deadline and nothing else. Only so many
// invokes may await an answer at once; more fail with busy.
func (s *Session) Invoke(ctx context.Context, action string, target json.RawMessage) (InvokeResult, error) {
	deadline := DefaultInvokeDeadline
	if d, ok := ctx.Deadline(); ok {
		deadline = time.Until(d)
	}
	deadline = min(deadline, MaxInvokeDeadline)
	if deadline <= 0 {
		return InvokeResult{}, addonError(AddonErrTimeout)
	}
	if len(target) == 0 {
		target = json.RawMessage("null")
	}
	params, err := json.Marshal(InvokeParams{Action: action, Target: target, DeadlineMs: deadline.Milliseconds()})
	if err != nil {
		return InvokeResult{}, addonError(AddonErrBadArguments)
	}
	s.mu.Lock()
	if !s.helloed || s.closing {
		s.mu.Unlock()
		return InvokeResult{}, addonError(AddonErrNotRunning)
	}
	if len(s.invokes) >= s.limits.Invokes {
		s.mu.Unlock()
		return InvokeResult{}, addonError(AddonErrBusy)
	}
	s.nextID++
	id := s.nextID
	ch := make(chan invokeAnswer, 1)
	s.invokes[id] = ch
	s.mu.Unlock()
	s.reply(Frame{ID: id, Method: AddonMethodInvoke, Params: params})
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	forget := func() {
		s.mu.Lock()
		delete(s.invokes, id)
		s.mu.Unlock()
	}
	select {
	case ans := <-ch:
		if ans.err != nil {
			return InvokeResult{}, ans.err
		}
		r, ok := ParseInvokeResult(ans.result)
		if !ok {
			return InvokeResult{}, addonError(AddonErrFailed)
		}
		return r, nil
	case <-timer.C:
		forget()
		return InvokeResult{}, addonError(AddonErrTimeout)
	case <-ctx.Done():
		forget()
		return InvokeResult{}, addonError(AddonErrTimeout)
	}
}
