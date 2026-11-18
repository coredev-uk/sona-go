// Package wasmtest builds the WebAssembly add-on Sona's tests run
// (testdata/module) for GOOS=wasip1, once per test process.
package wasmtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	once  sync.Once
	bytes []byte
	err   error
)

// Module is the test add-on's WebAssembly module. Its behaviour is picked by
// the LC_WASMTEST_MODE variable in the environment the host gives it (LC_*
// is on the allowlist, which is how a test reaches a module); see
// testdata/module/main.go. The build needs the Go toolchain, which every
// `go test` run has.
func Module(t testing.TB) []byte {
	t.Helper()
	once.Do(func() { bytes, err = build() })
	if err != nil {
		t.Fatalf("building the wasm test add-on: %v", err)
	}
	return bytes
}

func build() ([]byte, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "sona-wasmtest")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	_, file, _, _ := runtime.Caller(0)
	pkg := filepath.Dir(file)
	out := filepath.Join(dir, "module.wasm")
	cmd := exec.Command(goBin, "build", "-trimpath", "-ldflags=-s -w", "-o", out, "./testdata/module")
	cmd.Dir = pkg
	// GOFLAGS carries the repository's build tags, which a wasip1 build of
	// this package doesn't want; cgo does not exist for wasip1.
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0", "GOFLAGS=")
	if b, err := cmd.CombinedOutput(); err != nil {
		return nil, &buildError{err, string(b)}
	}
	return os.ReadFile(out)
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }
