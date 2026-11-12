package mediatest

import (
	"os"
	"runtime"
	"testing"
)

// The main goroutine stays on the main thread, for Main to run its run loop
// there.
func init() { runtime.LockOSThread() }

// Main runs the tests and exits with their status. On macOS it runs them
// while the main thread runs its run loop, which AVFoundation needs to load
// and play a video (MyGo's ui/video does it through the main queue; an app
// has the run loop of AppKit, a test binary has none):
//
//	func TestMain(m *testing.M) { mediatest.Main(m) }
//
// A package whose tests open a video with ui/video, or media.OpenMotion, has
// one.
func Main(m *testing.M) {
	if runLoop == nil {
		os.Exit(m.Run())
	}
	go func() { os.Exit(m.Run()) }()
	runLoop()
}
