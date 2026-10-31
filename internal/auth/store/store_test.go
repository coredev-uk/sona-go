package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// fakeKeyring is an in-memory Secret Service. down makes every call fail the
// way go-keyring does without a D-Bus secrets provider.
type fakeKeyring struct {
	items map[string]string
	down  bool
}

var errNoService = errors.New("The name org.freedesktop.secrets was not provided by any .service files")

func (k *fakeKeyring) Get(service, user string) (string, error) {
	if k.down {
		return "", errNoService
	}
	v, ok := k.items[service+"/"+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (k *fakeKeyring) Set(service, user, password string) error {
	if k.down {
		return errNoService
	}
	if k.items == nil {
		k.items = map[string]string{}
	}
	k.items[service+"/"+user] = password
	return nil
}

func (k *fakeKeyring) Delete(service, user string) error {
	if k.down {
		return errNoService
	}
	if _, ok := k.items[service+"/"+user]; !ok {
		return keyring.ErrNotFound
	}
	delete(k.items, service+"/"+user)
	return nil
}

var rec = Record{Token: "0.Atoken", ObtainedAt: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC), Restricted: true}

func TestFileRoundTripPermsDelete(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sona") // not yet created
	s, err := OpenWith(dir, &fakeKeyring{down: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Backend() != BackendFile {
		t.Fatalf("backend = %q", s.Backend())
	}
	if _, err := s.Get(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty Get = %v", err)
	}
	if err := s.Set(rec); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, FileName)
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
	got, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != rec.Token || !got.ObtainedAt.Equal(rec.ObtainedAt) || !got.Restricted {
		t.Fatalf("got %v", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file still present: %v", err)
	}
	if err := s.Delete(); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestFileTightensLoosePerms(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenWith(dir, &fakeKeyring{down: true}, nil)
	if err := s.Set(rec); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, FileName)
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
}

func TestFileCorrupt(t *testing.T) {
	for name, body := range map[string]string{"not json": "{nope", "no token": `{"restricted":true}`} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			s, _ := OpenWith(dir, &fakeKeyring{down: true}, nil)
			if _, err := s.Get(); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("Get = %v, want ErrCorrupt", err)
			}
			if err := s.Set(rec); err != nil {
				t.Fatal(err)
			}
			if got, err := s.Get(); err != nil || got.Token != rec.Token {
				t.Fatalf("after overwrite: %v %v", got, err)
			}
		})
	}
}

func TestSecretServiceSelectedAndUsed(t *testing.T) {
	dir := t.TempDir()
	kr := &fakeKeyring{}
	s, err := OpenWith(dir, kr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Backend() != SystemBackend {
		t.Fatalf("backend = %q", s.Backend())
	}
	if _, err := s.Get(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty Get = %v", err)
	}
	if err := s.Set(rec); err != nil {
		t.Fatal(err)
	}
	if _, ok := kr.items[Service+"/"+Key]; !ok {
		t.Fatal("record not in keyring under sona/mut")
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("secret service store wrote the fallback file")
	}
	if got, err := s.Get(); err != nil || got.Token != rec.Token || !got.Restricted {
		t.Fatalf("got %v %v", got, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if len(kr.items) != 0 {
		t.Fatal("keyring entry not deleted")
	}
	if err := s.Delete(); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestFileMigratesIntoSecretService(t *testing.T) {
	dir := t.TempDir()
	old, _ := OpenWith(dir, &fakeKeyring{down: true}, nil)
	if err := old.Set(rec); err != nil {
		t.Fatal(err)
	}
	kr := &fakeKeyring{}
	s, err := OpenWith(dir, kr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Backend() != SystemBackend {
		t.Fatalf("backend = %q", s.Backend())
	}
	if got, err := s.Get(); err != nil || got.Token != rec.Token {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("plaintext file kept after migration")
	}
}

func TestSecretKeptBesideTheRecord(t *testing.T) {
	dir := t.TempDir()
	down := &fakeKeyring{down: true}
	old, err := OpenSecretWith(dir, "lastfm", down, nil)
	if err != nil {
		t.Fatal(err)
	}
	if old.Backend() != BackendFile {
		t.Fatalf("backend = %q", old.Backend())
	}
	if err := old.Set("session-key"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials-lastfm")
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("fallback file: %v %v", fi, err)
	}
	// The MUT record and the secret do not share a file or a keyring entry.
	mut, _ := OpenWith(dir, down, nil)
	if err := mut.Set(rec); err != nil {
		t.Fatal(err)
	}
	if got, err := old.Get(); err != nil || got != "session-key" {
		t.Fatalf("secret after MUT write: %q %v", got, err)
	}

	kr := &fakeKeyring{}
	s, err := OpenSecretWith(dir, "lastfm", kr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := kr.items[Service+"/lastfm"]; got != "session-key" {
		t.Fatalf("keyring entry = %q", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("plaintext secret kept after migration")
	}
	if _, ok := kr.items[Service+"/"+Key]; ok {
		t.Fatal("secret migration touched the MUT entry")
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if _, err := OpenSecretWith(dir, Key, kr, nil); err == nil {
		t.Fatal("a named secret may not reuse the MUT key")
	}
}

func TestRecordStringRedacts(t *testing.T) {
	if s := rec.String(); s == "" || strings.Contains(s, rec.Token) {
		t.Fatalf("String = %q", s)
	}
}
