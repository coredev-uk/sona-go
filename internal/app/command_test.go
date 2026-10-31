package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/app/dbustest"
)

func TestIsCommand(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"private", "on"}, true},
		{[]string{"private"}, true},
		{nil, false},
		{[]string{"--hidden"}, false},
		{[]string{"music://music.apple.com/us/album/x/1"}, false},
		{[]string{"--hidden", "private"}, false}, // only the first argument names a command
	} {
		if got := IsCommand(tc.args); got != tc.want {
			t.Errorf("IsCommand(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

// A command reaches the running instance's handler with its arguments, and
// its reply or error comes back; without a running instance it fails with
// ErrNotRunning.
func TestCommandRoundTrip(t *testing.T) {
	addr := dbustest.Start(t)
	ctx := context.Background()
	client := dbustest.Conn(t, addr)

	if _, err := SendCommand(ctx, client, []string{"private", "status"}); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("no instance: err = %v, want ErrNotRunning", err)
	}

	primary, err := Acquire(ctx, dbustest.Conn(t, addr), Activation{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = primary.Close() })

	// Before the handlers are installed (start-up), commands fail plainly.
	if _, err := SendCommand(ctx, client, []string{"private"}); err == nil || !strings.Contains(err.Error(), "starting") {
		t.Fatalf("before HandleCommands: err = %v", err)
	}

	calls := make(chan []string, 4)
	primary.HandleCommands(Commands{"private": func(args []string) (string, error) {
		calls <- args
		if len(args) > 0 && args[0] == "bad" {
			return "", errors.New("usage: sona private [on|off|toggle|status]")
		}
		return "private listening: on", nil
	}})
	out, err := SendCommand(ctx, client, []string{"private", "on"})
	if err != nil || out != "private listening: on" {
		t.Fatalf("reply = %q, %v", out, err)
	}
	if got := <-calls; !reflect.DeepEqual(got, []string{"on"}) {
		t.Errorf("handler args = %q", got)
	}
	if _, err := SendCommand(ctx, client, []string{"private", "bad"}); err == nil || err.Error() != "usage: sona private [on|off|toggle|status]" {
		t.Errorf("failing command: err = %v", err)
	}
	if _, err := SendCommand(ctx, client, []string{"nope"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("unknown command: err = %v", err)
	}

	// A command is not an activation: nothing is forwarded as a launch.
	select {
	case a := <-primary.Activations():
		t.Errorf("command delivered as an activation: %+v", a)
	default:
	}

	if err := primary.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := SendCommand(ctx, client, []string{"private"}); !errors.Is(err, ErrNotRunning) {
		t.Errorf("after Close: err = %v, want ErrNotRunning", err)
	}
}
