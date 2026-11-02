//go:build cgo

package cdmhost

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// osTestChildMode runs the macOS test child mode "probe": it enters the
// sandbox like the real child, tries what code in the CDM might, and
// prints each outcome ("ok": it worked) as JSON.
func osTestChildMode(m string) {
	if m != "probe" {
		return
	}
	sb := enterSandbox()
	outcome := func(err error) string {
		if err == nil {
			return "ok"
		}
		return err.Error()
	}
	res := map[string]string{"seatbelt": outcome(nil)}
	if !sb.Seatbelt {
		res["seatbelt"] = "not under sandbox-exec"
	}
	c, err := net.DialTimeout("tcp", "127.0.0.1:9", 2*time.Second)
	if err == nil {
		_ = c.Close()
	}
	res["tcp connect"] = outcome(err)
	home := os.Getenv("HOME")
	f, err := os.CreateTemp(home, "sona-write-")
	if err == nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}
	res["home write"] = outcome(err)
	_, err = os.ReadDir(filepath.Join(home, "Library", "Keychains"))
	res["keychain folder"] = outcome(err)
	f, err = os.CreateTemp("", "sona-tmp-")
	if err == nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}
	res["temp write"] = outcome(err)
	_ = json.NewEncoder(os.Stdout).Encode(res)
	os.Exit(0)
}

// TestSeatbeltConfinesChild starts a real child through spawn and checks
// its report, then a probe child under the same profile: no network, no
// writes to the home folder, no keychain folder, but temporary files.
// Without sandbox-exec (or with SONA_SEATBELT=0) it checks the child says
// why it runs unconfined.
func TestSeatbeltConfinesChild(t *testing.T) {
	h, err := spawn(Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := h.call(ctx, request{Op: opInit, CDMPath: "/nonexistent/libwidevinecdm.dylib", Sink: "fakesink"})
	if err == nil || r.Info == nil {
		t.Fatalf("init with a missing CDM: %v %+v", err, r.Info)
	}
	sb := h.withSandbox(r.Info.Sandbox)
	t.Logf("sandbox %+v", sb)
	paths, ps := seatbelt()
	if reason := ps.reason; reason != "" {
		if sb.Seatbelt || sb.Active() || sb.Reason != reason || reason == "" {
			t.Fatalf("unconfined child: sandbox %+v, want the reason %q", sb, reason)
		}
		t.Skip("no macOS sandbox here:", reason)
	}
	if !sb.Active() || sb.Reason != "" {
		t.Fatalf("sandbox %+v, want every layer", sb)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := append(childEnv(slog.New(slog.DiscardHandler)), envTestChild+"=probe", envSeatbeltChild+"=1")
	p, err := startChild([]string{sandboxExec, "-p", seatbeltProfile(paths), exe}, env, nil)
	if err != nil {
		t.Fatal(err)
	}
	var res map[string]string
	derr := json.NewDecoder(p.stdout).Decode(&res)
	_ = p.stdin.Close()
	_ = p.cmd.Wait()
	if derr != nil {
		t.Fatalf("probe child: %v", derr)
	}
	t.Logf("probe: %v", res)
	for _, denied := range []string{"tcp connect", "home write", "keychain folder"} {
		if res[denied] == "ok" {
			t.Errorf("%s worked under the sandbox", denied)
		}
	}
	for _, allowed := range []string{"seatbelt", "temp write"} {
		if res[allowed] != "ok" {
			t.Errorf("%s: %s", allowed, res[allowed])
		}
	}
}
