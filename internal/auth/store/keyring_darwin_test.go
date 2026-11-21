package store

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/zalando/go-keyring"
)

// throwawayKeychain is a SystemKeyring over a new, unlocked keychain file
// that is deleted after the test: the user's login keychain is never
// touched. User interaction is off for the test, so a call that would make
// macOS ask the user fails instead of waiting for an answer.
func throwawayKeychain(t *testing.T) (SystemKeyring, string) {
	t.Helper()
	if err := secLoad(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sona-test.keychain-db")
	security(t, "create-keychain", "-p", "test", path)
	t.Cleanup(func() { _ = exec.Command("/usr/bin/security", "delete-keychain", path).Run() })

	s, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	var (
		open        func(path string, kc *uintptr) int32
		interaction func(allowed bool) int32
	)
	purego.RegisterLibFunc(&open, s, "SecKeychainOpen")
	purego.RegisterLibFunc(&interaction, s, "SecKeychainSetUserInteractionAllowed")
	var kc uintptr
	if err := secStatus("open", open(path, &kc)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sec.release(kc) })
	if err := secStatus("set user interaction", interaction(false)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { interaction(true) })
	return SystemKeyring{keychain: kc, keychainPath: path}, path
}

// testExe is the test binary's path as the keychain records it (symlinks
// resolved: /tmp is /private/tmp).
func testExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

// security runs /usr/bin/security, failing the test on an error. It is cut
// off after a while: a command that waits for a keychain prompt is a failure.
func security(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/security", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("security %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// decryptApps lists the applications the access list of the keychain's
// only item lets read its secret without asking (the "decrypt"
// authorisation), as `security dump-keychain -a` prints them.
func decryptApps(t *testing.T, path string) []string {
	t.Helper()
	var apps []string
	inDecrypt := false
	sc := bufio.NewScanner(strings.NewReader(security(t, "dump-keychain", "-a", path)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "authorizations"):
			inDecrypt = strings.Contains(line, " decrypt")
		case !inDecrypt:
		case line == "applications: <null>":
			apps = append(apps, "<any>")
		case line != "" && line[0] >= '0' && line[0] <= '9' && strings.Contains(line, ": "):
			_, app, _ := strings.Cut(line, ": ")
			app, _, _ = strings.Cut(app, " (")
			apps = append(apps, app)
		}
	}
	return apps
}

func wantOnlyThisBinary(t *testing.T, path string) {
	t.Helper()
	if apps, exe := decryptApps(t, path), testExe(t); len(apps) != 1 || apps[0] != exe {
		t.Fatalf("applications trusted to read the item: %q, want only %q", apps, exe)
	}
}

func TestKeychainRoundTrip(t *testing.T) {
	kr, _ := throwawayKeychain(t)
	if _, err := kr.Get(Service, Key); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("Get of a missing item: %v, want keyring.ErrNotFound", err)
	}
	for _, v := range []string{`{"token":"a"}`, "two\nlines, ünïcode"} {
		if err := kr.Set(Service, Key, v); err != nil {
			t.Fatal(err)
		}
		if got, err := kr.Get(Service, Key); err != nil || got != v {
			t.Fatalf("Get = %q, %v; want %q", got, err, v)
		}
	}
	if err := kr.Set(Service, "lastfm", "other"); err != nil {
		t.Fatal(err)
	}
	if err := kr.Delete(Service, Key); err != nil {
		t.Fatal(err)
	}
	if err := kr.Delete(Service, Key); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("second Delete: %v, want keyring.ErrNotFound", err)
	}
	if got, err := kr.Get(Service, "lastfm"); err != nil || got != "other" {
		t.Fatalf("another key: %q, %v", got, err)
	}
}

// An item Sona writes lets Sona read it and no other program, not
// /usr/bin/security, which any program can run (GHSA-49cj-524r-6gp3).
func TestKeychainItemTrustsOnlySona(t *testing.T) {
	kr, path := throwawayKeychain(t)
	if err := kr.Set(Service, Key, "secret"); err != nil {
		t.Fatal(err)
	}
	wantOnlyThisBinary(t, path)
	if err := kr.Set(Service, Key, "replaced"); err != nil {
		t.Fatal(err)
	}
	wantOnlyThisBinary(t, path)
}

