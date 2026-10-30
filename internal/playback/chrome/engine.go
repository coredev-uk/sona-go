// Package chrome runs the Sona playback engine in a Widevine-capable Chrome
// controlled over the DevTools protocol (via a pipe, not a TCP port).
//
// Linux WebKitGTK (the Wails webview) has no Widevine CDM, so protected
// Apple Music playback runs here instead: a dedicated Chrome profile loads the
// embedded engine page at https://music.apple.com/sona-engine/ (fulfilled
// locally via Fetch interception) and MusicKit JS plays through Chrome's CDM.
package chrome

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"

	"github.com/coredev-uk/sona-go/internal/playback/enginepage"
)

// Mode selects how the engine window is (not) shown.
type Mode string

const (
	// ModeHeadless runs Chrome's headless mode: no window at all.
	ModeHeadless Mode = "headless"
	// ModeWindow shows a normal window; for debugging.
	ModeWindow Mode = "window"
)

// Options configures Start.
type Options struct {
	ChromePath string // empty: search PATH (ChromeCandidates)
	ProfileDir string // persistent user-data-dir (Widevine state, MusicKit storage)
	Mode       Mode
	ExtraFlags map[string]any
	Output     io.Writer // Chrome's stdout/stderr; nil discards
}

// Event is a message emitted by the engine page.
type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Engine is a running Chrome engine.
type Engine struct {
	ctx         context.Context
	cancel      context.CancelFunc
	allocCancel context.CancelFunc
	events      chan Event
	closeOnce   sync.Once
}

// ChromeCandidates are binary names searched when Options.ChromePath is empty.
var ChromeCandidates = []string{"google-chrome-stable", "google-chrome", "chromium", "chromium-browser"}

// FindChrome returns the first Chrome-family binary on PATH.
func FindChrome() (string, error) {
	for _, name := range ChromeCandidates {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no Chrome/Chromium binary found on PATH")
}

// Start launches Chrome and loads the engine page.
func Start(parent context.Context, opts Options) (*Engine, error) {
	if opts.ProfileDir == "" {
		return nil, errors.New("chrome engine: ProfileDir is required")
	}
	if opts.ChromePath == "" {
		p, err := FindChrome()
		if err != nil {
			return nil, err
		}
		opts.ChromePath = p
	}
	if err := os.MkdirAll(opts.ProfileDir, 0o700); err != nil {
		return nil, fmt.Errorf("chrome engine profile: %w", err)
	}

	// Deliberately not chromedp.DefaultExecAllocatorOptions: those add
	// --enable-automation (sets navigator.webdriver), --mute-audio and
	// --disable-background-networking, all wrong for a music player.
	allocOpts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(opts.ChromePath),
		chromedp.UserDataDir(opts.ProfileDir),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("password-store", "basic"),
		chromedp.Flag("autoplay-policy", "no-user-gesture-required"),
		// Go owns MPRIS; stop Chrome registering a second media player on D-Bus.
		chromedp.Flag("disable-features", "MediaSessionService,HardwareMediaKeyHandling,GlobalMediaControls,Translate"),
		// Keep timers and media pipelines running while the engine is invisible.
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-extensions", true),
		// Tag the PipeWire/PulseAudio stream as music (Chrome overrides
		// application.name itself, so only the role sticks).
		chromedp.Env("PULSE_PROP=media.role=music"),
	}
	if opts.Mode == "" || opts.Mode == ModeHeadless {
		allocOpts = append(allocOpts, chromedp.Flag("headless", true), chromedp.Flag("hide-scrollbars", true))
	}
	for k, v := range opts.ExtraFlags {
		allocOpts = append(allocOpts, chromedp.Flag(k, v))
	}
	if opts.Output != nil {
		allocOpts = append(allocOpts, chromedp.CombinedOutput(opts.Output))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(parent, allocOpts...)
	ctx, cancel := chromedp.NewContext(allocCtx)
	e := &Engine{ctx: ctx, cancel: cancel, allocCancel: allocCancel, events: make(chan Event, 64)}

	ready := make(chan struct{})
	var readyOnce sync.Once
	emit := func(_ context.Context, ev Event) (bool, error) {
		if ev.Type == "ready" {
			readyOnce.Do(func() { close(ready) })
		}
		select {
		case e.events <- ev:
		default: // slow consumer: drop rather than stall the page
		}
		return true, nil
	}

	// Starts the browser; the binding exists before the engine page loads.
	if err := chromedp.Do(ctx, chromedp.ExposeFunc("sonaEmit", emit)); err != nil {
		e.Close()
		return nil, fmt.Errorf("chrome engine start: %w", err)
	}
	paused := chromedp.Events(ctx, fetch.RequestPaused)
	go e.servePage(paused)

	if _, err := chromedp.Call(ctx, fetch.Enable, fetch.EnableParams{Patterns: []*fetch.RequestPattern{{
		URLPattern:   enginepage.URL + "*",
		RequestStage: fetch.RequestStageRequest,
	}}}); err != nil {
		e.Close()
		return nil, fmt.Errorf("chrome engine fetch: %w", err)
	}
	if err := chromedp.Do(ctx, chromedp.Navigate(enginepage.URL)); err != nil {
		e.Close()
		return nil, fmt.Errorf("chrome engine navigate: %w", err)
	}
	select {
	case <-ready:
	case <-ctx.Done():
		e.Close()
		return nil, ctx.Err()
	}
	return e, nil
}

// servePage fulfils requests for the engine URL with the embedded page.
func (e *Engine) servePage(paused func(func(fetch.EventRequestPaused, error) bool)) {
	for ev, err := range paused {
		if err != nil {
			return
		}
		if !strings.HasPrefix(ev.Request.URL, enginepage.URL) {
			_, _ = chromedp.Call(e.ctx, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: ev.RequestID})
			continue
		}
		_, _ = chromedp.Call(e.ctx, fetch.FulfillRequest, fetch.FulfillRequestParams{
			RequestID:    ev.RequestID,
			ResponseCode: 200,
			ResponseHeaders: []*fetch.HeaderEntry{
				{Name: "Content-Type", Value: "text/html; charset=utf-8"},
				{Name: "Cache-Control", Value: "no-store"},
			},
			Body: enginepage.HTML,
		})
	}
}

// Events streams engine page events until the engine closes.
func (e *Engine) Events() <-chan Event { return e.events }

// Call invokes window.sona[method](...args) in the engine page, awaits the
// returned promise, and decodes its JSON result into out (if non-nil).
func (e *Engine) Call(ctx context.Context, out any, method string, args ...any) error {
	if args == nil {
		args = []any{}
	}
	argJSON, err := json.Marshal(args)
	if err != nil {
		return err
	}
	expr := fmt.Sprintf("window.sona[%q](...%s)", method, argJSON)

	// Run on the engine's tab but honour the caller's deadline.
	runCtx, stop := context.WithCancel(e.ctx)
	defer stop()
	go func() {
		select {
		case <-ctx.Done():
			stop()
		case <-runCtx.Done():
		}
	}()
	raw, err := chromedp.Run(runCtx, chromedp.Evaluate[[]byte](expr, chromedp.EvalAwaitPromise))
	if err != nil {
		return fmt.Errorf("engine %s: %w", method, err)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// Close shuts Chrome down.
func (e *Engine) Close() {
	e.closeOnce.Do(func() {
		e.cancel()
		e.allocCancel()
	})
}
