// Package power follows whether the computer runs on battery, from
// UPower's devices on the system bus.
//
// UPower's own OnBattery is not used to decide: it is true whenever its
// composite "display device" is discharging, and UPower counts a UPS whose
// state it does not know, or a laptop battery that reports "discharging"
// while the adapter is plugged in, as discharging. Decide reads the power
// sources themselves: an AC adapter that is online means mains power;
// batteries that do not power the computer (mice, keyboards, headsets: a
// Battery with PowerSupply false, or any other device type) never count.
package power

import (
	"fmt"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	upowerName   = "org.freedesktop.UPower"
	upowerPath   = "/org/freedesktop/UPower"
	deviceIface  = upowerName + ".Device"
	devicePrefix = upowerPath + "/devices/"
	propsIface   = "org.freedesktop.DBus.Properties"
	propsSignal  = propsIface + ".PropertiesChanged"
)

// UPower device types (org.freedesktop.UPower.Device Type).
const (
	TypeLinePower = 1
	TypeBattery   = 2
	TypeUPS       = 3
)

// UPower device states (org.freedesktop.UPower.Device State).
const (
	StateDischarging      = 2
	StatePendingDischarge = 6
)

// Device is the part of a UPower device that decides the power source.
type Device struct {
	Type uint32
	// PowerSupply: the device powers the computer (false for the battery
	// of a mouse, keyboard or headset).
	PowerSupply bool
	IsPresent   bool
	// Online: a line power device (an AC adapter) is connected.
	Online bool
	State  uint32
}

func (d Device) discharging() bool {
	return d.State == StateDischarging || d.State == StatePendingDischarge
}

// Sources: what decided State.OnBattery.
const (
	// SourceAC: an AC adapter is online.
	SourceAC = "ac"
	// SourceBattery: the computer's own battery powers it (its adapter is
	// unplugged, or it has none and the battery is discharging).
	SourceBattery = "battery"
	// SourceUPS: a UPS is discharging (mains power is out).
	SourceUPS = "ups"
	// SourceMains: no AC adapter is reported and nothing that powers the
	// computer is discharging: a desktop, or a laptop whose battery is
	// charging or full.
	SourceMains = "mains"
)

// State is the power source and how it was decided.
type State struct {
	OnBattery bool
	Source    string
	// UPower is UPower's own OnBattery, for Diagnostics when it disagrees.
	UPower bool
	// Ignored counts devices with a battery that does not power the
	// computer (peripherals).
	Ignored int
}

// Decide works out the power source from UPower's devices (every device
// but the composite DisplayDevice) and UPower's OnBattery.
func Decide(devs []Device, upower bool) State {
	var adapters, online, batteries int
	var battery, ups bool
	st := State{UPower: upower}
	for _, d := range devs {
		switch {
		case d.Type == TypeLinePower:
			if d.PowerSupply {
				adapters++
				if d.Online {
					online++
				}
			}
		case d.Type == TypeBattery && d.PowerSupply:
			if d.IsPresent {
				batteries++
				battery = battery || d.discharging()
			}
		case d.Type == TypeUPS:
			if d.IsPresent {
				ups = ups || d.discharging()
			}
		case d.Type != 0:
			st.Ignored++
		}
	}
	switch {
	case online > 0:
		st.Source = SourceAC
	case adapters > 0 && batteries > 0:
		// Unplugged: the battery powers the computer whatever state it
		// reports.
		st.OnBattery, st.Source = true, SourceBattery
	case battery:
		st.OnBattery, st.Source = true, SourceBattery
	case ups:
		st.OnBattery, st.Source = true, SourceUPS
	default:
		st.Source = SourceMains
	}
	return st
}

// Monitor follows UPower's devices.
type Monitor struct {
	conn *dbus.Conn
	ch   chan *dbus.Signal

	mu    sync.Mutex
	state State
}

