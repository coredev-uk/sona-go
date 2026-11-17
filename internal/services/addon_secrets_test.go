package services

import (
	"errors"
	"sync"
	"testing"

	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/config"
)

// flakyKeyring is a keyring whose writes and deletes of chosen items fail,
// or are silently dropped: the crashes and faults a migration must survive.
type flakyKeyring struct {
	*memKeyring
	mu                        sync.Mutex
	failSet, failDelete, drop map[string]bool // by item name (the keyring's user)
}

func newFlaky() *flakyKeyring {
	return &flakyKeyring{memKeyring: &memKeyring{}, failSet: map[string]bool{}, failDelete: map[string]bool{}, drop: map[string]bool{}}
}

func (k *flakyKeyring) Set(service, user, password string) error {
	k.mu.Lock()
	fail, drop := k.failSet[user], k.drop[user]
	k.mu.Unlock()
	switch {
	case fail:
		return errors.New("the keyring went away")
	case drop:
		return nil
	}
	return k.memKeyring.Set(service, user, password)
}

func (k *flakyKeyring) Delete(service, user string) error {
	k.mu.Lock()
	fail := k.failDelete[user]
	k.mu.Unlock()
	if fail {
		return errors.New("the keyring went away")
	}
	return k.memKeyring.Delete(service, user)
}

func (k *flakyKeyring) heal() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.failSet, k.failDelete, k.drop = map[string]bool{}, map[string]bool{}, map[string]bool{}
}

func (k *flakyKeyring) has(item string) (string, bool) {
	v, err := k.Get(store.Service, item)
	return v, err == nil
}

func migrationEnv(t *testing.T, kr store.Keyring) *Env {
	t.Helper()
	return &Env{Deps: Deps{Keyring: kr, Paths: config.Paths{Config: t.TempDir()}, Logger: discard()}}
}

func TestMigrateSecretMovesTheOldItem(t *testing.T) {
	kr := newFlaky()
	_ = kr.memKeyring.Set(store.Service, "lastfm", "sk-old")
	e := migrationEnv(t, kr)
	if err := e.migrateSecret(discard(), "lastfm", "lastfm", "session"); err != nil {
		t.Fatal(err)
	}
	if v, ok := kr.has("addon.lastfm"); !ok || v != `{"session":"sk-old"}` {
		t.Errorf("addon.lastfm = %q, %v", v, ok)
	}
	if _, ok := kr.has("lastfm"); ok {
		t.Error("the old item survived the move")
	}
	// Again: nothing left to move, nothing changes.
	if err := e.migrateSecret(discard(), "lastfm", "lastfm", "session"); err != nil {
		t.Fatal(err)
	}
	if v, _ := kr.has("addon.lastfm"); v != `{"session":"sk-old"}` {
		t.Errorf("a second run changed it: %q", v)
	}
}

// With nothing stored there is nothing to move, and no empty item appears.
func TestMigrateSecretWithNothingStoredLeavesNothing(t *testing.T) {
	kr := newFlaky()
	e := migrationEnv(t, kr)
	if err := e.migrateSecret(discard(), "listenbrainz", "listenbrainz", "token"); err != nil {
		t.Fatal(err)
	}
	if len(kr.all()) != 0 {
		t.Errorf("keyring = %q", kr.all())
	}
}

// The crash test. A crash between the copy and the deletion leaves both
// items; the next start checks the copy and finishes. A crash before the
// copy leaves the old one alone. The sign-in is never in neither place.
func TestMigrateSecretSurvivesACrashAtEveryStep(t *testing.T) {
	newer := func() (*flakyKeyring, *Env) {
		kr := newFlaky()
		_ = kr.memKeyring.Set(store.Service, "listenbrainz", "tok-old")
		return kr, migrationEnv(t, kr)
	}
	inEitherPlace := func(kr *flakyKeyring) bool {
		_, old := kr.has("listenbrainz")
		v, nu := kr.has("addon.listenbrainz")
		return old || (nu && v == `{"token":"tok-old"}`)
	}

	t.Run("after the copy, before the delete", func(t *testing.T) {
		kr, e := newer()
		kr.failDelete["listenbrainz"] = true
		if err := e.migrateSecret(discard(), "listenbrainz", "listenbrainz", "token"); err == nil {
			t.Fatal("a failed delete went unnoticed")
		}
		_, old := kr.has("listenbrainz")
		v, nu := kr.has("addon.listenbrainz")
		if !old || !nu || v != `{"token":"tok-old"}` {
			t.Fatalf("after the crash: old %v, new %q %v", old, v, nu)
		}
		kr.heal() // the next start
		if err := e.migrateSecret(discard(), "listenbrainz", "listenbrainz", "token"); err != nil {
			t.Fatal(err)
		}
		if _, old := kr.has("listenbrainz"); old {
			t.Error("the next start left the old item")
		}
		if v, _ := kr.has("addon.listenbrainz"); v != `{"token":"tok-old"}` {
			t.Errorf("addon.listenbrainz = %q", v)
		}
	})

	t.Run("before the copy", func(t *testing.T) {
		kr, e := newer()
		kr.failSet["addon.listenbrainz"] = true
		if err := e.migrateSecret(discard(), "listenbrainz", "listenbrainz", "token"); err == nil {
			t.Fatal("a failed copy went unnoticed")
		}
		if !inEitherPlace(kr) {
			t.Fatal("the sign-in is in neither place")
		}
		if _, nu := kr.has("addon.listenbrainz"); nu {
			t.Error("a half-written new item")
		}
		kr.heal()
		if err := e.migrateSecret(discard(), "listenbrainz", "listenbrainz", "token"); err != nil {
			t.Fatal(err)
		}
		if v, _ := kr.has("addon.listenbrainz"); v != `{"token":"tok-old"}` {
			t.Errorf("addon.listenbrainz = %q", v)
		}
		if _, old := kr.has("listenbrainz"); old {
			t.Error("the old item survived")
		}
	})

	t.Run("a copy that does not read back keeps the old item", func(t *testing.T) {
		kr, e := newer()
		kr.drop["addon.listenbrainz"] = true // the write "succeeds" and is lost
		if err := e.migrateSecret(discard(), "listenbrainz", "listenbrainz", "token"); err == nil {
			t.Fatal("an unchecked copy was trusted")
		}
		if v, ok := kr.has("listenbrainz"); !ok || v != "tok-old" {
			t.Errorf("the old item = %q, %v", v, ok)
		}
	})
}

