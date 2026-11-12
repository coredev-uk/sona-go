//go:build cgo && (darwin || windows)

package cdmhost

import "github.com/coredev-uk/sona-go/internal/playback"

// RealtimeSupported: neither macOS nor Windows has an RTKit to ask. Core
// Audio runs its own I/O thread at realtime priority, and WASAPI's render
// thread registers with the Multimedia Class Scheduler itself, so the
// parent asks for nothing.
const RealtimeSupported = false

// realtimeInitial is every host's Realtime on macOS and Windows.
var realtimeInitial = Realtime{State: playback.RealtimeUnsupported}

// defaultRTEnv asks no one: a report from the child is ignored.
var defaultRTEnv = rtEnv{}

// audioThreadIDs reports no threads: there is nothing to raise them with.
func audioThreadIDs() []int { return nil }
