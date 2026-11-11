// Package cli is the `sona <command>` command line: the command table
// (parsing, help, the manual page and the shell completions are generated
// from it), the JSON schemas commands answer with, the client that sends a
// command to the running Sona and prints its answer, and the protocol both
// ends share. The transports (D-Bus on Linux, a local socket elsewhere)
// are internal/app's; the running Sona's side is internal/services'.
//
// This package imports nothing of Sona's but Go's message catalogue
// (internal/i18n/l10n), so every other package may use its types.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
)

// Request is one command sent to the running Sona. Args are the command's
// positional arguments; options that only change how the answer is
// printed (--json, --format, --follow) stay with the client.
type Request struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	// Kind and Limit narrow `search` (--type, --limit).
	Kind  string `json:"kind,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// Reply is the running Sona's answer to a Request: Result (the command's
// JSON, schema.go) or Error. A status stream (`status --follow`) is one
// Reply per line, each Result a Status.
type Reply struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Error is a command that failed in the running Sona: a stable Code (the
// client prints its own text for the codes it knows) and Sona's message,
// in English. An Error made here (Usage) also keeps its message's format
// and arguments, so the client words it in the user's language.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`

	format l10n.Message
	args   []any
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// Error codes of the protocol. Others (services' codes, such as
// "not_signed_in" or "network") pass through with Sona's message.
const (
	// CodeUsage: the request's arguments are wrong (exit status 2).
	CodeUsage = "usage"
	// CodeBadLink: `open` was given something that isn't an Apple Music
	// link Sona can open (exit status 2).
	CodeBadLink = "bad_link"
	// CodeUnknownCommand: the running Sona has no such command: it is
	// another version.
	CodeUnknownCommand = "unknown_command"
	// CodeStarting: Sona is running but not ready for commands yet.
	CodeStarting = "starting"
	// CodeShuttingDown: Sona is quitting.
	CodeShuttingDown = "shutting_down"
	// CodeNothingPlaying: the command needs a song selected.
	CodeNothingPlaying = "nothing_playing"
	// CodeNoLyrics: the playing song has no lyrics (none of the sources has
	// them, or it is live radio or a music video).
	CodeNoLyrics = "no_lyrics"
	// CodeNotSynced: lyrics without timing can't be written as LRC. Made by
	// the client, never sent by Sona.
	CodeNotSynced = "not_synced"
	// CodeAccountLoading: Sona is still reading the stored account (a slow
	// keyring); commands that need it can be tried again in a moment.
	CodeAccountLoading = "account_loading"
	// CodeIncompatible: the running Sona speaks another version of the
	// protocol.
	CodeIncompatible = "incompatible"
	// CodeFailed is any other failure.
	CodeFailed = "failed"
)

// Usage returns a CodeUsage error with the message format and args.
func Usage(format l10n.Message, args ...any) *Error {
	return &Error{Code: CodeUsage, Message: l10n.English.Sprintf(format, args...), format: format, args: args}
}

// ErrNotRunning is returned by a Transport (or its dialler) when no Sona
// is running.
var ErrNotRunning = errors.New("no running Sona")

// Transport carries commands to the running Sona.
type Transport interface {
	// Call sends req and returns the command's result. It fails with
	// ErrNotRunning when no Sona runs and with *Error when the command
	// failed there.
	Call(ctx context.Context, req Request) (json.RawMessage, error)
	// Follow calls each with the current status and then with every
	// change, until Sona quits (it returns nil) or ctx ends; an error from
	// each ends it with that error.
	Follow(ctx context.Context, each func(Status) error) error
	Close() error
}

// Server is the running Sona's side of the commands.
type Server interface {
	// Call runs one command and returns its result, one of the types in
	// schema.go. The request has passed the table's checks (a remote
	// command, its arguments and options valid). An *Error keeps its
	// code; any other error is CodeFailed.
	Call(ctx context.Context, req Request) (any, error)
	// Watch delivers the current status at once, then every change, until
	// cancel or until Sona quits (the channel closes).
	Watch() (updates <-chan Status, cancel func())
}

