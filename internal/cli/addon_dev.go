package cli

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// `sona addons dev <folder>` runs an add-on from a folder without installing
// it. The running Sona does the work: it checks the folder, asks the person in
// its own window, runs the add-on and keeps its log; this side asks, then polls
// for the log and prints it until Ctrl+C. Three verbs carry it, and only the
// first is the person's: the others are the client's own.
const (
	addonsDev     = "dev"      // addons dev <folder>: start (the person's verb)
	addonsDevLog  = "dev-log"  // addons dev-log <token> <next>: the lines since
	addonsDevStop = "dev-stop" // addons dev-stop <token>: end the run
)

// Timings of `addons dev`.
const (
	// devAskTimeout bounds the start: Sona waits this long for the person to
	// confirm (a little longer than the wait Sona itself keeps).
	devAskTimeout = 75 * time.Second
	// MaxDevFolder bounds the folder's length.
	MaxDevFolder = 4096
	// DevLogBatch is the most lines one poll returns.
	DevLogBatch = 500
)

// devPollEvery is how often the client asks for new lines. A run whose
// client stops asking (a killed terminal) is ended by Sona after a lease.
var devPollEvery = 300 * time.Millisecond

// AddonDev is the answer to `addons dev <folder>` once the person confirmed
// and the add-on started: who it is, and the token the client's later requests
// carry (only this client can read the log or end the run).
type AddonDev struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Permissions []string `json:"permissions"`
	Token       string   `json:"token"`
}

// AddonDevLog is the answer to `addons dev-log`: the lines after the ones the
// client has, and where the next poll starts. Ended says the add-on is gone
// and Exit how it left; Lines may still hold its last words.
type AddonDevLog struct {
	Lines []string `json:"lines"`
	Next  int64    `json:"next"`
	Ended bool     `json:"ended"`
	Exit  string   `json:"exit"`
}

// checkAddons validates `addons`'s arguments. The person may send dev and a
// folder; the client's own polls (a request, never a command line) carry a
// token and a line number.
func checkAddons(in *invocation) error {
	verb := in.args[0]
	want := map[string]int{addonsDev: 2}
	if in.fromRequest {
		want = map[string]int{addonsDev: 2, addonsDevLog: 3, addonsDevStop: 2}
	}
	n, ok := want[verb]
	switch {
	case !ok:
		return Usage(msgBadChoice, "addons", verb, addonsDev)
	case len(in.args) < n:
		if verb == addonsDev {
			return Usage(msgMissingArg, "addons", "folder")
		}
		return Usage(msgMissingArg, "addons", "token")
	case len(in.args) > n:
		return Usage(msgTooManyArgs, "addons")
	}
	switch verb {
	case addonsDev:
		dir := in.args[1]
		if dir == "" || len(dir) > MaxDevFolder || strings.ContainsRune(dir, 0) {
			return Usage(msgBadAddonFolder, "addons")
		}
		if in.fromRequest && !filepath.IsAbs(dir) {
			return Usage(msgBadAddonFolder, "addons")
		}
	case addonsDevLog:
		if next, err := strconv.ParseInt(in.args[2], 10, 64); err != nil || next < 0 {
			return Usage(msgBadChoice, "addons", in.args[2], "0, 1, 2…")
		}
	}
	return nil
}

// addonsDev runs `sona addons dev <folder>`.
func (env Env) addonsDev(ctx context.Context, in *invocation, t Transport) int {
	p := env.Printer
	dir, err := filepath.Abs(in.args[1])
	if err != nil {
		return env.fail(Usage(msgBadAddonFolder, "addons"))
	}
	start, cancel := context.WithTimeout(ctx, devAskTimeout)
	raw, err := t.Call(start, Request{Command: "addons", Args: []string{addonsDev, dir}})
	cancel()
	if err != nil {
		return env.unreachable(in, err)
	}
	var d AddonDev
	if err := decode(raw, &d); err != nil {
		return env.fail(err)
	}
	name := env.text(d.Name)
	env.errLine(p.Sprintf(msgAddonsRunning, name, env.text(d.ID), env.text(dir)))
	next := int64(0)
	tick := time.NewTicker(devPollEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			// Ctrl+C: end the run, then say so. The context is gone, so the
			// request has one of its own.
			stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			_, _ = t.Call(stop, Request{Command: "addons", Args: []string{addonsDevStop, d.Token}})
			cancel()
			env.errLine(p.Sprintf(msgAddonsStopped, name))
			return ExitOK
		case <-tick.C:
		}
		poll, cancel := context.WithTimeout(ctx, commandTimeout)
		raw, err := t.Call(poll, Request{Command: "addons", Args: []string{addonsDevLog, d.Token, strconv.FormatInt(next, 10)}})
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				continue // the interrupt: handled above
			}
			return env.unreachable(in, err)
		}
		var l AddonDevLog
		if err := decode(raw, &l); err != nil {
			return env.fail(err)
		}
		for _, line := range l.Lines {
			env.logLine(line)
		}
		next = max(l.Next, next)
		if l.Ended {
			env.errLine(p.Sprintf(msgAddonsEnded, name, env.text(l.Exit)))
			return ExitOK
		}
	}
}

