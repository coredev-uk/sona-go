package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// resign makes the manifest the key holder would publish for version on
// target over the release's archive.
func (r *release) resign(version, target string) {
	d := sha256.Sum256(r.archive)
	r.manifest.Version = version
	r.manifest.SHA256 = hex.EncodeToString(d[:])
	r.manifest.Signature = Sign(r.priv, d[:])
	r.manifest.StatementSignature = SignStatement(r.priv, target, version, d[:])
}

func (r *release) downloads() int { return r.hits["/archive.tar.gz"] }

// TestStatementFormat pins the bytes cmd/releasesign signs, which installs
// already out there must go on checking the same way.
func TestStatementFormat(t *testing.T) {
	digest := sha256.Sum256([]byte("archive"))
	want := "sona-update-v1\x00darwin-arm64\x000.6.0\x00stable\x00" + hex.EncodeToString(digest[:])
	if got := string(Statement("darwin-arm64", "0.6.0", ChannelStable, digest[:])); got != want {
		t.Fatalf("statement = %q, want %q", got, want)
	}
	// A fixed seed makes the signature a constant, so a change to the format
	// fails here.
	priv := ed25519.NewKeyFromSeed([]byte("sona-update-test-seed-0123456789"))
	const wantSig = "qzLtJzlBqwslst2/JhrUHkmephBu5K6+Na6j00CwlITIhYwwhJDW7eLnU3STjM+TYwJ94EyK345A19iej+x7CA=="
	if got := SignStatement(priv, "darwin-arm64", "0.6.0", digest[:]); got != wantSig {
		t.Errorf("signature = %s", got)
	}
}

func TestChannelOf(t *testing.T) {
	for v, want := range map[string]string{
		"0.6.0": ChannelStable, "v1.2.3": ChannelStable, "1.0.0+build.5": ChannelStable,
		"0.6.0-rc.1": ChannelPrerelease, "1.0.0-beta": ChannelPrerelease,
		"": "", "latest": "", "1.2": "",
	} {
		if got := ChannelOf(v); got != want {
			t.Errorf("ChannelOf(%q) = %q, want %q", v, got, want)
		}
	}
}

// A manifest the key holder signed for this platform and version passes the
// feed and the source, and the archive is installed.
func TestSignedManifestIsAccepted(t *testing.T) {
	r := newRelease(t, "0.3.0", goodSig)
	m, raw, err := r.feed().Manifest(context.Background(), "darwin-arm64", false)
	if err != nil {
		t.Fatal(err)
	}
	s := source(r, r.pub)
	s.Update, s.Raw = m, raw
	body, err := s.Open(context.Background(), m.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = body.Close()
}

// The attack of GHSA-6qp4-86m8-prj5: an older genuine release's manifest,
// replayed. Its statement is real, so the feed reads it, but a version that
// is not newer than the running one is never downloaded.
func TestReplayedOlderManifestIsNotInstalled(t *testing.T) {
	for _, offered := range []string{"0.2.0", "0.1.0", "0.2.0-rc.1"} {
		t.Run(offered, func(t *testing.T) {
			r := newRelease(t, offered, goodSig)
			m, _, err := r.feed().Manifest(context.Background(), "darwin-arm64", true)
			if err != nil {
				t.Fatal(err)
			}
			if Newer(m.Version, "0.3.0") {
				t.Fatalf("%s offered to 0.3.0", m.Version)
			}
			s := source(r, r.pub)
			s.Update, s.Current = m, "0.3.0"
			if _, err := s.Open(context.Background(), m.URL); !errors.Is(err, ErrNotNewer) {
				t.Fatalf("err = %v, want ErrNotNewer", err)
			}
			if r.downloads() != 0 {
				t.Fatal("a refused version was downloaded")
			}
		})
	}
	t.Run("the running version", func(t *testing.T) {
		r := newRelease(t, "0.3.0", goodSig)
		s := source(r, r.pub)
		s.Current = "0.3.0"
		if _, err := s.Open(context.Background(), r.manifest.URL); !errors.Is(err, ErrNotNewer) {
			t.Fatalf("err = %v, want ErrNotNewer", err)
		}
	})
	t.Run("a build without a release version", func(t *testing.T) {
		r := newRelease(t, "0.3.0", goodSig)
		s := source(r, r.pub)
		s.Current = "dev"
		if _, err := s.Open(context.Background(), r.manifest.URL); !errors.Is(err, ErrNotNewer) {
			t.Fatalf("err = %v, want ErrNotNewer", err)
		}
	})
}

// An archive of the other platform, with its own valid signatures, under this
// platform's manifest name.
func TestCrossPlatformManifestIsRefused(t *testing.T) {
	r := newRelease(t, "0.3.0", goodSig)
	r.resign("0.3.0", "windows-amd64")
	if _, _, err := r.feed().Manifest(context.Background(), "darwin-arm64", false); !errors.Is(err, ErrSignature) {
		t.Fatalf("feed: err = %v, want ErrSignature", err)
	}
	if _, err := source(r, r.pub).Open(context.Background(), r.manifest.URL); !errors.Is(err, ErrSignature) {
		t.Fatalf("source: err = %v, want ErrSignature", err)
	}
	if r.downloads() != 0 {
		t.Fatal("the other platform's archive was downloaded")
	}
}

// The version is what the advisory's reproduction changes: the old archive
// and its signatures, under a version of the attacker's choosing.
func TestTamperedManifestIsRefused(t *testing.T) {
	for name, edit := range map[string]func(*release){
		"version":           func(r *release) { r.manifest.Version = "99.0.0" },
		"digest": func(r *release) {
			d := sha256.Sum256([]byte("another archive"))
			r.manifest.SHA256 = hex.EncodeToString(d[:])
		},
		"digest not hex":   func(r *release) { r.manifest.SHA256 = strings.Repeat("z", 64) },
		"digest too short": func(r *release) { r.manifest.SHA256 = r.manifest.SHA256[:32] },
		"statement":        func(r *release) { r.manifest.StatementSignature = r.manifest.Signature },
		"statement garbage": func(r *release) {
			r.manifest.StatementSignature = "!!"
		},
		"statement missing (a manifest from before it existed)": func(r *release) {
			r.manifest.StatementSignature, r.manifest.SHA256 = "", ""
		},
		"signed by another key": func(r *release) {
			_, other, _ := ed25519.GenerateKey(nil)
			d := sha256.Sum256(r.archive)
			r.manifest.StatementSignature = SignStatement(other, "darwin-arm64", r.manifest.Version, d[:])
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRelease(t, "0.3.0-rc.1", goodSig)
			edit(r)
			if _, _, err := r.feed().Manifest(context.Background(), "darwin-arm64", true); !errors.Is(err, ErrSignature) {
				t.Fatalf("feed: err = %v, want ErrSignature", err)
			}
			if _, err := source(r, r.pub).Open(context.Background(), r.manifest.URL); !errors.Is(err, ErrSignature) {
				t.Fatalf("source: err = %v, want ErrSignature", err)
			}
			if r.downloads() != 0 {
				t.Fatal("a refused manifest's archive was downloaded")
			}
		})
	}
}

