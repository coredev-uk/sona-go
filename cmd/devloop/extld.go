package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/coredev-uk/sona-go/internal/fsutil"
)

// A Go program with cgo (SQLite, the playback host's C++ shim) is linked by
// the system's C++ compiler, and before it links, the Go linker asks that
// compiler half a dozen times whether it knows a flag (--build-id,
// -no-pie…) by linking a trivial program: with nix's compiler wrapper that is
// about 0.3 s each, more than half of a restart's link. The answers never
// change for one compiler, so devloop makes `go build` link through itself
// (-extld=devloop, DEVLOOP_EXTLD naming the real linker): a probe is
// answered from a file after the first time, the real link is passed on.

const extldEnv = "DEVLOOP_EXTLD"

// extld is devloop acting as the external linker; args are the compiler's.
func extld(real string, args []string) int {
	probe := probeKey(real, args)
	if probe != "" {
		if r, ok := loadProbe(probe); ok {
			_, _ = os.Stdout.WriteString(r.Output)
			return r.Code
		}
	}
	cmd := exec.Command(real, args...)
	cmd.Stdin = os.Stdin
	if probe == "" {
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return exitCode(cmd.Run())
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	r := probeResult{Code: exitCode(cmd.Run()), Output: out.String()}
	storeProbe(probe, r)
	_, _ = os.Stdout.WriteString(r.Output)
	return r.Code
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 1
}

type probeResult struct {
	Code   int
	Output string
}

// probeKey is the cache key of a call the Go linker makes to ask about a
// flag (it links its own trivial.c into a.out in its temporary directory),
// "" for any other call.
func probeKey(real string, args []string) string {
	trivial := false
	norm := make([]string, len(args))
	for i, a := range args {
		norm[i] = a
		switch filepath.Base(a) {
		case "trivial.c":
			trivial = true
			norm[i] = "trivial.c"
		case "a.out":
			norm[i] = "a.out"
		}
		if strings.HasPrefix(a, "@") { // an argument file: not a probe's
			return ""
		}
	}
	if !trivial {
		return ""
	}
	path, err := exec.LookPath(real)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(path + "\x00" + strings.Join(norm, "\x00")))
	return hex.EncodeToString(sum[:])
}

func probeFile(key string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "sona-devloop", "extld", key+".json")
}

func loadProbe(key string) (probeResult, bool) {
	var r probeResult
	path := probeFile(key)
	if path == "" {
		return r, false
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &r) != nil {
		return r, false
	}
	return r, true
}

func storeProbe(key string, r probeResult) {
	path := probeFile(key)
	if path == "" || os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	data, _ := json.Marshal(r)
	_ = fsutil.WriteFileAtomic(path, data, 0o644)
}
