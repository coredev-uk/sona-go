package update

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"golang.org/x/mod/semver"
)

// The channels a manifest's statement names. A version with a pre-release
// part (0.6.0-rc.1) is on the prerelease channel, any other on stable.
const (
	ChannelStable     = "stable"
	ChannelPrerelease = "prerelease"
)

// ChannelOf is the channel of version v ("" for a string that isn't one).
func ChannelOf(v string) string {
	sv, ok := semverOf(v)
	switch {
	case !ok:
		return ""
	case semver.Prerelease(sv) != "":
		return ChannelPrerelease
	}
	return ChannelStable
}

// ErrNotNewer means an update isn't a newer version than the running one.
var ErrNotNewer = errors.New("update: the update is not newer than the running version")

// statementPrefix names and versions the signed statement, so a signature
// made for something else (an archive digest, another format) never verifies
// as one.
const statementPrefix = "sona-update-v1"

// Statement is what cmd/releasesign signs beside the archive's digest:
//
//	sona-update-v1 NUL target NUL version NUL channel NUL hex(sha256)
//
// The manifest's version and platform sit outside MyGo's signature, which
// covers only the digest, so the digest alone can't tell a genuine archive of
// this platform and release from an older release's, or another platform's,
// that someone put under a newer version. NUL can't occur in any field:
// target and channel are fixed words, version a semantic version and the
// digest hex.
func Statement(target, version, channel string, digest []byte) []byte {
	var b bytes.Buffer
	for _, f := range []string{statementPrefix, target, version, channel, hex.EncodeToString(digest)} {
		if b.Len() > 0 {
			b.WriteByte(0)
		}
		b.WriteString(f)
	}
	return b.Bytes()
}

// SignStatement signs the statement of an archive with digest for version on
// target (the channel follows from the version), base64 like Sign.
func SignStatement(key ed25519.PrivateKey, target, version string, digest []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, Statement(target, version, ChannelOf(version), digest)))
}

// VerifyStatement checks that m's version, target and archive digest are the
// ones the maintainer's key signed. It reads nothing but m: the digest comes
// from m.SHA256, which Source.Open then holds the download to.
func VerifyStatement(key ed25519.PublicKey, m Manifest, target string) error {
	if m.SHA256 == "" || m.StatementSignature == "" {
		return fmt.Errorf("%w: the manifest carries no signed statement", ErrSignature)
	}
	digest, err := hex.DecodeString(m.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("%w: the manifest's digest is not a SHA-256", ErrSignature)
	}
	channel := ChannelOf(m.Version)
	sig, err := base64.StdEncoding.DecodeString(m.StatementSignature)
	if target == "" || channel == "" || err != nil || !ed25519.Verify(key, Statement(target, m.Version, channel, digest), sig) {
		return fmt.Errorf("%w: the manifest's version %q is not signed for %s", ErrSignature, m.Version, target)
	}
	return nil
}
