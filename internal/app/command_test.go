package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/coredev-uk/sona-go/internal/app/dbustest"
	"github.com/coredev-uk/sona-go/internal/cli"
)

// fakeServer answers commands like the running Sona would and follows a
// feed of statuses.
type fakeServer struct {
	feed  *cli.Feed
	calls chan cli.Request
}

func newFakeServer() *fakeServer {
	f := &fakeServer{calls: make(chan cli.Request, 8)}
	f.feed = cli.NewFeed(func() cli.Status { return cli.Status{Running: true, State: cli.StateIdle, Repeat: "off"} })
	return f
}

func (f *fakeServer) Call(_ context.Context, req cli.Request) (any, error) {
	f.calls <- req
	switch req.Command {
	case "status":
		return cli.Status{Running: true, State: cli.StateIdle, Repeat: "off"}, nil
	case "private":
		return cli.Private{On: len(req.Args) > 0 && req.Args[0] == "on", Source: cli.SourceSetting}, nil
	case "like":
		return nil, &cli.Error{Code: cli.CodeNothingPlaying}
	case "next":
		return nil, errors.New("engine exploded")
	}
	return cli.Status{Running: true, State: cli.StatePlaying}, nil
}

func (f *fakeServer) Watch() (<-chan cli.Status, func()) { return f.feed.Watch() }

// commandContract checks what every transport must do once an instance
// is up but before HandleCommands: commands report that Sona is
// starting; then, served by srv, they reach it with their arguments and
// carry its results and error codes back.
func commandContract(t *testing.T, client cli.Transport, handle func(cli.Server)) *fakeServer {
	t.Helper()
	ctx := context.Background()
	var cerr *cli.Error
	if _, err := client.Call(ctx, cli.Request{Command: "status"}); !errors.As(err, &cerr) || cerr.Code != cli.CodeStarting {
		t.Fatalf("before HandleCommands: err = %v, want starting", err)
	}
	srv := newFakeServer()
	handle(srv)

	raw, err := client.Call(ctx, cli.Request{Command: "private", Args: []string{"on"}})
	if err != nil {
		t.Fatal(err)
	}
	var p cli.Private
	if err := json.Unmarshal(raw, &p); err != nil || !p.On || p.Source != cli.SourceSetting {
		t.Errorf("private on = %s, %v", raw, err)
	}
	if got := <-srv.calls; !reflect.DeepEqual(got, cli.Request{Command: "private", Args: []string{"on"}}) {
		t.Errorf("server got %+v", got)
	}
	for _, tc := range []struct {
		req  cli.Request
		code string
	}{
		{cli.Request{Command: "like"}, cli.CodeNothingPlaying},
		{cli.Request{Command: "next"}, cli.CodeFailed},
		// Checked against the table before the server sees them.
		{cli.Request{Command: "private", Args: []string{"maybe"}}, cli.CodeUsage},
		{cli.Request{Command: "volume", Args: []string{"101"}}, cli.CodeUsage},
		{cli.Request{Command: "help"}, cli.CodeUnknownCommand},
		{cli.Request{Command: "dance"}, cli.CodeUnknownCommand},
	} {
		_, err := client.Call(ctx, tc.req)
		if !errors.As(err, &cerr) || cerr.Code != tc.code {
			t.Errorf("%+v: err = %v, want code %s", tc.req, err, tc.code)
		}
	}
	for range 2 { // like and next reached the server; the rest didn't
		<-srv.calls
	}
	select {
	case req := <-srv.calls:
		t.Errorf("an invalid request reached the server: %+v", req)
	default:
	}
	return srv
}

// followUntilQuit runs client.Follow, publishes a change and a
// position-only change on srv (a paused song's position nudged well inside
// the feed's seek jump, so wall-clock timing can't make it a seek), quits
// Sona with quit and checks that the stream saw the current status, the
// changes (not the position) and then ended cleanly.
func followUntilQuit(t *testing.T, client cli.Transport, srv *fakeServer, quit func()) {
	t.Helper()
	got := make(chan cli.Status, 8)
	done := make(chan error, 1)
	go func() {
		done <- client.Follow(context.Background(), func(s cli.Status) error {
			got <- s
			return nil
		})
	}()
	wait := func() cli.Status {
		t.Helper()
		select {
		case s := <-got:
			return s
		case <-time.After(5 * time.Second):
			t.Fatal("no status")
		}
		return cli.Status{}
	}
	if s := wait(); s.State != cli.StateIdle {
		t.Errorf("first status = %+v, want the current one", s)
	}
	playing := cli.Status{Running: true, State: cli.StatePlaying, Track: &cli.Track{ID: "1", Title: "Song"}, Volume: 40}
	// Both transports subscribe before they deliver the first status.
	srv.feed.Publish(playing)
	if s := wait(); s.State != cli.StatePlaying || s.Track == nil || s.Track.Title != "Song" || s.Volume != 40 {
		t.Errorf("change = %+v", s)
	}
	paused := playing
	paused.State = cli.StatePaused
	srv.feed.Publish(paused)
	if s := wait(); s.State != cli.StatePaused {
		t.Errorf("pause = %+v", s)
	}
	moved := paused
	moved.Position = 0.25
	srv.feed.Publish(moved)
	quit()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Follow after quit = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Follow did not end when Sona quit")
	}
	close(got)
	for s := range got {
		if s.Position == moved.Position {
			t.Errorf("a position-only change was streamed: %+v", s)
		}
	}
}

func TestBusCommands(t *testing.T) {
	addr := dbustest.Start(t)
	ctx := context.Background()
	client := &BusClient{conn: dbustest.Conn(t, addr)}

	if _, err := client.Call(ctx, cli.Request{Command: "status"}); !errors.Is(err, cli.ErrNotRunning) {
		t.Fatalf("no instance: err = %v, want ErrNotRunning", err)
	}
	if err := client.Follow(ctx, func(cli.Status) error { return nil }); !errors.Is(err, cli.ErrNotRunning) {
		t.Fatalf("follow, no instance: err = %v, want ErrNotRunning", err)
	}

	primary, err := Acquire(ctx, dbustest.Conn(t, addr), Activation{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = primary.Close() })
	srv := commandContract(t, client, primary.HandleCommands)

	// A command is not an activation.
	select {
	case a := <-primary.Activations():
		t.Errorf("command delivered as an activation: %+v", a)
	default:
	}

	followUntilQuit(t, client, srv, func() {
		if err := primary.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := client.Call(ctx, cli.Request{Command: "status"}); !errors.Is(err, cli.ErrNotRunning) {
		t.Errorf("after Close: err = %v, want ErrNotRunning", err)
	}
}

// An owner of the name without the Run method (an older Sona) is another
// version, not "not running".
func TestBusCommandsIncompatible(t *testing.T) {
	addr := dbustest.Start(t)
	squatter := dbustest.Conn(t, addr)
	if _, err := squatter.RequestName(BusName, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := (&BusClient{conn: dbustest.Conn(t, addr)}).Call(ctx, cli.Request{Command: "status"})
	var cerr *cli.Error
	if !errors.As(err, &cerr) || cerr.Code != cli.CodeIncompatible {
		t.Errorf("err = %v, want incompatible", err)
	}
}
