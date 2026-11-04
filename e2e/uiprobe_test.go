//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/e2e/automation"
)

// TestUIProbe is CI's post-merge measurement (the uiprobe job), not a check:
// it only runs with E2E_UIPROBE=1 and E2E_OUT set. It starts the app five
// times for `uiprobe -firstpaint`, then once more for the Albums scroll table
// (cmd/uiprobe against the app's automation socket and its sona.log, with
// MYGO_FRAME_STATS=all), and writes both tables as Markdown into E2E_OUT.
func TestUIProbe(t *testing.T) {
	if os.Getenv("E2E_UIPROBE") != "1" {
		t.Skip("E2E_UIPROBE=1 runs the uiprobe measurement")
	}
	out := os.Getenv("E2E_OUT")
	if out == "" {
		t.Fatal("E2E_OUT: the directory the tables are written into")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	probeBin := filepath.Join(root, "bin", "uiprobe")
	build := exec.Command("go", "build", "-o", probeBin, "./cmd/uiprobe")
	build.Dir = root
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/uiprobe: %v\n%s", err, b)
	}
	t.Setenv("MYGO_FRAME_STATS", "all")

	const starts = 5
	var logs []string
	for i := 1; i <= starts; i++ {
		name := fmt.Sprintf("start%d", i)
		t.Run(name, func(t *testing.T) {
			a := startApp(t)
			until(t, startTimeout, "first paint", func() bool {
				return strings.Contains(a.log(), "startup: first paint")
			})
		})
		logs = append(logs, filepath.Join(out, "TestUIProbe_"+name+"-sona.log"))
	}
	fp, err := exec.Command(probeBin, append([]string{"-firstpaint"}, logs...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("uiprobe -firstpaint: %v\n%s", err, fp)
	}
	writeTable(t, filepath.Join(out, "uiprobe-firstpaint.md"), string(fp))

	t.Run("albums", func(t *testing.T) {
		a := startApp(t)
		// The app serves one automation client: startApp's connection
		// has proved it is up, and uiprobe takes the socket over.
		if _, err := a.client.Wait(startTimeout, "the Albums page", func(tr *automation.Tree) bool { return tr.HasText("24 albums") }); err != nil {
			t.Fatal(err)
		}
		_ = a.client.Close()
		sock := ""
		for _, e := range a.cmd.Env {
			if v, ok := strings.CutPrefix(e, "MYGO_AUTOMATION="); ok {
				sock = v
			}
		}
		cmd := exec.Command(probeBin, "-socket", sock, "-log", a.paths.LogFile(), "-runs", "3", "-idle", "10s")
		cmd.Env = append(os.Environ(), "SWAYSOCK="+os.Getenv("SWAYSOCK"))
		var stdout, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		deadline := time.AfterFunc(5*time.Minute, func() { _ = cmd.Process.Kill() })
		defer deadline.Stop()
		err = cmd.Run()
		writeTable(t, filepath.Join(out, "uiprobe-albums.md"), stdout.String())
		writeTable(t, filepath.Join(out, "uiprobe-albums-stderr.txt"), stderr.String())
		if err != nil {
			t.Fatalf("uiprobe: %v\n%s", err, stderr.String())
		}
	})
}

func writeTable(t *testing.T, name, s string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}