// Open connects to the system bus, reads UPower's devices and subscribes
// to their changes. It fails where UPower is not running.
func Open() (*Monitor, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("power: system bus: %w", err)
	}
	m := &Monitor{conn: conn, ch: make(chan *dbus.Signal, 32)}
	if m.state, err = m.read(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	for _, opts := range [][]dbus.MatchOption{
		{dbus.WithMatchPathNamespace(upowerPath), dbus.WithMatchInterface(propsIface), dbus.WithMatchMember("PropertiesChanged")},
		{dbus.WithMatchObjectPath(upowerPath), dbus.WithMatchInterface(upowerName)},
	} {
		if err := conn.AddMatchSignal(opts...); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("power: watch UPower: %w", err)
		}
	}
	conn.Signal(m.ch)
	return m, nil
}

// read asks UPower for its OnBattery and every device's power properties.
func (m *Monitor) read() (State, error) {
	daemon := m.conn.Object(upowerName, upowerPath)
	v, err := daemon.GetProperty(upowerName + ".OnBattery")
	if err != nil {
		return State{}, fmt.Errorf("power: UPower OnBattery: %w", err)
	}
	upower, _ := v.Value().(bool)
	var paths []dbus.ObjectPath
	if err := daemon.Call(upowerName+".EnumerateDevices", 0).Store(&paths); err != nil {
		return State{}, fmt.Errorf("power: UPower devices: %w", err)
	}
	devs := make([]Device, 0, len(paths))
	for _, p := range paths {
		var props map[string]dbus.Variant
		if err := m.conn.Object(upowerName, p).Call(propsIface+".GetAll", 0, deviceIface).Store(&props); err != nil {
			continue // removed meanwhile
		}
		devs = append(devs, deviceOf(props))
	}
	return Decide(devs, upower), nil
}

func deviceOf(props map[string]dbus.Variant) Device {
	var d Device
	d.Type, _ = props["Type"].Value().(uint32)
	d.PowerSupply, _ = props["PowerSupply"].Value().(bool)
	d.IsPresent, _ = props["IsPresent"].Value().(bool)
	d.Online, _ = props["Online"].Value().(bool)
	d.State, _ = props["State"].Value().(uint32)
	return d
}

// State returns the power source now.
func (m *Monitor) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Run calls fn with each change of State until Close.
func (m *Monitor) Run(fn func(State)) {
	for sig := range m.ch {
		if !relevant(sig) {
			continue
		}
		// One read for a burst of signals (a device added changes several
		// properties at once).
		for drained := false; !drained; {
			select {
			case _, ok := <-m.ch:
				if !ok {
					return
				}
			default:
				drained = true
			}
		}
		st, err := m.read()
		if err != nil {
			continue
		}
		m.mu.Lock()
		changed := st != m.state
		m.state = st
		m.mu.Unlock()
		if changed {
			fn(st)
		}
	}
}

// Close disconnects; Run returns.
func (m *Monitor) Close() error { return m.conn.Close() }

// decisive are the properties Decide reads; other changes (percentage,
// energy, time to empty) are ignored.
var decisive = map[string]bool{"OnBattery": true, "Type": true, "PowerSupply": true, "IsPresent": true, "Online": true, "State": true}

// relevant reports whether sig can change the State: a device added or
// removed, or a change of a property Decide reads on UPower or one of its
// devices (the composite DisplayDevice excluded).
func relevant(sig *dbus.Signal) bool {
	if sig == nil {
		return false
	}
	switch sig.Name {
	case upowerName + ".DeviceAdded", upowerName + ".DeviceRemoved":
		return sig.Path == upowerPath
	case propsSignal:
	default:
		return false
	}
	path := string(sig.Path)
	if path != upowerPath && (!strings.HasPrefix(path, devicePrefix) || path == devicePrefix+"DisplayDevice") {
		return false
	}
	if len(sig.Body) < 2 {
		return false
	}
	if iface, _ := sig.Body[0].(string); iface != upowerName && iface != deviceIface {
		return false
	}
	changed, _ := sig.Body[1].(map[string]dbus.Variant)
	for k := range changed {
		if decisive[k] {
			return true
		}
	}
	// Invalidated properties (Body[2]) carry no values: read again.
	if len(sig.Body) > 2 {
		if inv, _ := sig.Body[2].([]string); len(inv) > 0 {
			return true
		}
	}
	return false
}
