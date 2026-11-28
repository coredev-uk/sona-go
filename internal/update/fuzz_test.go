package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"path"
	"strings"
	"testing"
	"time"
)

// The fuzz targets below feed the updater what a release page, a download or
// a man in the middle can send. Their seeds are in testdata/fuzz/<target>
// (made-up, signed with a key from a fixed seed that signs nothing real);
// go test runs them as ordinary tests.

// fuzzKey is the maintainer key of the fuzz targets, and fuzzOther a key that
// never signs anything: no input may verify under it.
var (
	fuzzKey   = ed25519.NewKeyFromSeed([]byte("sona-update-fuzz-seed-0123456789"))
	fuzzOther = ed25519.NewKeyFromSeed([]byte("sona-update-fuzz-other-012345678"))
)

const (
	fuzzTarget  = "darwin-arm64"
	fuzzVersion = "0.6.0"
)

// fuzzDigest is the digest of the archive the genuine manifest and statement
// are for.
var fuzzDigest = sha256.Sum256([]byte("archive"))

// genuineManifest is what the key holder publishes for fuzzVersion.
func genuineManifest() Manifest { return signedManifest(fuzzVersion) }

// fuzzPrerelease is a second version the key holder signed.
const fuzzPrerelease = "0.6.0-rc.1"

// signedManifest is the manifest the key holder publishes for version.
func signedManifest(version string) Manifest {
	return Manifest{
		Version: version, Notes: "notes", Date: "2026-11-01",
		URL: "https://github.com/coredev-uk/sona-go/releases/download/v0.6.0/sona.tar.gz", Size: 1234,
		Signature: Sign(fuzzKey, fuzzDigest[:]), SHA256: hex.EncodeToString(fuzzDigest[:]),
		StatementSignature: SignStatement(fuzzKey, fuzzTarget, version, fuzzDigest[:]),
	}
}

// genuineRelease is the release.json the key holder publishes for 0.6.0.
func genuineRelease(t testing.TB) ReleaseInfo {
	t.Helper()
	r, err := NewReleaseInfo(fuzzKey, fuzzVersion, time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC), "## Changes\n- one")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// FuzzVerifyStatement checks a manifest's signed statement against fields an
// attacker controls.
//
// Invariants: no panic; a nil error means the version, the digest and the
// signature are the ones the key holder signed for this target (the digest
// may differ in hex case only), and the target is the signed one; under any
// other key no input verifies; every error is ErrSignature.
func FuzzVerifyStatement(f *testing.F) {
	genuine := genuineManifest()
	want, _ := base64.StdEncoding.DecodeString(genuine.StatementSignature)
	f.Fuzz(func(t *testing.T, version, target, digest, sig string) {
		m := Manifest{Version: version, SHA256: digest, StatementSignature: sig}
		err := VerifyStatement(fuzzKey.Public().(ed25519.PublicKey), m, target)
		if err != nil {
			if !errors.Is(err, ErrSignature) {
				t.Fatalf("error %v is not ErrSignature", err)
			}
		} else {
			got, _ := base64.StdEncoding.DecodeString(sig)
			if version != fuzzVersion || target != fuzzTarget || !strings.EqualFold(digest, genuine.SHA256) || !bytes.Equal(got, want) {
				t.Fatalf("verified version %q target %q digest %q signature %q", version, target, digest, sig)
			}
		}
		if VerifyStatement(fuzzOther.Public().(ed25519.PublicKey), m, target) == nil {
			t.Fatalf("verified under another key: %q %q %q %q", version, target, digest, sig)
		}
	})
}

// FuzzParseReleaseInfo reads release.json.
//
// Invariants: no panic; a nil error means the document names the version,
// time and notes the key holder signed, however its JSON is spelt (case of
// keys, duplicates, escapes) and the signature is the genuine one; under any
// other key no input verifies; every error is ErrSignature or a decoding
// error, never a success.
func FuzzParseReleaseInfo(f *testing.F) {
	genuine := genuineRelease(f)
	f.Fuzz(func(t *testing.T, raw []byte) {
		r, err := ParseReleaseInfo(raw, fuzzKey.Public().(ed25519.PublicKey))
		if err == nil {
			if r != genuine {
				t.Fatalf("verified %+v, the key holder signed %+v", r, genuine)
			}
		}
		if _, err := ParseReleaseInfo(raw, fuzzOther.Public().(ed25519.PublicKey)); err == nil {
			t.Fatalf("verified under another key: %q", raw)
		}
	})
}

