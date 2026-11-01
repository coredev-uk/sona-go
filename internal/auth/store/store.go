// Package store persists Sona's secrets: the Music User Token record and
// other named secrets (the scrobblers' session key and user token).
//
// Every secret lives in the system keyring (via go-keyring, service "sona",
// one key per secret: "mut" for the MUT record): the Secret Service on
// Linux and other Unix systems, the login Keychain on macOS, Credential
// Manager on Windows (SystemBackend). When the keyring isn't reachable, Open and OpenSecret fall back to a file in the config directory
// (credentials.json for the MUT, credentials-<key> for named secrets) with
// mode 0600 and say so: they log a warning and Backend reports BackendFile so
// Settings and Diagnostics can show an "unencrypted" warning. Only an error
// from the keyring selects the file: every call blocks for as long as the
// keyring takes (an unlock prompt, a Secret Service activation), so callers
// keep it off the UI path (auth.Manager runs it in the background with a
// deadline). See docs/auth.md §7.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// Keyring service and key names.
const (
	Service = "sona"
	Key     = "mut"
)

// FileName is the fallback credentials file inside the config directory.
const FileName = "credentials.json"

// Backend names where the record is kept.
type Backend string

const (
	BackendSecretService     Backend = "secret-service"
	BackendKeychain          Backend = "keychain"
	BackendCredentialManager Backend = "credential-manager"
	BackendFile              Backend = "file (unencrypted)"
)

// Errors returned by Store.Get and Secret.Get.
var (
	ErrNotFound = errors.New("store: no stored credentials")
	ErrCorrupt  = errors.New("store: stored credentials are unreadable")
)

// Record is the persisted Music User Token and its metadata.
type Record struct {
	Token      string    `json:"token"`
	ObtainedAt time.Time `json:"obtained_at"`
	Restricted bool      `json:"restricted"` // itre: explicit content restricted
}

// String keeps the token out of logs.
func (r Record) String() string {
	return fmt.Sprintf("mut(len=%d obtained=%s restricted=%v)", len(r.Token), r.ObtainedAt.UTC().Format(time.RFC3339), r.Restricted)
}

// LogValue keeps the token out of structured logs.
func (r Record) LogValue() slog.Value { return slog.StringValue(r.String()) }

// Store persists one Record.
type Store interface {
	// Get returns the stored record, ErrNotFound when there is none, or an
	// error wrapping ErrCorrupt when the stored bytes do not decode.
	Get() (Record, error)
	// Set replaces the stored record.
	Set(Record) error
	// Delete removes the record; deleting a missing record is not an error.
	Delete() error
	// Backend reports where the record is kept.
	Backend() Backend
}

// Secret persists one opaque string (a session key, an API token).
type Secret interface {
	// Get returns the stored secret, ErrNotFound when there is none, or an
	// error wrapping ErrCorrupt when the stored value is empty.
	Get() (string, error)
	// Set replaces the stored secret.
	Set(string) error
	// Delete removes the secret; deleting a missing secret is not an error.
	Delete() error
	// Backend reports where the secret is kept.
	Backend() Backend
}

// Keyring is the subset of go-keyring Sona uses; tests inject fakes.
type Keyring interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

// SystemKeyring is the desktop Secret Service via go-keyring.
type SystemKeyring struct{}

func (SystemKeyring) Get(service, user string) (string, error) { return keyring.Get(service, user) }
func (SystemKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}
func (SystemKeyring) Delete(service, user string) error { return keyring.Delete(service, user) }

// Open returns the Secret Service store when it is reachable and the 0600
// file store in configDir otherwise. It may block, without a deadline,
// while the desktop keyring prompts to unlock its collection or starts.
func Open(configDir string, logger *slog.Logger) (Store, error) {
	return OpenWith(configDir, SystemKeyring{}, logger)
}

// OpenWith is Open with an explicit keyring.
//
// Availability is probed by reading the real entry: success or
// keyring.ErrNotFound means the Secret Service answered. A credentials file
// left by an earlier fallback is moved into the keyring once it is available.
func OpenWith(configDir string, kr Keyring, logger *slog.Logger) (Store, error) {
	s, err := openSlot(filepath.Join(configDir, FileName), Key, kr, func(b []byte) error {
		_, err := decode(b)
		return err
	}, logger)
	if err != nil {
		return nil, err
	}
	return recordStore{s}, nil
}

// OpenSecret returns the named secret (keyring key "<key>" under Service)
// with the same Secret Service probe, 0600 file fallback
// (<configDir>/credentials-<key>) and migration as Open. key must not be
// Key.
func OpenSecret(configDir, key string, logger *slog.Logger) (Secret, error) {
	return OpenSecretWith(configDir, key, SystemKeyring{}, logger)
}

