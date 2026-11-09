//go:build linux || (cgo && (darwin || windows))

package native

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// The test binary doubles as the playback host child (TestPreviewPlaysLive).
func TestMain(m *testing.M) {
	cdmhost.ChildMain()
	os.Exit(m.Run())
}

// envPreviewRequire, when set, fails TestPreviewPlaysLive instead of
// skipping it when Apple can't be reached (CI, which must run it).
const envPreviewRequire = "SONA_PREVIEW_REQUIRE"

// A real song's preview plays end to end with the developer token alone:
// the web player's token, a catalog search, the clip from Apple's CDN,
// and a real host child that loads no CDM, playing into a fakesink. It
// advances, plays out at its end, and nothing asked for a licence. It
// skips only when Apple can't be reached.
func TestPreviewPlaysLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	hc := &http.Client{Timeout: 30 * time.Second}
	dev := devtoken.NewWebPlayer(hc)
	if _, err := dev.Token(ctx); err != nil {
		var ne net.Error
		if errors.As(err, &ne) && os.Getenv(envPreviewRequire) == "" {
			t.Skipf("offline: Apple's web player can't be reached (%s=1 fails instead): %v", envPreviewRequire, err)
		}
		t.Fatalf("developer token: %v", err)
	}
	api := applemusic.New(hc, dev, func() string { return "" })
	const sf = "us"
	songs, err := api.SearchSongs(ctx, sf, "Radiohead Weird Fishes", 5)
	if err != nil {
		t.Fatalf("catalog search with the developer token only: %v", err)
	}
	var id string
	for _, s := range songs {
		if len(s.Attributes.Previews) > 0 && s.Attributes.Previews[0].URL != "" {
			id = s.ID
			break
		}
	}
	if id == "" {
		t.Fatalf("no song with a preview among %d hits", len(songs))
	}

	e := New(Options{
		Client: api, IdleTimeout: -1, Preview: true, Storefront: func() string { return sf },
		Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	})
	e.spawn = func(ctx context.Context, lib string) (host, error) {
		if lib != "" {
			t.Errorf("preview host asked for CDM %s", lib)
		}
		return cdmhost.Start(ctx, cdmhost.Options{Clear: true, Sink: "fakesink"})
	}
	defer func() { _ = e.Close() }()

	if err := e.Play(ctx, id, 0); err != nil {
		t.Fatalf("Play(%s): %v", id, err)
	}
	info, ok := e.Host()
	if !ok || info.CDMPath != "" || info.CDMInterface != 0 {
		t.Fatalf("host %+v (running %v), want one without a CDM", info, ok)
	}
	st := e.Status()
	if st.State != playback.StatePlaying || !st.Preview {
		t.Fatalf("status %+v, want playing a preview", st)
	}
	t0 := time.Now()
	from := st.Position + t0.Sub(st.UpdatedAt)
	time.Sleep(3 * time.Second)
	st = e.Status()
	at := st.Position + time.Since(st.UpdatedAt)
	if st.State != playback.StatePlaying || at-from < 2*time.Second {
		t.Fatalf("after 3 s: %s at %v (from %v), want it advancing", st.State, at, from)
	}
	dur := st.Duration
	if dur <= 0 || dur > 45*time.Second {
		t.Fatalf("duration %v: not a preview", dur)
	}
	t.Logf("preview %s: %v long, advanced %v in 3 s; host pid %d, decoder %s, sandbox %s", id, dur, at-from, info.PID, info.Decoder, info.Sandbox.Kind())

	// The end: it plays out (the coordinator then starts the next item).
	if err := e.Seek(ctx, dur-2*time.Second); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for e.Status().State != playback.StateEnded {
		if time.Now().After(deadline) {
			t.Fatalf("not ended: %+v", e.Status())
		}
		time.Sleep(50 * time.Millisecond)
	}
	e.mu.Lock()
	sessions := len(e.sessions)
	e.mu.Unlock()
	e.certMu.Lock()
	cert := e.cert
	e.certMu.Unlock()
	if sessions != 0 || cert != nil {
		t.Errorf("%d CDM sessions, certificate fetched %v", sessions, cert != nil)
	}
}