// goKeyringItem adds an item as go-keyring v0.2.8 did: through
// /usr/bin/security, so that only /usr/bin/security may read it, with the
// secret encoded behind go-keyring's prefix.
func goKeyringItem(t *testing.T, path, user, stored string) {
	t.Helper()
	security(t, "add-generic-password", "-U", "-s", Service, "-a", user, "-w", stored, path)
	if apps := decryptApps(t, path); len(apps) != 1 || apps[0] != "/usr/bin/security" {
		t.Fatalf("go-keyring's item trusts %q, want /usr/bin/security", apps)
	}
}

// A secret go-keyring stored is read without a prompt and moved into an
// item only Sona may read.
func TestKeychainMovesGoKeyringItems(t *testing.T) {
	const secret = `{"token":"mut"}`
	for name, stored := range map[string]string{
		"base64": goKeyringBase64Prefix + base64.StdEncoding.EncodeToString([]byte(secret)),
		"hex":    goKeyringHexPrefix + hex.EncodeToString([]byte(secret)),
	} {
		t.Run(name, func(t *testing.T) {
			kr, path := throwawayKeychain(t)
			goKeyringItem(t, path, Key, stored)
			if got, err := kr.Get(Service, Key); err != nil || got != secret {
				t.Fatalf("Get = %q, %v; want %q", got, err, secret)
			}
			wantOnlyThisBinary(t, path)
			if got, err := kr.read(Service, Key); err != nil || got != secret {
				t.Fatalf("Sona's item holds %q, %v; want the plain secret", got, err)
			}
		})
	}
}

// Set and Delete replace or remove a go-keyring item too, so a secret
// written or deleted before any Get leaves no item /usr/bin/security reads.
func TestKeychainSetAndDeleteReplaceGoKeyringItems(t *testing.T) {
	kr, path := throwawayKeychain(t)
	goKeyringItem(t, path, Key, goKeyringBase64Prefix+"b2xk")
	if err := kr.Set(Service, Key, "new"); err != nil {
		t.Fatal(err)
	}
	wantOnlyThisBinary(t, path)
	if got, err := kr.Get(Service, Key); err != nil || got != "new" {
		t.Fatalf("Get = %q, %v", got, err)
	}

	kr, path = throwawayKeychain(t)
	goKeyringItem(t, path, Key, goKeyringBase64Prefix+"b2xk")
	if err := kr.Delete(Service, Key); err != nil {
		t.Fatal(err)
	}
	if err := kr.exists(Service, Key); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("after Delete the item is still there: %v", err)
	}
}

// An item an earlier build created, which this build may not delete, has
// its secret replaced by the tombstone, which reads as no item.
func TestKeychainDeleteTombstonesAnItemItMayNotDelete(t *testing.T) {
	kr, path := throwawayKeychain(t)
	// Sona's creator code, but created and owned by /usr/bin/security: an
	// item this test binary may neither read nor delete.
	security(t, "add-generic-password", "-c", "Sona", "-s", Service, "-a", Key, "-w", "old-token", path)
	if _, err := kr.Get(Service, Key); err == nil || errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("Get of an item this build may not read: %v, want an access error", err)
	}
	if err := kr.Delete(Service, Key); err != nil {
		t.Fatal(err)
	}
	// security prints a secret with a NUL byte in hex.
	if got, want := strings.TrimSpace(security(t, "find-generic-password", "-s", Service, "-a", Key, "-w", path)), hex.EncodeToString([]byte(tombstone)); got != want {
		t.Fatalf("after Delete the item holds %q, want the tombstone %q", got, want)
	}

	kr, _ = throwawayKeychain(t)
	if err := kr.Set(Service, Key, tombstone); err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Get(Service, Key); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("Get of a tombstone: %v, want keyring.ErrNotFound", err)
	}
	if err := kr.Set(Service, Key, "again"); err != nil {
		t.Fatal(err)
	}
	if got, err := kr.Get(Service, Key); err != nil || got != "again" {
		t.Fatalf("Get after Set over a tombstone = %q, %v", got, err)
	}
}
