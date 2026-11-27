package update

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ReleaseInfoName is the release asset that carries a release's version,
// publication time and notes, the same for every platform, signed with the
// key that signs the update manifests. Reading it is a plain download that
// GitHub's API limit doesn't cover; the API's release endpoints are only the
// fallback for releases published without it (up to v0.8.0-rc.4).
const ReleaseInfoName = "release.json"

// MaxReleaseInfo bounds what reading release.json takes in.
const MaxReleaseInfo = 1 << 20

// ReleaseInfo is release.json. cmd/releasesign writes it.
type ReleaseInfo struct {
	// Version is the release's version without a leading "v".
	Version string `json:"version"`
	// Published is when the release was made, RFC 3339.
	Published string `json:"published"`
	// Notes is the release's changelog (Markdown): the release body cut at
	// InstallNotesMarker and without the version's own heading.
	Notes string `json:"notes,omitempty"`
	// Signature is the Ed25519 signature of ReleaseStatement, base64.
	Signature string `json:"signature"`
}

// releasePrefix names and versions the signed statement, so a signature made
// for something else (an archive's statement, another format) never verifies
// as one.
const releasePrefix = "sona-release-v1"

// ReleaseStatement is what cmd/releasesign signs for release.json:
//
//	sona-release-v1 NUL version NUL channel NUL published NUL hex(sha256(notes))
//
// It binds the notes, the publication time and the version (and its channel)
// to the key holder, so release.json put under another tag, or with other
// notes, doesn't verify as that release's. NUL can't occur in any field but
// the notes, which are hashed: version is a semantic version, channel a fixed
// word and published an RFC 3339 time.
func ReleaseStatement(version, channel, published, notes string) []byte {
	digest := sha256.Sum256([]byte(notes))
	var b bytes.Buffer
	for _, f := range []string{releasePrefix, version, channel, published, hex.EncodeToString(digest[:])} {
		if b.Len() > 0 {
			b.WriteByte(0)
		}
		b.WriteString(f)
	}
	return b.Bytes()
}

// NewReleaseInfo is the release.json of version, signed with key (the
// channel follows from the version). Published is written in UTC.
func NewReleaseInfo(key ed25519.PrivateKey, version string, published time.Time, notes string) (ReleaseInfo, error) {
	version = strings.TrimPrefix(version, "v")
	if !Valid(version) {
		return ReleaseInfo{}, fmt.Errorf("update: %q is not a version", version)
	}
	r := ReleaseInfo{Version: version, Published: published.UTC().Format(time.RFC3339), Notes: notes}
	r.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, ReleaseStatement(r.Version, ChannelOf(r.Version), r.Published, r.Notes)))
	return r, nil
}

// ParseReleaseInfo decodes release.json and checks that the key holder signed
// it: its version, its time and its notes. Anything unsigned or signed by
// another key is ErrSignature.
func ParseReleaseInfo(raw []byte, pub ed25519.PublicKey) (ReleaseInfo, error) {
	var r ReleaseInfo
	if err := json.Unmarshal(raw, &r); err != nil {
		return ReleaseInfo{}, fmt.Errorf("update: decode %s: %w", ReleaseInfoName, err)
	}
	if _, err := time.Parse(time.RFC3339, r.Published); err != nil || !Valid(r.Version) || strings.HasPrefix(r.Version, "v") {
		return ReleaseInfo{}, fmt.Errorf("update: %s names version %q published %q", ReleaseInfoName, r.Version, r.Published)
	}
	sig, err := base64.StdEncoding.DecodeString(r.Signature)
	if err != nil || !ed25519.Verify(pub, ReleaseStatement(r.Version, ChannelOf(r.Version), r.Published, r.Notes), sig) {
		return ReleaseInfo{}, fmt.Errorf("%w: %s of %q is not signed", ErrSignature, ReleaseInfoName, r.Version)
	}
	return r, nil
}
