// Package power follows whether the machine runs on battery, from UPower's
// OnBattery property on the system bus.
package power

import (
	"fmt"
	"sync/atomic"

	"github.com/godbus/dbus/v5"
)

const (
	upowerName  = "org.freedesktop.UPower"
	upowerPath  = "/org/freedesktop/UPower"
	propsIface  = "org.freedesktop.DBus.Properties"
	propsSignal = propsIface + ".PropertiesChanged"
)

// Monitor follows UPower's OnBattery.
type Monitor struct {
	conn *dbus.Conn
	ch   chan *dbus.Signal
	on   atomic.Bool
}

// Open connects to the system bus, reads OnBattery and subscribes to its
// changes. It fails where UPower is not running (then the machine is
// treated as on mains power).
func Open() (*Monitor, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("power: system bus: %w", err)
	}
	m := &Monitor{conn: conn, ch: make(chan *dbus.Signal, 8)}
	v, err := conn.Object(upowerName, upowerPath).GetProperty(upowerName + ".OnBattery")
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("power: UPower OnBattery: %w", err)
	}
	on, _ := v.Value().(bool)
	m.on.Store(on)
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(upowerPath),
		dbus.WithMatchInterface(propsIface),
		dbus.WithMatchMember("PropertiesChanged"),
	); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("power: watch UPower: %w", err)
	}
	conn.Signal(m.ch)
	return m, nil
}

// OnBattery reports whether the machine runs on battery now.
func (m *Monitor) OnBattery() bool { return m.on.Load() }

// Run calls fn with each change of OnBattery until Close.
func (m *Monitor) Run(fn func(onBattery bool)) {
	for sig := range m.ch {
		on, ok := onBatteryChange(sig)
		if !ok || m.on.Swap(on) == on {
			continue
		}
		fn(on)
	}
}

// Close disconnects; Run returns.
func (m *Monitor) Close() error { return m.conn.Close() }

// onBatteryChange reads OnBattery from a UPower PropertiesChanged signal.
func onBatteryChange(sig *dbus.Signal) (bool, bool) {
	if sig == nil || sig.Name != propsSignal || sig.Path != upowerPath || len(sig.Body) < 2 {
		return false, false
	}
	if iface, _ := sig.Body[0].(string); iface != upowerName {
		return false, false
	}
	changed, _ := sig.Body[1].(map[string]dbus.Variant)
	v, ok := changed["OnBattery"]
	if !ok {
		return false, false
	}
	on, ok := v.Value().(bool)
	return on, ok
}
