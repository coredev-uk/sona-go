// Package privatemode is Sona's private listening switch. While it is on,
// the plays Sona reports to Apple carry "private-enabled": true (what
// MusicKit JS's privateEnabled does, so Apple keeps them out of the
// listening history and recommendations), and Sona sends no Last.fm or
// ListenBrainz scrobbles, shows no Discord status and writes nothing to the
// local listening history.
//
// Mode is the one source of truth. Every consumer asks it when it acts on a
// play, through Since with the start of that listen, so a toggle applies
// from the next event: turning private mode on keeps the rest of the listen
// in progress private (what was already sent, such as its start, is not
// recalled), and turning it off leaves that listen private until it ends,
// so a listen never goes public halfway.
//
// The effective state comes from one of three sources:
//
//   - SourceEnvironment: SONA_PRIVATE=1 forces it on in any build; it
//     cannot be turned off for the run.
//   - SourceDevelopment: a development build (no `production` tag) starts
//     private whatever the saved setting says. Toggling changes this
//     session only and never writes the setting. SONA_PRIVATE=0 opts out,
//     for a deliberate live check.
//   - SourceSetting: otherwise the saved setting (config.Private) decides,
//     and toggling writes it.
package privatemode

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// EnvVar overrides private mode for one run (see Env).
const EnvVar = "SONA_PRIVATE"

// Source is where the effective state comes from.
type Source string

// Sources.
const (
	SourceSetting     Source = "setting"
	SourceDevelopment Source = "development"
	SourceEnvironment Source = "environment"
)

// ErrForced is returned when turning off private mode that SONA_PRIVATE=1
// forces on.
var ErrForced = errors.New("private mode is forced on by " + EnvVar + "=1")

// State is the effective private mode and where it comes from.
type State struct {
	On     bool   `json:"on"`
	Source Source `json:"source"`
}

// Override is an SONA_PRIVATE value.
type Override int

// Overrides.
const (
	OverrideNone Override = iota // unset or empty: the build's default
	OverrideOn                   // forced on
	OverrideOff                  // a development build follows the setting
)

// Env parses an SONA_PRIVATE value: "1", "true", "on" or "yes" force private
// mode on; "0", "false", "off" or "no" make a development build follow the
// saved setting; "" means no override. Any other value forces it on and
// returns an error to log, so a typo never makes a run report plays.
func Env(v string) (Override, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return OverrideNone, nil
	case "1", "true", "on", "yes":
		return OverrideOn, nil
	case "0", "false", "off", "no":
		return OverrideOff, nil
	}
	return OverrideOn, fmt.Errorf("%s=%q is not 0 or 1: private mode is forced on", EnvVar, v)
}

// Options configures a Mode.
type Options struct {
	// Saved is the saved setting at start.
	Saved bool
	// Save writes the setting; Set calls it only for SourceSetting. A nil
	// Save makes such a Set change the running state only.
	Save func(on bool) error
	// Dev is a development build: it starts private (SourceDevelopment).
	Dev bool
	// Override is SONA_PRIVATE, parsed by Env.
	Override Override
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// Mode is safe for concurrent use.
type Mode struct {
	source Source
	save   func(bool) error
	now    func() time.Time

	setMu sync.Mutex // orders Set calls, which save outside mu

	mu      sync.Mutex
	on      bool
	saved   bool
	lastOff time.Time // when it last turned off; zero if never on

	obsMu sync.Mutex // delivers changes one at a time, in order
	obs   []func(State)
}

// New returns the Mode for o.
func New(o Options) *Mode {
	m := &Mode{save: o.Save, now: o.Now, saved: o.Saved}
	if m.now == nil {
		m.now = time.Now
	}
	switch {
	case o.Override == OverrideOn:
		m.source, m.on = SourceEnvironment, true
	case o.Dev && o.Override != OverrideOff:
		m.source, m.on = SourceDevelopment, true
	default:
		m.source, m.on = SourceSetting, o.Saved
	}
	return m
}

// State returns the effective state.
func (m *Mode) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return State{On: m.on, Source: m.source}
}

// On reports whether private mode is on now.
func (m *Mode) On() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.on
}

// Since reports whether a listen that started at start is private: private
// mode is on now, or was on at some point since start. A nil Mode is never
// private.
func (m *Mode) Since(start time.Time) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.on || m.lastOff.After(start)
}

// Set turns private mode on or off. With SourceSetting it saves the
// setting first (an error leaves the state unchanged); with
// SourceDevelopment it changes this session only. Turning off a mode
// forced by SONA_PRIVATE=1 fails with ErrForced.
func (m *Mode) Set(on bool) (State, error) {
	m.setMu.Lock()
	defer m.setMu.Unlock()
	switch m.source {
	case SourceEnvironment:
		if !on {
			return m.State(), ErrForced
		}
		return m.State(), nil
	case SourceSetting:
		if m.save != nil {
			// Unlocked: saving runs the settings observers, which call
			// SavedChanged.
			if err := m.save(on); err != nil {
				return m.State(), err
			}
		}
		m.mu.Lock()
		m.saved = on
		m.mu.Unlock()
	}
	m.apply(on)
	return m.State(), nil
}

// Toggle flips private mode (see Set).
func (m *Mode) Toggle() (State, error) {
	m.setMu.Lock()
	on := !m.On()
	m.setMu.Unlock()
	return m.Set(on)
}

// SavedChanged follows the saved setting changed elsewhere (Settings, an
// import). Only SourceSetting applies it.
func (m *Mode) SavedChanged(on bool) {
	m.mu.Lock()
	m.saved = on
	follow := m.source == SourceSetting
	m.mu.Unlock()
	if follow {
		m.apply(on)
	}
}

// Saved returns the saved setting as last seen.
func (m *Mode) Saved() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saved
}

// OnChange registers fn to run after every change of the effective state,
// outside the Mode's lock, one change at a time.
func (m *Mode) OnChange(fn func(State)) {
	m.obsMu.Lock()
	m.obs = append(m.obs, fn)
	m.obsMu.Unlock()
}

func (m *Mode) apply(on bool) {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	m.mu.Lock()
	if m.on == on {
		m.mu.Unlock()
		return
	}
	m.on = on
	if !on {
		m.lastOff = m.now()
	}
	st := State{On: on, Source: m.source}
	m.mu.Unlock()
	for _, fn := range m.obs {
		fn(st)
	}
}