// OpenSecretWith is OpenSecret with an explicit keyring.
func OpenSecretWith(configDir, key string, kr Keyring, logger *slog.Logger) (Secret, error) {
	if key == "" || key == Key || strings.ContainsAny(key, `/\`) {
		return nil, fmt.Errorf("store: invalid secret key %q", key)
	}
	s, err := openSlot(filepath.Join(configDir, "credentials-"+key), key, kr, func(b []byte) error {
		_, err := decodeSecret(b)
		return err
	}, logger)
	if err != nil {
		return nil, err
	}
	return secretStore{s}, nil
}

// slot is one stored value: a Secret Service entry or its 0600 file.
type slot interface {
	get() ([]byte, error) // ErrNotFound when absent
	set([]byte) error
	del() error
	backend() Backend
}

// openSlot probes the keyring with key, falls back to the file at path, and
// moves a valid leftover file into the keyring (removing an invalid one).
func openSlot(path, key string, kr Keyring, valid func([]byte) error, logger *slog.Logger) (slot, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	file := &fileSlot{path: path}
	_, err := kr.Get(Service, key)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		logger.Warn("system keyring unavailable; storing credentials unencrypted",
			"path", file.path, "err", err)
		return file, nil
	}
	ks := &keyringSlot{kr: kr, key: key, file: file}
	b, ferr := file.get()
	if ferr == nil {
		ferr = valid(b)
	}
	switch {
	case ferr == nil:
		if err := ks.set(b); err != nil {
			return nil, fmt.Errorf("store: migrate %s to the system keyring: %w", file.path, err)
		}
		logger.Info("moved credentials from file to the system keyring", "path", file.path)
	case errors.Is(ferr, ErrCorrupt):
		logger.Warn("removing unreadable credentials file", "path", file.path, "err", ferr)
		if err := file.del(); err != nil {
			return nil, err
		}
	case !errors.Is(ferr, ErrNotFound):
		return nil, ferr
	}
	return ks, nil
}

func decode(b []byte) (Record, error) {
	var r Record
	if err := json.Unmarshal(b, &r); err != nil || r.Token == "" {
		if err == nil {
			err = errors.New("empty token")
		}
		return Record{}, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	return r, nil
}

func decodeSecret(b []byte) (string, error) {
	if len(b) == 0 {
		return "", fmt.Errorf("%w: empty secret", ErrCorrupt)
	}
	return string(b), nil
}

// recordStore keeps the Record as JSON.
type recordStore struct{ s slot }

func (r recordStore) Backend() Backend { return r.s.backend() }

func (r recordStore) Get() (Record, error) {
	b, err := r.s.get()
	if err != nil {
		return Record{}, err
	}
	return decode(b)
}

func (r recordStore) Set(rec Record) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return r.s.set(b)
}

func (r recordStore) Delete() error { return r.s.del() }

// secretStore keeps the string as is.
type secretStore struct{ s slot }

func (r secretStore) Backend() Backend { return r.s.backend() }

func (r secretStore) Get() (string, error) {
	b, err := r.s.get()
	if err != nil {
		return "", err
	}
	return decodeSecret(b)
}

func (r secretStore) Set(v string) error {
	if v == "" {
		return errors.New("store: empty secret")
	}
	return r.s.set([]byte(v))
}

func (r secretStore) Delete() error { return r.s.del() }

// keyringSlot keeps the value in the Secret Service. set and del also
// remove a leftover fallback file.
type keyringSlot struct {
	kr   Keyring
	key  string
	file *fileSlot
}

func (s *keyringSlot) backend() Backend { return SystemBackend }

func (s *keyringSlot) get() ([]byte, error) {
	v, err := s.kr.Get(Service, s.key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: keyring get: %w", err)
	}
	return []byte(v), nil
}

func (s *keyringSlot) set(b []byte) error {
	if err := s.kr.Set(Service, s.key, string(b)); err != nil {
		return fmt.Errorf("store: keyring set: %w", err)
	}
	return s.file.del()
}

func (s *keyringSlot) del() error {
	err := s.kr.Delete(Service, s.key)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		err = fmt.Errorf("store: keyring delete: %w", err)
	} else {
		err = nil
	}
	return errors.Join(err, s.file.del())
}

// fileSlot keeps the value in a 0600 file.
type fileSlot struct {
	path string
}

func (s *fileSlot) backend() Backend { return BackendFile }

func (s *fileSlot) get() ([]byte, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	if fi, err := os.Stat(s.path); err == nil && fi.Mode().Perm() != 0o600 {
		if err := os.Chmod(s.path, 0o600); err != nil {
			return nil, fmt.Errorf("store: restrict %s: %w", s.path, err)
		}
	}
	return b, nil
}

// set writes atomically: a 0600 temp file in the same directory, fsynced,
// then renamed over the old file.
func (s *fileSlot) set(b []byte) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("store: %w", err)
	}
	base := filepath.Base(s.path)
	ext := filepath.Ext(base)
	f, err := os.CreateTemp(dir, "."+strings.TrimSuffix(base, ext)+"-*"+ext)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op after a successful rename
	// CreateTemp already uses 0600; Chmod guards against an unusual umask setup.
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return fmt.Errorf("store: %w", err)
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return fmt.Errorf("store: write %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("store: sync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("store: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("store: %w", err)
	}
	return nil
}

func (s *fileSlot) del() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("store: %w", err)
	}
	return nil
}
