package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
)

// Exit statuses of `sona <command>`.
const (
	ExitOK = 0
	// ExitFailed: the command failed in the running Sona.
	ExitFailed = 1
	// ExitUsage: the command line is wrong.
	ExitUsage = 2
	// ExitNotRunning: no Sona is running, or it can't be reached.
	ExitNotRunning = 3
)

// Env is what Run needs from the process.
type Env struct {
	Stdout, Stderr io.Writer
	// Dial connects to the running Sona; it fails with ErrNotRunning when
	// none runs.
	Dial func() (Transport, error)
	// Version is Sona's version, for the manual page.
	Version string
	// Printer words help, the manual page, completions' descriptions,
	// printed lines and errors (nil: English). JSON output is the same in
	// every language.
	Printer *l10n.Printer
}

// Run runs a command line whose first argument is a command (IsCommand)
// and returns the exit status. Commands for the running Sona go through
// env.Dial; help and completion run here.
func Run(ctx context.Context, argv []string, env Env) int {
	in, err := parse(argv)
	if err != nil {
		return env.fail(err)
	}
	p := env.Printer
	if in.help {
		return env.done(writeHelp(env.Stdout, p, in.cmd))
	}
	switch in.cmd.name {
	case "help":
		switch {
		case in.man:
			return env.done(writeMan(env.Stdout, p, env.Version))
		case len(in.args) == 1:
			return env.done(writeHelp(env.Stdout, p, lookup(in.args[0])))
		}
		return env.done(writeOverview(env.Stdout, p))
	case "completion":
		return env.done(writeCompletion(env.Stdout, p, in.args[0]))
	}

	t, err := env.Dial()
	if err != nil {
		return env.unreachable(in, err)
	}
	defer func() { _ = t.Close() }()
	if in.follow {
		return env.follow(ctx, in, t)
	}
	cctx, cancel := context.WithTimeout(ctx, in.cmd.roundTrip())
	defer cancel()
	raw, err := t.Call(cctx, in.request())
	if err != nil {
		return env.unreachable(in, err)
	}
	if err := env.print(in, raw); err != nil {
		return env.fail(err)
	}
	return ExitOK
}

// print writes a command's result: the JSON as received with --json,
// else its text.
func (env Env) print(in *invocation, raw json.RawMessage) error {
	if in.json {
		_, err := fmt.Fprintf(env.Stdout, "%s\n", raw)
		return err
	}
	w, p := env.Stdout, env.Printer
	switch in.cmd.name {
	case "status", "play", "pause", "toggle", "next", "prev":
		var s Status
		if err := decode(raw, &s); err != nil {
			return err
		}
		return env.writeStatus(in, s)
	case "queue":
		var q Queue
		if err := decode(raw, &q); err != nil {
			return err
		}
		return writeQueue(w, p, q)
	case "search":
		var r SearchResults
		if err := decode(raw, &r); err != nil {
			return err
		}
		return writeSearch(w, p, r)
	case "open":
		var o Opened
		if err := decode(raw, &o); err != nil {
			return err
		}
		return writeLine(w, p.Sprintf(msgOpened, o.Kind, o.ID))
	case "volume":
		var v Volume
		if err := decode(raw, &v); err != nil {
			return err
		}
		return writeLine(w, p.Sprintf(msgFieldLine, p.T(msgFieldVolume), p.Sprintf(msgPercent, v.Volume)))
	case "like":
		var l Like
		if err := decode(raw, &l); err != nil {
			return err
		}
		return writeLine(w, likeLine(p, l))
	case "private":
		var pv Private
		if err := decode(raw, &pv); err != nil {
			return err
		}
		return writeLine(w, privateLine(p, pv))
	}
	return writeLine(w, string(raw))
}

// writeStatus prints a status the way the invocation asks: JSON, through
// its template, all fields (`status`) or one line (the others, --follow).
func (env Env) writeStatus(in *invocation, s Status) error {
	switch {
	case in.json:
		b, err := json.Marshal(s)
		if err != nil {
			return err
		}
		return writeLine(env.Stdout, string(b))
	case in.format != "":
		t, err := statusTemplate(in.format)
		if err != nil {
			return err
		}
		return executeTemplate(env.Stdout, t, s)
	case in.cmd.name == "status" && !in.follow:
		return writeStatus(env.Stdout, env.Printer, s)
	}
	return writeLine(env.Stdout, summary(env.Printer, s))
}

