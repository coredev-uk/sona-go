//go:build linux

package sandbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// An init that dies before it reports (at the dynamic loader, on a program
// whose libraries its environment doesn't name) said only "EOF", which told a
// user nothing. The error now says how it ended: its exit status, and that
// what it printed is in the add-on's log. 127 is the loader's.
func TestAnInitThatEndsWithoutAReportSaysHowItEnded(t *testing.T) {
	var log strings.Builder
	prep := func(cmd *exec.Cmd) (*launch, error) {
		cmd.Path = "/bin/sh"
		cmd.Args = []string{"sh", "-c", "echo 'error while loading shared libraries: libgstapp-1.0.so.0' >&2; exit 127"}
		cmd.SysProcAttr = plainAttr()
		pr, pw, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		// The child holds the pipe's write end (descriptor 3) and never
		// writes: the report ends in EOF when it exits, as the init's did.
		cmd.ExtraFiles = []*os.File{pw}
		l := &launch{release: func() { _ = pr.Close(); _ = pw.Close() }}
		l.ready = func() error {
			_ = pw.Close()
			_, err := readReport(pr)
			return err
		}
		l.failed = exitedWithoutReport
		return l, nil
	}
	_, err := startUnix(Spec{Dir: t.TempDir(), Stderr: &log}, prep)
	reason, ok := ReasonOf(err)
	if !ok || reason != ReasonSetup {
		t.Fatalf("err = %v, want a setup failure of the sandbox", err)
	}
	for _, want := range []string{"ended without a report", "exit status 127", "add-on's log"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, lacks %q", err, want)
		}
	}
	if strings.HasSuffix(err.Error(), "EOF") {
		t.Errorf("err = %q: says only EOF", err)
	}
	if !strings.Contains(log.String(), "libgstapp-1.0.so.0") {
		t.Errorf("what the init printed did not reach the log: %q", log.String())
	}
}

// An init that hangs is not "ended": it is killed, and the error says it made
// no report in time, not an exit status of Sona's own signal.
func TestAnInitThatHangsIsNotReportedAsExited(t *testing.T) {
	err := exitedWithoutReport(&noReportError{os.ErrDeadlineExceeded}, errors.New("signal: killed"))
	if s := err.Error(); !strings.Contains(s, "no report in") || strings.Contains(s, "signal: killed") {
		t.Errorf("err = %q", s)
	}
	if err := exitedWithoutReport(&notExecError{"x"}, nil); err == nil || strings.Contains(err.Error(), "report") {
		t.Errorf("another error was reworded: %v", err)
	}
}

// What an add-on's loader needs from its environment reaches it through the
// sandbox: LD_LIBRARY_PATH is passed on by the init to the add-on, and the
// folder it names, one of Policy.Read (sandbox.ProgramReads names it), is
// readable there, where the library folders of a Nix or AppImage install are
// outside the system's.
func TestLoaderPathAndItsFolderReachTheAddon(t *testing.T) {
	libs, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(libs, "libexample.so.0")
	if err := os.WriteFile(lib, []byte("not really a library"), 0o644); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := []string{"LD_LIBRARY_PATH=" + libs}
	r := run(t, Policy{Read: ProgramReads(self, env)}, append(env, envProbeSecret+"="+lib)...)
	if r.Secret != "" {
		t.Errorf("the add-on could not read the library folder LD_LIBRARY_PATH names: %s", r.Secret)
	}
	found := false
	for _, kv := range r.Env {
		found = found || kv == "LD_LIBRARY_PATH="+libs
	}
	if !found {
		t.Errorf("LD_LIBRARY_PATH did not reach the add-on: %v", r.Env)
	}
}
