package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"text/template"
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
}

// Run runs a command line whose first argument is a command (IsCommand)
// and returns the exit status. Commands for the running Sona go through
// env.Dial; help and completion run here.
func Run(ctx context.Context, argv []string, env Env) int {
	in, err := parse(argv)
	if err != nil {
		return env.fail(err)
	}
	if in.help {
		return env.done(writeHelp(env.Stdout, in.cmd))
	}
	switch in.cmd.name {
	case "help":
		switch {
		case in.man:
			return env.done(writeMan(env.Stdout, env.Version))
		case len(in.args) == 1:
			return env.done(writeHelp(env.Stdout, lookup(in.args[0])))
		}
		return env.done(writeOverview(env.Stdout))
	case "completion":
		return env.done(writeCompletion(env.Stdout, in.args[0]))
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
	w := env.Stdout
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
		return writeQueue(w, q)
	case "search":
		var r SearchResults
		if err := decode(raw, &r); err != nil {
			return err
		}
		return writeSearch(w, r)
	case "open":
		var o Opened
		if err := decode(raw, &o); err != nil {
			return err
		}
		return writeLine(w, fmt.Sprintf(msgOpened, o.Kind, o.ID))
	case "volume":
		var v Volume
		if err := decode(raw, &v); err != nil {
			return err
		}
		return writeLine(w, msgFieldVolume+": "+fmt.Sprintf(msgPercent, v.Volume))
	case "like":
		var l Like
		if err := decode(raw, &l); err != nil {
			return err
		}
		return writeLine(w, likeLine(l))
	case "private":
		var p Private
		if err := decode(raw, &p); err != nil {
			return err
		}
		return writeLine(w, privateLine(p))
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
		return writeStatus(env.Stdout, s)
	}
	return writeLine(env.Stdout, summary(s))
}

// follow runs `status --follow`: a line now and one per change until Sona
// quits, then a last line saying it isn't running.
func (env Env) follow(ctx context.Context, in *invocation, t Transport) int {
	var tmpl *template.Template
	if in.format != "" {
		var err error
		if tmpl, err = statusTemplate(in.format); err != nil {
			return env.fail(Usage(msgBadTemplate, err))
		}
	}
	var last *Status
	err := t.Follow(ctx, func(s Status) error {
		if last != nil && s.sameAs(*last) {
			return nil
		}
		last = &s
		if tmpl != nil {
			return executeTemplate(env.Stdout, tmpl, s)
		}
		return env.writeStatus(in, s)
	})
	if err != nil {
		var cmdErr *Error
		if !errors.Is(err, ErrNotRunning) && !errors.As(err, &cmdErr) && last != nil {
			err = fmt.Errorf(msgFollowEnded, err)
		}
		return env.unreachable(in, err)
	}
	if err := env.writeStatus(in, NotRunning()); err != nil {
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
		env.errorf("%s", msgNotRunning)
	default:
		env.errorf(msgUnreachable, err)
	}
	return ExitNotRunning
}

// fail prints err and returns its exit status.
func (env Env) fail(err error) int {
	var e *Error
	if !errors.As(err, &e) {
		env.errorf("%v", err)
		return ExitFailed
	}
	env.errorf("%s", errorText(e))
	switch e.Code {
	case CodeUsage, CodeBadLink:
		return ExitUsage
	case CodeStarting, CodeShuttingDown:
		return ExitNotRunning
	}
	return ExitFailed
}

// errorText is the message for a failed command: the client's own for the
// codes it knows, Sona's otherwise.
func errorText(e *Error) string {
	switch e.Code {
	case CodeUsage:
		if e.Message == "" {
			return msgExitUsage
		}
	case CodeBadLink:
		return msgBadLink
	case CodeStarting:
		return msgStarting
	case CodeShuttingDown:
		return msgShuttingDown
	case CodeNothingPlaying:
		return msgNothingPlaying
	case CodeUnknownCommand, CodeIncompatible:
		return msgIncompatible
	case "not_signed_in":
		return msgNotSignedIn
	case "private_forced":
		return msgPrivateForced
	case "unsupported":
		return msgUnsupportedOnOS
	}
	return e.Error()
}

func (env Env) errorf(format string, args ...any) {
	_, _ = fmt.Fprintf(env.Stderr, msgErrorPrefix+format+"\n", args...)
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