// follow runs `status --follow`: a line now and one per change until Sona
// quits, then a last line saying it isn't running.
// Lines identical to the one before (a change the line doesn't show, such
// as the volume in the default text) are left out; --json prints each.
func (env Env) follow(ctx context.Context, in *invocation, t Transport) int {
	var (
		last    *Status
		printed []byte
	)
	emit := func(s Status) error {
		var b bytes.Buffer
		line := env
		line.Stdout = &b
		if err := line.writeStatus(in, s); err != nil {
			return err
		}
		if bytes.Equal(b.Bytes(), printed) {
			return nil
		}
		printed = b.Bytes()
		_, err := env.Stdout.Write(printed)
		return err
	}
	err := t.Follow(ctx, func(s Status) error {
		if last != nil && s.sameAs(*last) {
			return nil
		}
		last = &s
		return emit(s)
	})
	if err != nil {
		var cmdErr *Error
		if !errors.Is(err, ErrNotRunning) && !errors.As(err, &cmdErr) && last != nil {
			err = errors.New(env.Printer.Sprintf(msgFollowEnded, err))
		}
		return env.unreachable(in, err)
	}
	if err := emit(NotRunning()); err != nil {
		return env.fail(err)
	}
	return ExitOK
}

// unreachable reports a failed round trip: no Sona (status prints that it
// isn't running), a command that failed there, or a broken transport.
func (env Env) unreachable(in *invocation, err error) int {
	var cmdErr *Error
	switch {
	case errors.As(err, &cmdErr):
		return env.fail(err)
	case errors.Is(err, ErrNotRunning) && in.cmd.name == "status":
		if werr := env.writeStatus(in, NotRunning()); werr != nil {
			return env.fail(werr)
		}
		return ExitNotRunning
	case errors.Is(err, ErrNotRunning):
		env.errorf(msgNotRunning)
	default:
		env.errorf(msgUnreachable, err)
	}
	return ExitNotRunning
}

// fail prints err and returns its exit status.
func (env Env) fail(err error) int {
	var e *Error
	if !errors.As(err, &e) {
		env.write(err.Error())
		return ExitFailed
	}
	env.write(errorText(env.Printer, e))
	switch e.Code {
	case CodeUsage, CodeBadLink:
		return ExitUsage
	case CodeStarting, CodeShuttingDown:
		return ExitNotRunning
	}
	return ExitFailed
}

// errorText is the message for a failed command: the client's own for
// the codes it knows, else Sona's (in English) after the client's words.
func errorText(p *l10n.Printer, e *Error) string {
	if e.format != "" {
		return p.Sprintf(e.format, e.args...)
	}
	switch e.Code {
	case CodeUsage:
		if e.Message == "" {
			return p.T(msgExitUsage)
		}
		return e.Message
	case CodeBadLink:
		return p.T(msgBadLink)
	case CodeStarting:
		return p.T(msgStarting)
	case CodeShuttingDown:
		return p.T(msgShuttingDown)
	case CodeNothingPlaying:
		return p.T(msgNothingPlaying)
	case CodeAccountLoading:
		return p.T(msgAccountLoading)
	case CodeUnknownCommand, CodeIncompatible:
		return p.T(msgIncompatible)
	}
	// Services' codes (internal/services/errors.go) the commands can fail
	// with.
	if m, ok := serviceErrors[e.Code]; ok {
		return p.T(m)
	}
	return p.Sprintf(msgFailed, e.Error())
}

// serviceErrors words the running Sona's error codes
// (internal/services/errors.go) that commands can fail with.
var serviceErrors = map[string]l10n.Message{
	"not_signed_in":   msgNotSignedIn,
	"unauthorized":    msgNotSignedIn,
	"mut_rejected":    msgSignInAgain,
	"private_forced":  msgPrivateForced,
	"unsupported":     msgUnsupportedOnOS,
	"network":         msgNetwork,
	"timeout":         msgTimeout,
	"not_found":       msgNotFound,
	"not_playable":    msgNotPlayable,
	"not_released":    msgNotReleased,
	"no_subscription": msgNoSubscription,
	"no_cdm":          msgNoCDM,
	"keyring":         msgKeyring,
}

// errorf prints an error message, "sona: " first.
func (env Env) errorf(format l10n.Message, args ...any) {
	env.write(env.Printer.Sprintf(format, args...))
}

func (env Env) write(msg string) {
	_, _ = fmt.Fprintln(env.Stderr, errorPrefix+msg)
}

// done is the exit status after printing locally.
func (env Env) done(err error) int {
	if err != nil {
		return env.fail(err)
	}
	return ExitOK
}

func decode(raw json.RawMessage, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return &Error{Code: CodeIncompatible, Message: err.Error()}
	}
	return nil
}

func writeLine(w io.Writer, s string) error {
	_, err := fmt.Fprintln(w, s)
	return err
}
