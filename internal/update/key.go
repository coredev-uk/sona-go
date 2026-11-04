package update

import (
	"crypto/ed25519"
	_ "embed"
	"encoding/base64"
	"errors"
	"strings"
)

// publicKey is the Ed25519 public key (base64) that signs this build's
// updates: the maintainer's, committed as update.pub (`go tool mygo keygen`
// writes the pair). Release builds also hand the same file to MyGo's updater
// (-X github.com/egoist/mygo.packageUpdateKey). The file is empty until the
// maintainer commits a key, and an empty key means self-updates are off:
// nothing unsigned is ever installed.
//
//go:embed update.pub
var publicKey string

// PublicKey returns the embedded public key ("" when updates are off).
func PublicKey() string { return strings.TrimSpace(publicKey) }

// ErrNoKey means the build carries no update key.
var ErrNoKey = errors.New("update: this build has no update signing key")

// ErrSignature means an archive isn't signed with the build's key.
var ErrSignature = errors.New("update: the archive is not signed with the app's key")

// ErrInstall means an update couldn't be downloaded, unpacked or swapped in.
var ErrInstall = errors.New("update: the update couldn't be installed")

// ParseKey decodes a base64 Ed25519 public key; "" is ErrNoKey.
func ParseKey(s string) (ed25519.PublicKey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ErrNoKey
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("update: invalid public key")
	}
	return ed25519.PublicKey(b), nil
}

// Sign signs an archive's SHA-256 digest, the format MyGo's updater checks.
func Sign(key ed25519.PrivateKey, digest []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, digest))
}

// Verify checks the signature of an archive's SHA-256 digest.
func Verify(key ed25519.PublicKey, digest []byte, signature string) error {
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || !ed25519.Verify(key, digest, sig) {
		return ErrSignature
	}
	return nil
}