// Handle decodes a request, checks it against the command table, runs it
// on srv and returns the reply to send back. A nil srv means Sona is
// still starting.
func Handle(ctx context.Context, srv Server, raw []byte) Reply {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return Reply{Error: &Error{Code: CodeIncompatible, Message: err.Error()}}
	}
	if err := checkRequest(req); err != nil {
		return Reply{Error: AsError(err)}
	}
	if srv == nil {
		return Reply{Error: &Error{Code: CodeStarting}}
	}
	res, err := srv.Call(ctx, req)
	if err != nil {
		return Reply{Error: AsError(err)}
	}
	b, err := json.Marshal(res)
	if err != nil {
		return Reply{Error: &Error{Code: CodeFailed, Message: err.Error()}}
	}
	return Reply{Result: b}
}

// AsError is err as an *Error: itself when it is one, else CodeFailed with
// its message.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: CodeFailed, Message: err.Error()}
}

// DecodeReply reads a Reply: its Result, or its Error.
func DecodeReply(raw []byte) (json.RawMessage, error) {
	var r Reply
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, &Error{Code: CodeIncompatible, Message: err.Error()}
	}
	if r.Error != nil {
		return nil, r.Error
	}
	return r.Result, nil
}

// StatusReply is s as a Reply, for status streams.
func StatusReply(s Status) []byte {
	b, _ := json.Marshal(s) // plain data: cannot fail
	out, _ := json.Marshal(Reply{Result: b})
	return out
}

// seekJump is how far a status's position may be from where the last one
// published has playback by now before it counts as a jump (a seek, a song
// restarted) and is published: the position moving on its own never is.
const seekJump = 500 * time.Millisecond

// Feed broadcasts the running Sona's status to followers (Server.Watch).
// Publish drops a status that differs from the last one only in its
// position, unless the position jumped, so followers get one value per
// change and every seek.
type Feed struct {
	current func() Status
	now     func() time.Time // time.Now; a test's own

	mu     sync.Mutex
	last   Status
	lastAt time.Time // when last was published
	have   bool
	subs   map[chan Status]struct{}
	closed bool
}

// NewFeed returns a feed whose followers start with current() (the status
// as of the moment they subscribe).
func NewFeed(current func() Status) *Feed {
	return &Feed{current: current, now: time.Now, subs: map[chan Status]struct{}{}}
}

// Publish sends s to every follower unless only its position changed
// since the last status published, and by no more than playing on would
// have moved it. A follower that hasn't taken the previous status gets s
// instead.
func (f *Feed) Publish(s Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	if f.closed || (f.have && s.sameAs(f.last) && !f.jumped(s, now)) {
		return
	}
	f.last, f.lastAt, f.have = s, now, true
	for ch := range f.subs {
		select {
		case <-ch: // replace the status not yet taken
		default:
		}
		ch <- s
	}
}

// jumped reports that s, taken at now, is not where playback of the last
// status published has got to: a seek, or a song started again.
func (f *Feed) jumped(s Status, now time.Time) bool {
	want := f.last.Position
	if f.last.State == StatePlaying {
		want += now.Sub(f.lastAt).Seconds()
		if t := f.last.Track; t != nil && t.Duration > 0 {
			want = min(want, t.Duration)
		}
	}
	return math.Abs(s.Position-want) > seekJump.Seconds()
}

// Watch implements Server.Watch.
func (f *Feed) Watch() (<-chan Status, func()) {
	ch := make(chan Status, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		close(ch)
		return ch, func() {}
	}
	cur := f.current()
	ch <- cur
	f.subs[ch] = struct{}{}
	return ch, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if _, ok := f.subs[ch]; ok {
			delete(f.subs, ch)
			close(ch)
		}
	}
}

// Close ends every follower's channel; later Watch channels start closed.
func (f *Feed) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	for ch := range f.subs {
		close(ch)
	}
	clear(f.subs)
}

// sameAs reports whether s and o differ at most in their position.
func (s Status) sameAs(o Status) bool {
	s.Position, o.Position = 0, 0
	if (s.Track == nil) != (o.Track == nil) {
		return false
	}
	if s.Track != nil && *s.Track != *o.Track {
		return false
	}
	s.Track, o.Track = nil, nil
	return s == o
}
