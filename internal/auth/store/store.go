// Package store persists the Music User Token record.
//
// The record lives in the Secret Service (via go-keyring, service "sona", key
// "mut"). When no Secret Service is reachable, Open falls back to
// <configDir>/credentials.json with mode 0600 and says so: it logs a warning
// and Backend reports BackendFile so Settings and Diagnostics can show an
// "unencrypted" warning. See docs/auth.md §7.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	BackendSecretService Backend = "secret-service"
	BackendFile          Backend = "file (unencrypted)"
)

// Errors returned by Store.Get.
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
// file store in configDir otherwise. It may block while the desktop keyring
// prompts to unlock its collection.
func Open(configDir string, logger *slog.Logger) (Store, error) {
	return OpenWith(configDir, SystemKeyring{}, logger)
}

// OpenWith is Open with an explicit keyring.
//
// Availability is probed by reading the real entry: success or
// keyring.ErrNotFound means the Secret Service answered. A credentials file
// left by an earlier fallback is moved into the keyring once it is available.
func OpenWith(configDir string, kr Keyring, logger *slog.Logger) (Store, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	file := &fileStore{path: filepath.Join(configDir, FileName)}
	_, err := kr.Get(Service, Key)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		logger.Warn("secret service unavailable; storing credentials unencrypted",
			"path", file.path, "err", err)
		return file, nil
	}
	ks := &keyringStore{kr: kr, file: file}
	if rec, ferr := file.Get(); ferr == nil {
		if err := ks.Set(rec); err != nil {
			return nil, fmt.Errorf("store: migrate %s to secret service: %w", file.path, err)
		}
		logger.Info("moved credentials from file to secret service", "path", file.path)
	} else if errors.Is(ferr, ErrCorrupt) {
		logger.Warn("removing unreadable credentials file", "path", file.path, "err", ferr)
		if err := file.Delete(); err != nil {
			return nil, err
		}
	} else if !errors.Is(ferr, ErrNotFound) {
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

// keyringStore keeps the record as JSON in the Secret Service. Set and
// Delete also remove a leftover fallback file.
type keyringStore struct {
	kr   Keyring
	file *fileStore
}

func (s *keyringStore) Backend() Backend { return BackendSecretService }

func (s *keyringStore) Get() (Record, error) {
	v, err := s.kr.Get(Service, Key)
	if errors.Is(err, keyring.ErrNotFound) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("store: secret service get: %w", err)
	}
	return decode([]byte(v))
}

func (s *keyringStore) Set(r Record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := s.kr.Set(Service, Key, string(b)); err != nil {
		return fmt.Errorf("store: secret service set: %w", err)
	}
	return s.file.Delete()
}

func (s *keyringStore) Delete() error {
	err := s.kr.Delete(Service, Key)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		err = fmt.Errorf("store: secret service delete: %w", err)
	} else {
		err = nil
	}
	return errors.Join(err, s.file.Delete())
}

// fileStore keeps the record as JSON in a 0600 file.
type fileStore struct {
	path string
}

func (s *fileStore) Backend() Backend { return BackendFile }

func (s *fileStore) Get() (Record, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("store: %w", err)
	}
	if fi, err := os.Stat(s.path); err == nil && fi.Mode().Perm() != 0o600 {
		if err := os.Chmod(s.path, 0o600); err != nil {
			return Record{}, fmt.Errorf("store: restrict %s: %w", s.path, err)
		}
	}
	return decode(b)
}

// Set writes atomically: a 0600 temp file in the same directory, fsynced,
// then renamed over the old file.
func (s *fileStore) Set(r Record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("store: %w", err)
	}
	f, err := os.CreateTemp(dir, ".credentials-*.json")
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

func (s *fileStore) Delete() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("store: %w", err)
	}
	return nil
}
