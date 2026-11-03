package services

// EventPower carries PowerState when the computer's power source changes.
const EventPower = "power:changed"

// PowerState is the computer's power source as Sona decides it from
// UPower's devices (internal/power): motion artwork stops on battery.
type PowerState struct {
	// Followed: Sona reads the power source (UPower on Linux). False
	// where the platform has no monitor or UPower is not running; the
	// computer then counts as on mains power.
	Followed  bool `json:"followed"`
	OnBattery bool `json:"onBattery"`
	// Source is what decided: "ac" (an AC adapter is online), "battery"
	// (the computer's own battery), "ups" (a UPS is discharging), "mains"
	// (no adapter reported and nothing that powers the computer is
	// discharging: a desktop); "" when not followed.
	Source string `json:"source"`
	// UPower is UPower's own on-battery verdict, which Sona does not use
	// (it counts a UPS in an unknown state as discharging, for one).
	UPower bool `json:"upower"`
	// Ignored counts devices whose battery does not power the computer
	// (mice, keyboards, headsets).
	Ignored int `json:"ignored"`
	// Unreadable: the platform has a power monitor but it couldn't be read
	// (UPower not answering); the log says why.
	Unreadable bool `json:"unreadable"`
}

// MotionStatus is motion artwork's Go side for Settings → Diagnostics:
// the power source, the rendition cache and the newest download.
type MotionStatus struct {
	Power      PowerState `json:"power"`
	CacheFiles int        `json:"cacheFiles"`
	CacheBytes int64      `json:"cacheBytes"`
	// Last is the newest rendition download since Sona started (At 0:
	// none yet).
	Last MotionFetch `json:"last"`
}

// MotionFetch is one rendition download.
type MotionFetch struct {
	At    int64  `json:"at"`    // Unix ms
	Size  int    `json:"size"`  // rendition bucket, device pixels
	Codec string `json:"codec"` // "avc" (H.264) or "hevc"
	// Rendition is the variant chosen ("avc1.64001f 486×486, 1.7
	// Mbit/s"), "" when none was.
	Rendition string    `json:"rendition"`
	Error     ErrorCode `json:"error"` // "" when it worked
}

// MotionReport is the web view's side of motion artwork, which only
// WebKit knows: what decides whether motion plays, which codecs it can
// decode and how the newest video went. Diagnostics sends it before
// building a bug-report bundle, which then carries it.
type MotionReport struct {
	Allowed       bool `json:"allowed"`
	Setting       bool `json:"setting"`
	ReduceEffects bool `json:"reduceEffects"`
	ReducedMotion bool `json:"reducedMotion"`
	OnBattery     bool `json:"onBattery"`
	// H264 and HEVC are HTMLMediaElement.canPlayType's answers for
	// Apple's renditions ("probably", "maybe" or "").
	H264 string `json:"h264"`
	HEVC string `json:"hevc"`
	// Visible: the window's document is visible (videos pause when not).
	Visible bool `json:"visible"`
	// Playing counts the videos playing now.
	Playing int `json:"playing"`
	// Last is the newest video's outcome as Diagnostics shows it, for the
	// bundle (maintainer-facing, never shown in the UI).
	Last string `json:"last"`
}
