package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/coredev-uk/sona-go/internal/auth/mut"
	"github.com/coredev-uk/sona-go/internal/auth/store"
)

// hungKeyring is a Secret Service that answers only once let through, like
// an activation that never completes or an unlock prompt nobody answers.
// Without a gate it answers at once; fail is what every call returns once
// it answers (a dismissed prompt).
type hungKeyring struct {
	mu    sync.Mutex
	gate  chan struct{}
	fail  error
	items map[string]string
	ops   []string // answered calls: "get mut", "set mut", "delete mut"
}

func newHungKeyring() *hungKeyring { return &hungKeyring{items: map[string]string{}} }

func (k *hungKeyring) hang() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.gate == nil {
		k.gate = make(chan struct{})
	}
}

func (k *hungKeyring) release() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.gate != nil {
		close(k.gate)
		k.gate = nil
	}
}

func (k *hungKeyring) setFail(err error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.fail = err
}

// answer waits at the gate, then records op and returns fail.
func (k *hungKeyring) answer(op string) error {
	k.mu.Lock()
	gate := k.gate
	k.mu.Unlock()
	if gate != nil {
		<-gate
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.ops = append(k.ops, op)
	return k.fail
}

func (k *hungKeyring) Get(service, user string) (string, error) {
	if err := k.answer("get " + user); err != nil {
		return "", err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.items[service+"/"+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (k *hungKeyring) Set(service, user, password string) error {
	if err := k.answer("set " + user); err != nil {
		return err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.items[service+"/"+user] = password
	return nil
}

func (k *hungKeyring) Delete(service, user string) error {
	if err := k.answer("delete " + user); err != nil {
		return err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.items[service+"/"+user]; !ok {
		return keyring.ErrNotFound
	}
	delete(k.items, service+"/"+user)
	return nil
}

func (k *hungKeyring) put(t *testing.T, tok string) {
	t.Helper()
	b, err := json.Marshal(store.Record{Token: tok, ObtainedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.items[store.Service+"/"+store.Key] = string(b)
}

func (k *hungKeyring) token() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	var r store.Record
	_ = json.Unmarshal([]byte(k.items[store.Service+"/"+store.Key]), &r)
	return r.Token
}

// lastOps waits until the keyring answered n calls and returns the last
// ones.
func (k *hungKeyring) lastOps(t *testing.T, n int) []string {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		k.mu.Lock()
		ops := append([]string(nil), k.ops...)
		k.mu.Unlock()
		if len(ops) >= n {
			return ops[len(ops)-n:]
		}
		if time.Now().After(deadline) {
			t.Fatalf("keyring answered %v", ops)
		}
	}
}

func (k *hungKeyring) answered() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.ops)
}

// useKeyring makes the manager open the real store over k, with its 0600
// fallback file in a temporary config dir (returned).
func (e *env) useKeyring(t *testing.T, k *hungKeyring) string {
	dir := t.TempDir()
	e.open = func() (store.Store, error) { return store.OpenWith(dir, k, nil) }
	return dir
}

func (e *env) cacheAccount(t *testing.T, a Account) {
	t.Helper()
	if err := e.accounts.Save(context.Background(), a); err != nil {
		t.Fatal(err)
	}
}

func noFallbackFile(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, store.FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("credentials written to the fallback file (%v)", err)
	}
}

func TestStartReturnsWhileKeyringHangs(t *testing.T) {
	e := newEnv(t)
	k := newHungKeyring()
	k.put(t, goodMUT)
	e.cacheAccount(t, Account{StorefrontID: "gb", StorefrontName: "United Kingdom"})
	e.useKeyring(t, k)
	e.keyringSlow = 20 * time.Millisecond
	k.hang()
	m, rec := e.manager(t)

	begin := time.Now()
	m.Start()
	if d := time.Since(begin); d > 100*time.Millisecond {
		t.Fatalf("Start waited %v for the keyring", d)
	}
	if s := m.Status(); s.State != StateRestoring || s.Keyring != KeyringReady || m.UserToken() != "" {
		t.Fatalf("while the keyring hangs: %+v", s)
	}
	s := rec.wait(t, func(s Status) bool { return s.Keyring == KeyringWaiting })
	if s.State != StateRestoring || s.Backend != "" {
		t.Fatalf("waiting status = %+v", s)
	}

	k.release()
	s = rec.wait(t, func(s Status) bool { return s.State != StateRestoring })
	if s.State != StateSignedIn || s.Keyring != KeyringReady || s.Backend != store.SystemBackend || m.UserToken() != goodMUT {
		t.Fatalf("after the keyring answered: %+v", s)
	}
}

func TestKeyringTimeoutSignsOutUntilItAnswers(t *testing.T) {
	e := newEnv(t)
	k := newHungKeyring()
	k.put(t, goodMUT)
	e.cacheAccount(t, Account{StorefrontID: "gb"})
	dir := e.useKeyring(t, k)
	e.keyringSlow, e.keyringTimeout = 5*time.Millisecond, 50*time.Millisecond
	k.hang()
	m, rec := e.manager(t)

	m.Start()
	s := rec.wait(t, is(StateSignedOut))
	if s.Keyring != KeyringUnavailable || s.LastError == "" || s.Backend != "" || m.UserToken() != "" {
		t.Fatalf("after the timeout: %+v", s)
	}
	noFallbackFile(t, dir)

	// The answer still comes: auth carries on as if it had come in time.
	k.release()
	s = rec.wait(t, is(StateSignedIn))
	if s.Keyring != KeyringReady || s.LastError != "" || m.UserToken() != goodMUT {
		t.Fatalf("after the late answer: %+v", s)
	}
}

func TestKeyringErrorSignsOutAndRetryReads(t *testing.T) {
	e := newEnv(t)
	e.stored(goodMUT, true)
	e.store.err = errors.New("keyring get: no such secret collection")
	var opens atomic.Int32
	e.open = func() (store.Store, error) { opens.Add(1); return e.store, nil }
	m, rec := e.manager(t)

	s := start(t, m, rec)
	if s.State != StateSignedOut || s.Keyring != KeyringUnavailable || !strings.Contains(s.LastError, "no such secret collection") {
		t.Fatalf("after the error: %+v", s)
	}
	if e.store.token() != goodMUT {
		t.Fatal("the stored token was dropped")
	}

	e.store.mu.Lock()
	e.store.err = nil
	e.store.mu.Unlock()
	m.RetryKeyring()
	if s := rec.wait(t, is(StateRestoring)); s.Keyring != KeyringWaiting {
		t.Fatalf("a retry should say at once that it waits for the keyring: %+v", s)
	}
	if s := rec.wait(t, func(s Status) bool { return s.State != StateRestoring }); s.State != StateSignedIn || s.Keyring != KeyringReady {
		t.Fatalf("after RetryKeyring: %+v", s)
	}
	if n := opens.Load(); n != 2 {
		t.Fatalf("store opened %d times, want a fresh open for the retry", n)
	}
	m.RetryKeyring() // signed in: nothing to retry
	time.Sleep(20 * time.Millisecond)
	if s := m.Status(); s.State != StateSignedIn || opens.Load() != 2 {
		t.Fatalf("RetryKeyring while signed in: %+v", s)
	}
}

func TestDismissedPromptFallsBackVisiblyAndRetryUsesKeyring(t *testing.T) {
	e := newEnv(t)
	k := newHungKeyring()
	k.put(t, goodMUT)
	e.cacheAccount(t, Account{StorefrontID: "gb"})
	k.setFail(errors.New("failed to unlock correct collection '/org/freedesktop/secrets/aliases/default'"))
	e.useKeyring(t, k)
	m, rec := e.manager(t)

	// The existing, explicit fallback: signed out on the 0600 file, which
	// Status.Backend reports so Settings can say so.
	s := start(t, m, rec)
	if s.State != StateSignedOut || s.Backend != store.BackendFile {
		t.Fatalf("after the dismissed prompt: %+v", s)
	}

	k.setFail(nil) // unlocked this time
	m.RetryKeyring()
	s = rec.wait(t, func(s Status) bool { return s.State != StateRestoring })
	if s.State != StateSignedIn || s.Backend != store.SystemBackend || m.UserToken() != goodMUT {
		t.Fatalf("after RetryKeyring: %+v", s)
	}
}

func TestSignInWriteTimesOutAndALateWriteIsUndone(t *testing.T) {
	e := newEnv(t)
	k := newHungKeyring()
	dir := e.useKeyring(t, k)
	e.keyringSlow, e.keyringTimeout = 5*time.Millisecond, 80*time.Millisecond
	m, rec := e.manager(t)
	start(t, m, rec)

	k.hang()
	done := make(chan error, 1)
	go func() { done <- m.SignInWithToken(context.Background(), goodMUT) }()
	if s := rec.wait(t, func(s Status) bool { return s.Keyring == KeyringWaiting }); s.State != StateValidating {
		t.Fatalf("waiting status = %+v", s)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrKeyringTimeout) || !errors.Is(err, ErrKeyring) {
			t.Fatalf("SignInWithToken = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sign-in waited past the keyring timeout")
	}
	if s := m.Status(); s.State != StateSignedOut || s.Keyring != KeyringReady || m.UserToken() != "" {
		t.Fatalf("after the failed write: %+v", s)
	}
	noFallbackFile(t, dir)

	// The write lands late; the queued undo follows it.
	n := k.answered()
	k.release()
	if ops := k.lastOps(t, n+2); ops[len(ops)-2] != "set mut" || ops[len(ops)-1] != "delete mut" {
		t.Fatalf("keyring calls after the release: %v", ops)
	}
	if k.token() != "" {
		t.Fatal("the abandoned sign-in stayed in the keyring")
	}
}

func TestCancelSignInWhileTheKeyringHangs(t *testing.T) {
	e := newEnv(t)
	k := newHungKeyring()
	k.put(t, revokedMUT)
	e.cacheAccount(t, Account{StorefrontID: "gb"})
	e.useKeyring(t, k)
	e.keyringSlow = 5 * time.Millisecond
	m, rec := e.manager(t)
	start(t, m, rec)
	rec.wait(t, is(StateNeedsReauth))

	k.hang()
	done := make(chan error, 1)
	go func() { done <- m.SignInWithToken(context.Background(), goodMUT) }()
	rec.wait(t, func(s Status) bool { return s.Keyring == KeyringWaiting })
	m.CancelSignIn()
	select {
	case err := <-done:
		if !errors.Is(err, mut.ErrCanceled) {
			t.Fatalf("SignInWithToken = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel didn't end the wait for the keyring")
	}
	if s := m.Status(); s.State != StateNeedsReauth {
		t.Fatalf("after cancel: %+v", s)
	}

	// The late write is undone back to the token the manager still has.
	n := k.answered()
	k.release()
	if ops := k.lastOps(t, n+2); ops[len(ops)-2] != "set mut" || ops[len(ops)-1] != "set mut" {
		t.Fatalf("keyring calls after the release: %v", ops)
	}
	if k.token() != revokedMUT {
		t.Fatal("the cancelled sign-in replaced the stored token")
	}
}

func TestLateRestoreDoesNotUndoSignOut(t *testing.T) {
	e := newEnv(t)
	k := newHungKeyring()
	k.put(t, goodMUT)
	e.cacheAccount(t, Account{StorefrontID: "gb"})
	e.useKeyring(t, k)
	e.keyringSlow, e.keyringTimeout = 5*time.Millisecond, 50*time.Millisecond
	k.hang()
	m, rec := e.manager(t)
	m.Start()
	rec.wait(t, func(s Status) bool { return s.Keyring == KeyringWaiting })

	err := m.SignOut(context.Background())
	if !errors.Is(err, ErrKeyringTimeout) {
		t.Fatalf("SignOut = %v", err)
	}
	if s := m.Status(); s.State != StateSignedOut || !strings.Contains(s.LastError, "sign-out incomplete") {
		t.Fatalf("after SignOut: %+v", s)
	}

	// The read answers with the token, then the queued delete runs.
	k.release()
	k.lastOps(t, 3) // the probe, the read, the delete
	if k.token() != "" {
		t.Fatal("the stored token survived the sign-out")
	}
	time.Sleep(20 * time.Millisecond)
	if s := m.Status(); s.State != StateSignedOut || m.UserToken() != "" {
		t.Fatalf("the late read undid the sign-out: %+v", s)
	}
}
