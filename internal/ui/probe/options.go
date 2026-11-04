package probe

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Options are the probe's flags.
type Options struct {
	// Probe measures the scroll; Startup only the time until the covers
	// are on screen.
	Probe, Startup bool
	// Speed is the scroll speed in DIPs per second (2000), Max how far down
	// it goes at most (20000; the page's end if that is nearer), Runs the
	// passes (3) and Idle how long the process is watched at rest (10 s).
	Speed, Max float64
	Runs       int
	Idle       time.Duration
	// Interval is the display's frame interval in milliseconds (0:
	// measured).
	Interval float64
}

// On is whether the process was asked to probe.
func (o Options) On() bool { return o.Probe || o.Startup }

// Parse takes the probe's flags out of args and returns the rest, which
// are Sona's own. Flags take `-name value` and `-name=value`.
func Parse(args []string) (Options, []string, error) {
	o := Options{Speed: 2000, Max: 20000, Runs: 3, Idle: 10 * time.Second}
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") {
			rest = append(rest, a)
			continue
		}
		value := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("-%s needs a value", name)
			}
			i++
			return args[i], nil
		}
		var err error
		switch name {
		case "probe", "startup":
			on := true
			if hasVal {
				on, err = strconv.ParseBool(val)
			}
			if name == "probe" {
				o.Probe = on
			} else {
				o.Startup = on
			}
		case "speed", "max", "interval":
			var v string
			if v, err = value(); err == nil {
				var f float64
				if f, err = strconv.ParseFloat(v, 64); err == nil && f <= 0 {
					err = errors.New("must be positive")
				}
				switch name {
				case "speed":
					o.Speed = f
				case "max":
					o.Max = f
				default:
					o.Interval = f
				}
			}
		case "runs":
			var v string
			if v, err = value(); err == nil {
				if o.Runs, err = strconv.Atoi(v); err == nil && o.Runs < 1 {
					err = errors.New("must be at least 1")
				}
			}
		case "idle":
			var v string
			if v, err = value(); err == nil {
				o.Idle, err = time.ParseDuration(v)
			}
		default:
			rest = append(rest, a)
		}
		if err != nil {
			return o, nil, fmt.Errorf("-%s: %w", name, err)
		}
	}
	return o, rest, nil
}