// The statement names a digest; the bytes downloaded must be those.
func TestSourceHoldsTheDownloadToTheStatementsDigest(t *testing.T) {
	r := newRelease(t, "0.3.0", goodSig)
	r.archive = []byte(strings.Repeat("sona-archive;", 1000)[:len(r.archive)-1] + "X")
	if _, err := source(r, r.pub).Open(context.Background(), r.manifest.URL); !errors.Is(err, ErrSignature) {
		t.Fatalf("err = %v, want ErrSignature", err)
	}
}

// Only the key the build carries counts; none means updates are off.
func TestFeedNeedsTheBuildsKey(t *testing.T) {
	r := newRelease(t, "0.3.0", goodSig)
	other := newRelease(t, "0.3.0", goodSig)
	f := r.feed()
	f.Key = other.pub
	if _, _, err := f.Manifest(context.Background(), "darwin-arm64", false); !errors.Is(err, ErrSignature) {
		t.Fatalf("another key: err = %v, want ErrSignature", err)
	}
	f.Key = ""
	if _, _, err := f.Manifest(context.Background(), "darwin-arm64", false); !errors.Is(err, ErrNoKey) {
		t.Fatalf("no key: err = %v, want ErrNoKey", err)
	}
}

// The stable feed carries no pre-release, and a release's manifest is that
// release's.
func TestFeedRefusesTheWrongChannelOrRelease(t *testing.T) {
	r := newRelease(t, "0.4.0-rc.1", goodSig)
	if _, _, err := r.feed().Manifest(context.Background(), "darwin-arm64", false); err == nil || errors.Is(err, ErrSignature) {
		t.Fatalf("a pre-release on the stable feed: err = %v", err)
	}
	if _, _, err := r.feed().Manifest(context.Background(), "darwin-arm64", true); err != nil {
		t.Fatalf("the same manifest on the test-version feed: %v", err)
	}

	// An older release's genuine manifest uploaded to the newest tag.
	r = newRelease(t, "0.3.0", goodSig)
	r.resign("0.2.0", "darwin-arm64")
	if _, _, err := r.feed().Manifest(context.Background(), "darwin-arm64", true); err == nil || errors.Is(err, ErrSignature) {
		t.Fatalf("a manifest of another release: err = %v", err)
	}
}
