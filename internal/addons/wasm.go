package addons

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sysfs"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// WebAssembly add-ons (manifest `runtime: "wasm"`) run inside Sona's process
// on wazero, a WebAssembly runtime written in Go (no cgo). The protocol is
// the one a native add-on speaks, over the module's WASI standard input and
// output, so the session code serves both unchanged.
//
// A module has no ambient authority. WASI gives it only what the host hands
// it: its standard streams, the clock, random bytes, its arguments and
// environment, and the files of the directories preopened for it. Sona
// preopens one, the add-on's data folder (confinedFS), and passes no
// network: WASI preview 1 has no sockets. That is why a module needs no OS
// sandbox, and why `network` is a permission it can't have.
//
// It also runs in Sona's address space, so its CPU and memory are Sona's.
// What is capped, in WasmLimits: its linear memory, and the CPU it may
// burn. What is not: see docs/addons.md, "WebAssembly add-ons".

// WasmLimits are what a module may use. The defaults suit a small
// integration; tests lower them.
type WasmLimits struct {
	// MemoryPages caps the module's linear memory, in 64 KiB pages. A growth
	// past it fails as it would on a machine out of memory. 64 MiB holds a
	// Go module (whose runtime wants some 20 MiB) with room to spare, while
	// a leak or a hostile allocation costs Sona no more than that, and the
	// memory is a Go slice, so the total over a few add-ons stays small.
	MemoryPages uint32
	// CPUShare is the share of one core a module may use for long; CPUBurst
	// is how much it may save up and spend at once. The module's own time
	// counts, the time it waits in a host call (a read of its input, a
	// sleep) does not. A module that exhausts the budget is stopped.
	CPUShare float64
	CPUBurst time.Duration
	// Tick is how often the budget is looked at.
	Tick time.Duration
}

// DefaultWasmLimits are 64 MiB of memory and a quarter of a core, with a
// burst of five seconds: a loop that never ends is stopped after about
// seven seconds, a module that answers events in milliseconds never notices.
func DefaultWasmLimits() WasmLimits {
	return WasmLimits{MemoryPages: 1024, CPUShare: 0.25, CPUBurst: 5 * time.Second, Tick: 250 * time.Millisecond}
}

// ErrWasmCPU ends a module that used more CPU than its budget.
var ErrWasmCPU = errors.New("stopped: used more CPU than a wasm add-on may (a quarter of a core, for long)")

// WasmHost compiles and runs modules. One is shared by every wasm add-on;
// each run has a runtime of its own, so ending a run frees everything it
// held.
type WasmHost struct {
	cache  wazero.CompilationCache
	limits WasmLimits
	// CacheNote is why compiled code is kept in memory only, "" when it is
	// on disk (OpenWasmHost).
	CacheNote string
}

// NewWasmHost is a host whose compiled modules are cached in cacheDir (see
// openCache), or in memory when cacheDir is "" or can't be used.
func NewWasmHost(cacheDir string, limits WasmLimits) *WasmHost {
	h := &WasmHost{limits: limits}
	if cacheDir != "" {
		c, err := openCache(cacheDir)
		if err == nil {
			h.cache = c
			return h
		}
		h.CacheNote = err.Error()
	}
	h.cache = wazero.NewCompilationCache()
	return h
}

// Close releases the compilation cache. Runs must be over.
func (h *WasmHost) Close() { _ = h.cache.Close(context.Background()) }

// WasmSpec is how to run one module.
type WasmSpec struct {
	// Module is the module's bytes (ReadModule): what is compiled is what
	// was checked.
	Module []byte
	// Name is the module's first argument, its program name.
	Name string
	// Env is the module's environment as KEY=VALUE (WasmEnvironment).
	Env []string
	// DataDir, when set, is the one folder the module may use, seen as
	// /data. Empty: no files at all.
	DataDir string
	// Stderr receives the module's standard error (its log).
	Stderr io.Writer
}

// dataMount is where a module sees its data folder.
const dataMount = "/data"

// WasmProc is a running module. Like Proc, its standard input and output
// are the protocol channel.
type WasmProc struct {
	cancel context.CancelFunc
	ctx    context.Context
	done   chan struct{}
	ready  chan struct{} // closed when the module starts to run (it is compiled first)
	err    error

	// in is the module's standard input (we write), out its standard
	// output (we read).
	inR  *io.PipeReader
	inW  *io.PipeWriter
	outR *io.PipeReader
	outW *io.PipeWriter

	mu        sync.Mutex
	why       error // why the module was killed, when it was not Sona's own stop
	closeOnce sync.Once
}