// text is a word of Sona's or of an add-on's for the terminal: stripped of
// escape and control sequences when stdout is one.
func (env Env) text(s string) string {
	if env.Terminal {
		return TerminalSafe(s)
	}
	return s
}

func (env Env) logLine(line string) {
	_, _ = env.Stdout.Write([]byte(env.text(line) + "\n"))
}

func (env Env) errLine(line string) {
	_, _ = env.Stderr.Write([]byte(line + "\n"))
}

// TerminalSafe is s as one line a terminal can't be driven by: escape
// sequences (CSI, OSC, DCS and the other introducers, in their 7-bit and
// 8-bit forms) are removed whole, other control characters, bidirectional and
// format characters, and line breaks become a space, invalid UTF-8 a
// replacement character.
func TerminalSafe(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n <= 1:
			b.WriteRune('\uFFFD')
			i++
		case r == 0x1b:
			i = skipEscape(s, i+1)
		case r == 0x9b: // CSI
			i = skipCSI(s, i+n)
		case r == 0x9d || r == 0x90 || r == 0x98 || r == 0x9e || r == 0x9f: // OSC, DCS, SOS, PM, APC
			i = skipString(s, i+n)
		case r == '\t' || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.In(r, unicode.Zl, unicode.Zp):
			b.WriteByte(' ')
			i += n
		default:
			b.WriteRune(r)
			i += n
		}
	}
	return b.String()
}

// skipEscape returns where text resumes after the escape sequence whose
// introducer (the byte after ESC) is at i.
func skipEscape(s string, i int) int {
	if i >= len(s) {
		return i
	}
	switch s[i] {
	case '[':
		return skipCSI(s, i+1)
	case ']', 'P', 'X', '^', '_':
		return skipString(s, i+1)
	}
	// Other sequences are ESC, intermediate bytes (0x20–0x2f), one final byte.
	for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
		i++
	}
	if i < len(s) {
		i++
	}
	return i
}

// skipCSI skips parameter and intermediate bytes and the final byte.
func skipCSI(s string, i int) int {
	for i < len(s) && s[i] >= 0x20 && s[i] <= 0x3f {
		i++
	}
	if i < len(s) && s[i] >= 0x40 && s[i] <= 0x7e {
		i++
	}
	return i
}

// skipString skips a string sequence to its terminator: BEL, ST (ESC \) or
// the 8-bit ST. An unterminated one ends at the end of the line: a terminal
// would swallow what follows it.
func skipString(s string, i int) int {
	for i < len(s) {
		switch {
		case s[i] == 0x07:
			return i + 1
		case s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\':
			return i + 2
		case strings.HasPrefix(s[i:], "\u009c"):
			return i + len("\u009c")
		}
		i++
	}
	return i
}

var errDevNeedsToken = errors.New("addons dev: no token")

// DevRequest decodes the arguments of a request of `addons` for the running
// Sona. Verb is addonsDev, addonsDevLog or addonsDevStop (checkRequest has
// already validated them).
type DevRequest struct {
	Verb  string
	Dir   string // dev
	Token string // dev-log and dev-stop
	Next  int64  // dev-log
}

// ParseDevRequest reads a validated `addons` request.
func ParseDevRequest(req Request) (DevRequest, error) {
	if len(req.Args) == 0 {
		return DevRequest{}, errDevNeedsToken
	}
	r := DevRequest{Verb: req.Args[0]}
	switch r.Verb {
	case addonsDev:
		r.Dir = req.Args[1]
	case addonsDevLog:
		r.Token = req.Args[1]
		r.Next, _ = strconv.ParseInt(req.Args[2], 10, 64)
	case addonsDevStop:
		r.Token = req.Args[1]
	default:
		return DevRequest{}, &Error{Code: CodeUnknownCommand, Message: req.Command}
	}
	return r, nil
}

// Verbs of ParseDevRequest's result, for the running Sona's switch.
const (
	DevStart = addonsDev
	DevLog   = addonsDevLog
	DevStop  = addonsDevStop
)