// A sign-in made through the new place after the move is the newer one: it
// wins, and the old item still goes.
func TestMigrateSecretKeepsTheNewerSignIn(t *testing.T) {
	kr := newFlaky()
	_ = kr.memKeyring.Set(store.Service, "lastfm", "sk-old")
	_ = kr.memKeyring.Set(store.Service, "addon.lastfm", `{"session":"sk-new"}`)
	e := migrationEnv(t, kr)
	if err := e.migrateSecret(discard(), "lastfm", "lastfm", "session"); err != nil {
		t.Fatal(err)
	}
	if v, _ := kr.has("addon.lastfm"); v != `{"session":"sk-new"}` {
		t.Errorf("addon.lastfm = %q", v)
	}
	if _, old := kr.has("lastfm"); old {
		t.Error("the old item survived")
	}
}

// The move keeps the other secrets of the add-on's item.
func TestMigrateSecretKeepsTheOtherKeys(t *testing.T) {
	kr := newFlaky()
	_ = kr.memKeyring.Set(store.Service, "lastfm", "sk-old")
	_ = kr.memKeyring.Set(store.Service, "addon.lastfm", `{"other":"x"}`)
	e := migrationEnv(t, kr)
	if err := e.migrateSecret(discard(), "lastfm", "lastfm", "session"); err != nil {
		t.Fatal(err)
	}
	m, err := e.addonSecrets().all("lastfm")
	if err != nil || m["session"] != "sk-old" || m["other"] != "x" {
		t.Errorf("secrets = %v, %v", m, err)
	}
}

// The service moves both sign-ins when it loads, and the accounts stay
// connected through it.
func TestScrobbleServiceMovesTheOldSignInsAtLoad(t *testing.T) {
	se := newScrobbleEnv(t)
	_ = se.kr.Set(store.Service, "lastfm", "sk-old")
	_ = se.kr.Set(store.Service, "listenbrainz", "tok-old")
	st := se.s.Status()
	if !st.LastFM.Connected || !st.ListenBrainz.Connected {
		t.Fatalf("status = %+v", st)
	}
	if se.signIn(t, "lastfm", "session") != "sk-old" || se.signIn(t, "listenbrainz", "token") != "tok-old" {
		t.Error("the sign-ins are not in the add-ons' items")
	}
	for _, old := range []string{"lastfm", "listenbrainz"} {
		if _, err := se.kr.Get(store.Service, old); err == nil {
			t.Errorf("the old item %s survived", old)
		}
	}
}

// A move that fails (the keyring refuses the new item) leaves the account
// connected from the old item, and disconnecting removes it from both.
func TestScrobbleServiceKeepsWorkingWhenTheMoveFails(t *testing.T) {
	kr := newFlaky()
	kr.failSet["addon.lastfm"] = true
	se := newScrobbleEnvWith(t, kr)
	_ = kr.memKeyring.Set(store.Service, "lastfm", "sk-old")
	if st := se.s.Status(); !st.LastFM.Connected {
		t.Fatalf("status = %+v", st)
	}
	if _, ok := kr.has("lastfm"); !ok {
		t.Error("the old item was dropped though the move failed")
	}
	kr.heal()
	if _, err := se.s.Disconnect("lastfm"); err != nil {
		t.Fatal(err)
	}
	if _, ok := kr.has("lastfm"); ok {
		t.Error("the old item came back after Disconnect")
	}
}