// Start runs the module on a goroutine of its own: never the caller's, so
// compiling (the first time, a second or two for a large module) and running
// never hold what called it. Whatever goes wrong with the module is its
// exit error, Err.
func (h *WasmHost) Start(spec WasmSpec) (*WasmProc, error) {
	var fsys experimentalsys.FS
	if spec.DataDir != "" {
		f, err := newConfinedFS(spec.DataDir)
		if err != nil {
			return nil, fmt.Errorf("addons: data folder: %w", err)
		}
		fsys = f
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &WasmProc{ctx: ctx, cancel: cancel, done: make(chan struct{}), ready: make(chan struct{})}
	p.inR, p.inW = io.Pipe()
	p.outR, p.outW = io.Pipe()
	go func() {
		defer close(p.done)
		p.err = p.run(h, spec, fsys)
		// The module is over: its streams end, so the session sees the end.
		p.cancel()
		_ = p.inR.Close()
		_ = p.outW.Close()
	}()
	return p, nil
}

func (p *WasmProc) run(h *WasmHost, spec WasmSpec, fsys experimentalsys.FS) error {
	ctx := p.ctx
	cfg := wazero.NewRuntimeConfig().
		WithCompilationCache(h.cache).
		WithMemoryLimitPages(h.limits.MemoryPages).
		WithCloseOnContextDone(true).
		WithDebugInfoEnabled(false)
	rt := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer func() { _ = rt.Close(context.Background()) }()

	m := newMeter()
	// The meter listens to the WASI functions only: it is how the time a
	// module waits in the host is told from the time it computes.
	if _, err := wasi_snapshot_preview1.Instantiate(experimental.WithFunctionListenerFactory(ctx, m), rt); err != nil {
		return fmt.Errorf("wasi: %w", err)
	}
	// At most two cores compile at once: a start that follows the first
	// paint must not take the machine.
	compiled, err := rt.CompileModule(experimental.WithCompilationWorkers(ctx, 2), spec.Module)
	if err != nil {
		return fmt.Errorf("not a module Sona can run: %w", err)
	}
	if _, ok := compiled.ExportedFunctions()["_start"]; !ok {
		return errors.New("no _start: a wasm add-on is a WASI command (build for wasip1)")
	}

	stderr := spec.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	mc := wazero.NewModuleConfig().
		WithName("").
		WithArgs(spec.Name).
		WithStdin(p.inR).WithStdout(p.outW).WithStderr(stderr).
		WithSysWalltime().WithSysNanotime().
		WithNanosleep(p.sleep).WithOsyield(runtime.Gosched).
		WithRandSource(rand.Reader)
	for _, kv := range spec.Env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			mc = mc.WithEnv(k, v)
		}
	}
	if fsys != nil {
		mc = mc.WithFSConfig(wazero.NewFSConfig().(sysfs.FSConfig).WithSysFSMount(fsys, dataMount))
	}

	m.begin()
	close(p.ready)
	go m.watch(ctx, h.limits, func(err error) { p.kill(err) })
	_, err = rt.InstantiateModule(ctx, compiled, mc)
	return p.exitError(err)
}

// exitError turns what InstantiateModule returns into the module's exit
// status: nil for a clean exit.
func (p *WasmProc) exitError(err error) error {
	if err == nil {
		return nil
	}
	var ee *sys.ExitError
	if errors.As(err, &ee) {
		switch code := ee.ExitCode(); code {
		case 0:
			return nil
		case sys.ExitCodeContextCanceled, sys.ExitCodeDeadlineExceeded:
			if why := p.reason(); why != nil {
				return why
			}
			return errors.New("stopped")
		default:
			return fmt.Errorf("exit status %d", code)
		}
	}
	// A trap (unreachable, out of bounds, stack overflow): the first line
	// says what, the rest is a trace.
	line, _, _ := strings.Cut(err.Error(), "\n")
	return errors.New(line)
}

// kill ends the module because of why, which Err then reports.
func (p *WasmProc) kill(why error) {
	p.mu.Lock()
	if p.why == nil {
		p.why = why
	}
	p.mu.Unlock()
	p.Kill()
}

func (p *WasmProc) reason() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.why
}

// sleep is the module's clock sleep: it ends early when the module is
// stopped, so a module asleep in a long poll never holds a stop.
func (p *WasmProc) sleep(ns int64) {
	t := time.NewTimer(time.Duration(ns))
	defer t.Stop()
	select {
	case <-t.C:
	case <-p.ctx.Done():
	}
}

// Conn is the channel as one io.ReadWriteCloser for cli.Session.Serve: it
// reads the module's standard output, writes its standard input, and Close
// ends both (the module sees the end of its input).
func (p *WasmProc) Conn() io.ReadWriteCloser { return wasmConn{p} }

type wasmConn struct{ p *WasmProc }

func (c wasmConn) Read(b []byte) (int, error)  { return c.p.outR.Read(b) }
func (c wasmConn) Write(b []byte) (int, error) { return c.p.inW.Write(b) }
func (c wasmConn) Close() error {
	c.p.closeOnce.Do(func() {
		_ = c.p.inW.Close()
		_ = c.p.outR.Close()
	})
	return nil
}

