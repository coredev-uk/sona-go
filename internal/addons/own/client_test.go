package own

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
)

// host is the supervisor's end of the channel: a real cli.Session over a
// pipe pair, so the client is tested against the protocol as Sona speaks it.
type host struct {
	sess *cli.Session
	cli  *Client
	done chan error // the session's Serve
	// sonaOut is the session's output: closing it is the end of the add-on's
	// input.
	sonaOut *io.PipeWriter
}

type fakeHost struct{}

func (fakeHost) Call(context.Context, cli.Request) (any, error) {
	return nil, &cli.Error{Code: "failed"}
}

func (fakeHost) Watch(string) (<-chan any, func(), error) {
	return nil, nil, &cli.Error{Code: cli.AddonErrUnknownTopic}
}

type fakeOwn struct{ secrets map[string]string }

func (fakeOwn) WatchSettings() (<-chan any, func()) { return make(chan any), func() {} }
func (o fakeOwn) Secret(key string) (string, bool) {
	v, ok := o.secrets[key]
	return v, ok
}

func newHost(t *testing.T, id string, perms []string, own cli.AddonOwn) *host {
	t.Helper()
	toSona, fromAddon := io.Pipe() // the add-on writes, the session reads
	toAddon, fromSona := io.Pipe() // the session writes, the add-on reads
	sess := cli.NewSession(cli.AddonConfig{
		ID: id, Host: fakeHost{}, Own: own,
		Allowed: func(p string) bool { return p == cli.PermPlaybackRead },
	})
	h := &host{sess: sess, cli: NewClient(id, toAddon, fromAddon, nil), done: make(chan error, 1), sonaOut: fromSona}
	go func() { h.done <- sess.Serve(context.Background(), rwc{toSona, fromSona}) }()
	t.Cleanup(func() {
		sess.Close()
		_ = fromAddon.Close()
		_ = fromSona.Close()
	})
	return h
}

type rwc struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func (c rwc) Read(b []byte) (int, error)  { return c.r.Read(b) }
func (c rwc) Write(b []byte) (int, error) { return c.w.Write(b) }
func (c rwc) Close() error                { _ = c.r.Close(); return c.w.Close() }

// settled waits until the client has taken Sona's answer to hello: an invoke
// is answered only by a client that is reading. (A session closed between
// accepting a hello and queueing its answer never sends it.)
func settled(t *testing.T, sess *cli.Session) {
	t.Helper()
	select {
	case <-sess.Helloed():
	case <-time.After(10 * time.Second):
		t.Fatal("no hello")
	}
	_, _ = sess.Invoke(context.Background(), "addon.x.sync", nil)
}

func serve(h *host, perms []string, setup Setup) chan error {
	out := make(chan error, 1)
	go func() { out <- h.cli.Serve(context.Background(), perms, setup) }()
	return out
}

func wait(t *testing.T, ch chan error, what string) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return nil
	}
}

// The client says hello with the manifest's permissions, gets the grant
// back, and stops cleanly when Sona sends the shutdown event.
func TestClientHelloAndShutdown(t *testing.T) {
	h := newHost(t, "lastfm", nil, nil)
	var got cli.HelloResult
	out := serve(h, []string{cli.PermPlaybackRead, cli.PermLike}, func(_ context.Context, _ *Client, hello cli.HelloResult) error {
		got = hello
		return nil
	})
	settled(t, h.sess)
	h.sess.Close()
	if err := wait(t, out, "Serve after shutdown"); err != nil {
		t.Fatalf("Serve = %v, want a clean stop", err)
	}
	if got.ID != "lastfm" || got.Protocol != "2.0.0" || len(got.Permissions) != 1 || got.Permissions[0] != cli.PermPlaybackRead {
		t.Errorf("hello answer = %+v", got)
	}
}

// The end of the channel is a clean stop too: an add-on exits when its
// input ends.
func TestClientStopsWhenTheChannelEnds(t *testing.T) {
	h := newHost(t, "lastfm", nil, nil)
	out := serve(h, nil, nil)
	settled(t, h.sess)
	_ = h.sonaOut.Close()
	if err := wait(t, out, "Serve"); err != nil {
		t.Errorf("Serve = %v, want a clean stop", err)
	}
}

// A hello Sona refuses ends the add-on with the refusal.
func TestClientHelloRefused(t *testing.T) {
	h := newHost(t, "other", nil, nil)
	h.cli.id = "lastfm" // the client names an add-on the session wasn't started as
	err := wait(t, serve(h, nil, nil), "Serve")
	var ce *cli.Error
	if !errors.As(err, &ce) || ce.Code != cli.AddonErrBadHello {
		t.Errorf("Serve = %v, want bad_hello", err)
	}
}

// Sona's invoke reaches a handler, and a method nobody handles is
// unknown_method.
func TestClientAnswersSonaRequests(t *testing.T) {
	h := newHost(t, "lastfm", nil, nil)
	h.cli.Handle(cli.AddonMethodInvoke, func(_ context.Context, params json.RawMessage) (any, error) {
		var p cli.InvokeParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if p.Action == "addon.lastfm.boom" {
			return nil, errors.New("secret detail: https://token@example")
		}
		return cli.InvokeResult{Toast: "hello " + p.Action}, nil
	})
	out := serve(h, nil, nil)
	settled(t, h.sess)
	res, err := h.sess.Invoke(context.Background(), "addon.lastfm.x", nil)
	if err != nil || res.Toast != "hello addon.lastfm.x" {
		t.Errorf("invoke = %+v, %v", res, err)
	}
	// A failing handler is "failed", and nothing of its text crosses.
	_, err = h.sess.Invoke(context.Background(), "addon.lastfm.boom", nil)
	var ce *cli.Error
	if !errors.As(err, &ce) || ce.Code != cli.AddonErrFailed || ce.Message != "Sona couldn't do that" {
		t.Errorf("failing handler = %v", err)
	}
	h.sess.Close()
	_ = wait(t, out, "Serve")

	h2 := newHost(t, "lastfm", nil, nil)
	out2 := serve(h2, nil, nil)
	settled(t, h2.sess)
	if _, err := h2.sess.Invoke(context.Background(), "addon.lastfm.x", nil); !errors.As(err, &ce) || ce.Code != cli.AddonErrUnknownMethod {
		t.Errorf("unhandled = %v, want unknown_method", err)
	}
	h2.sess.Close()
	_ = wait(t, out2, "Serve")
}

// Secret returns what Sona holds under the add-on's ID and reports an empty
// slot as not found.
func TestClientSecret(t *testing.T) {
	h := newHost(t, "lastfm", nil, fakeOwn{secrets: map[string]string{"session": "sk-1"}})
	var (
		v              string
		found, missing bool
		err            error
	)
	ready := make(chan struct{})
	out := serve(h, nil, func(ctx context.Context, c *Client, _ cli.HelloResult) error {
		defer close(ready)
		if v, found, err = c.Secret(ctx, "session"); err != nil {
			return nil
		}
		_, ok, _ := c.Secret(ctx, "token")
		missing = !ok
		return nil
	})
	<-ready
	h.sess.Close()
	if e := wait(t, out, "Serve"); e != nil {
		t.Fatalf("Serve = %v", e)
	}
	if err != nil || !found || v != "sk-1" || !missing {
		t.Errorf("secret = %q found %v err %v, missing token %v", v, found, err, missing)
	}
}