// FuzzParseManifest reads update-<target>.json as the feed does: decoding,
// validation and the signed statement.
//
// Invariants: no panic; a nil error means the manifest's version, platform
// and archive digest are the signed ones (statement binding), its URL is
// https (or loopback http) and its size positive and within MaxArchive; the
// stable feed never accepts a pre-release; a prerelease read accepts the
// manifest only of the tag it came from; under any other key no input
// verifies.
func FuzzParseManifest(f *testing.F) {
	signed := map[string]Manifest{fuzzVersion: genuineManifest(), fuzzPrerelease: signedManifest(fuzzPrerelease)}
	f.Fuzz(func(t *testing.T, raw []byte, prerelease bool, tag string) {
		pub := fuzzKey.Public().(ed25519.PublicKey)
		m, err := parseManifest(raw, pub, fuzzTarget, prerelease, tag)
		if err == nil {
			want, ok := signed[m.Version]
			if !ok || !strings.EqualFold(m.SHA256, want.SHA256) || !sameSignature(m.StatementSignature, want.StatementSignature) {
				t.Fatalf("accepted %+v", m)
			}
			if checkURL(m.URL) != nil || m.Size <= 0 || m.Size > MaxArchive || m.Signature == "" {
				t.Fatalf("accepted an unusable manifest %+v", m)
			}
			if prerelease && m.Version != strings.TrimPrefix(tag, "v") || !prerelease && ChannelOf(m.Version) != ChannelStable {
				t.Fatalf("accepted %q on prerelease=%v tag %q", m.Version, prerelease, tag)
			}
		}
		if _, err := parseManifest(raw, fuzzOther.Public().(ed25519.PublicKey), fuzzTarget, prerelease, tag); err == nil {
			t.Fatalf("accepted under another key: %q", raw)
		}
	})
}

// sameSignature is whether two base64 signatures decode to the same bytes
// (base64 has more than one spelling of a value).
func sameSignature(a, b string) bool {
	x, errA := base64.StdEncoding.DecodeString(a)
	y, errB := base64.StdEncoding.DecodeString(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

// FuzzReleaseTag reads GitHub's releases.atom.
//
// Invariants: no panic; a tag that is returned is a version (with or without
// "v") with no "/", "?" or "#" in it, so it can only name a release of the
// repository; ErrNoRelease or a decoding error otherwise.
func FuzzReleaseTag(f *testing.F) {
	f.Fuzz(func(t *testing.T, raw []byte) {
		tag, err := releaseTag(raw, "https://github.com", Repo)
		if err != nil {
			if tag != "" {
				t.Fatalf("tag %q with error %v", tag, err)
			}
			return
		}
		if !Valid(strings.TrimPrefix(tag, "v")) || strings.ContainsAny(tag, "/?#") {
			t.Fatalf("tag %q", tag)
		}
	})
}

// FuzzCheckArchive reads an update archive (gzip tar) without extracting it.
//
// Invariants: no panic; an error is ErrArchive; when an archive is accepted,
// reading it again finds only files, directories and links that stay inside
// the app, every name a relative path inside the archive without a
// backslash, colon or NUL, no link at all on Windows, sona.exe at the top
// level for Windows and Sona.app alone for macOS.
func FuzzCheckArchive(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, target := range []string{"windows-amd64", "darwin-arm64", "linux-amd64"} {
			err := CheckArchive(bytes.NewReader(b), target)
			if err != nil {
				if !errors.Is(err, ErrArchive) {
					t.Fatalf("%s: error %v is not ErrArchive", target, err)
				}
				continue
			}
			acceptedArchive(t, b, target)
		}
	})
}

// acceptedArchive re-reads an archive CheckArchive let through.
func acceptedArchive(t *testing.T, b []byte, target string) {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("accepted what gzip refuses: %v", err)
	}
	tr := tar.NewReader(gz)
	top := map[string]bool{}
	n := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("accepted what tar refuses: %v", err)
		}
		n++
		name := path.Clean(hdr.Name)
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:\x00") {
			t.Fatalf("accepted the name %q", hdr.Name)
		}
		first, _, _ := strings.Cut(name, "/")
		top[first] = true
		switch hdr.Typeflag {
		case tar.TypeDir, tar.TypeReg:
		case tar.TypeSymlink:
			if path.IsAbs(hdr.Linkname) || !fs.ValidPath(path.Join(path.Dir(name), hdr.Linkname)) || target == "windows-amd64" {
				t.Fatalf("accepted the link %q -> %q for %s", hdr.Name, hdr.Linkname, target)
			}
		default:
			t.Fatalf("accepted the entry type %q of %q", hdr.Typeflag, hdr.Name)
		}
	}
	switch {
	case n == 0:
		t.Fatal("accepted an empty archive")
	case target == "windows-amd64" && !top["sona.exe"]:
		t.Fatalf("accepted a Windows archive without sona.exe: %v", top)
	case target == "darwin-arm64" && (len(top) != 1 || !top["Sona.app"]):
		t.Fatalf("accepted a macOS archive that is not Sona.app alone: %v", top)
	}
}