// Done is closed when the module has ended.
func (p *WasmProc) Done() <-chan struct{} { return p.done }

// Err is how it ended once Done is closed: nil for a clean exit.
func (p *WasmProc) Err() error {
	<-p.done
	return p.err
}

// Ready is closed when the module starts to run, once it is compiled and
// instantiated: the time it has to say hello begins there, not before.
func (p *WasmProc) Ready() <-chan struct{} { return p.ready }

// Where says what runs, for the add-on's log.
func (p *WasmProc) Where() string { return "WebAssembly in Sona" }

// Kill ends the module now: its context is cancelled, which stops it at its
// next loop or call, and its streams close, which frees a read or a write
// it waits in.
func (p *WasmProc) Kill() {
	p.cancel()
	_ = wasmConn{p}.Close()
}

// Stop ends the module as Proc.Stop does a process: its input closes (end
// of input is its signal to leave), it is given grace to do so, then it is
// killed. It returns once the module has ended. There is no second grace
// and no terminate signal: a module that is not leaving is stuck or busy.
func (p *WasmProc) Stop(grace time.Duration) {
	_ = wasmConn{p}.Close()
	t := time.NewTimer(grace)
	defer t.Stop()
	select {
	case <-p.done:
	case <-t.C:
		p.Kill()
		<-p.done
	}
}

// WasmEnvironment is the environment of the module of add-on id. It is
// Environment's allowlist without the variables that name host paths
// (HOME, PATH, TMPDIR and, on Windows, their kin): the module has no host
// paths, and its home would only tell it where the user's is. A module that
// has a data folder sees it at /data in SONA_ADDON_DATA.
func WasmEnvironment(parent []string, id string, data bool) []string {
	out := make([]string, 0, 10)
	for _, kv := range parent {
		name, _, ok := strings.Cut(kv, "=")
		if ok && (name == "LANG" || name == "LANGUAGE" || name == "TZ" || strings.HasPrefix(name, "LC_")) {
			out = append(out, kv)
		}
	}
	out = append(out, "SONA_ADDON_ID="+id, "SONA_ADDON_PROTOCOL=2")
	if data {
		out = append(out, "SONA_ADDON_DATA="+dataMount)
	}
	return out
}

// meter tells how long a module computes, as against waits. It listens to
// the WASI host functions: between a call into the host and its return the
// module waits, and in between the module is running. A module that never
// calls the host is running all the time.
type meter struct {
	mu     sync.Mutex
	inHost bool
	mark   time.Time     // the last switch between the module and the host
	spent  time.Duration // module time up to mark
}

func newMeter() *meter { return &meter{} }

// begin starts counting: from here the module is running.
func (m *meter) begin() {
	m.mu.Lock()
	m.mark = time.Now()
	m.mu.Unlock()
}

// NewFunctionListener implements experimental.FunctionListenerFactory:
// host functions only, never the module's own, which stay unhooked.
func (m *meter) NewFunctionListener(def api.FunctionDefinition) experimental.FunctionListener {
	if def.GoFunction() == nil {
		return nil
	}
	return m
}

func (m *meter) Before(context.Context, api.Module, api.FunctionDefinition, []uint64, experimental.StackIterator) {
	now := time.Now()
	m.mu.Lock()
	if !m.inHost {
		m.spent += now.Sub(m.mark)
	}
	m.inHost, m.mark = true, now
	m.mu.Unlock()
}

func (m *meter) leave() {
	now := time.Now()
	m.mu.Lock()
	m.inHost, m.mark = false, now
	m.mu.Unlock()
}

func (m *meter) After(context.Context, api.Module, api.FunctionDefinition, []uint64) { m.leave() }
func (m *meter) Abort(context.Context, api.Module, api.FunctionDefinition, error)    { m.leave() }

// total is the module's running time to now, the open stretch included.
func (m *meter) total(now time.Time) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inHost {
		return m.spent
	}
	return m.spent + now.Sub(m.mark)
}

// watch is the CPU budget: a bucket of CPUBurst that refills at CPUShare of
// a second per second and is spent by the module's running time. It calls
// stop with ErrWasmCPU when the bucket is empty.
func (m *meter) watch(ctx context.Context, l WasmLimits, stop func(error)) {
	tick := time.NewTicker(l.Tick)
	defer tick.Stop()
	level := l.CPUBurst
	last := time.Now()
	lastSpent := m.total(last)
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			spent := m.total(now)
			// A machine that slept or a process that was paused makes a long
			// tick; one core can't have run longer than the tick that passed.
			dt := min(now.Sub(last), 4*l.Tick)
			used := min(spent-lastSpent, dt)
			last, lastSpent = now, spent
			level = min(l.CPUBurst, level+time.Duration(l.CPUShare*float64(dt))) - used
			if level <= 0 {
				stop(ErrWasmCPU)
				return
			}
		}
	}
}
