package power

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestDecide(t *testing.T) {
	adapter := func(online bool) Device { return Device{Type: TypeLinePower, PowerSupply: true, Online: online} }
	battery := func(state uint32) Device {
		return Device{Type: TypeBattery, PowerSupply: true, IsPresent: true, State: state}
	}
	const (
		charging    = 1
		discharging = StateDischarging
		full        = 4
		unknown     = 0
	)
	mouse := Device{Type: 5, IsPresent: true, State: discharging}
	headset := Device{Type: 17, IsPresent: true, State: discharging}
	// A peripheral that reports itself as a battery, but not as powering
	// the computer.
	peripheralBattery := Device{Type: TypeBattery, PowerSupply: false, IsPresent: true, State: discharging}
	ups := func(state uint32) Device {
		return Device{Type: TypeUPS, PowerSupply: true, IsPresent: true, State: state}
	}

	for _, tc := range []struct {
		name    string
		devs    []Device
		upower  bool
		want    bool
		source  string
		ignored int
	}{
		{"desktop, no devices", nil, false, false, SourceMains, 0},
		{"desktop with a wireless mouse and headset", []Device{mouse, headset}, false, false, SourceMains, 2},
		{"desktop with a peripheral battery UPower counts", []Device{peripheralBattery}, true, false, SourceMains, 1},
		{"desktop on a UPS with an unknown state (UPower says battery)", []Device{ups(unknown)}, true, false, SourceMains, 0},
		{"desktop on a UPS that is charging", []Device{ups(charging)}, false, false, SourceMains, 0},
		{"desktop on a UPS that is discharging (power cut)", []Device{ups(discharging)}, true, true, SourceUPS, 0},
		{"laptop on AC, charging", []Device{adapter(true), battery(charging)}, false, false, SourceAC, 0},
		{"laptop on AC, battery reports discharging under load", []Device{adapter(true), battery(discharging)}, true, false, SourceAC, 0},
		{"laptop on AC, full", []Device{adapter(true), battery(full), mouse}, false, false, SourceAC, 1},
		{"laptop unplugged", []Device{adapter(false), battery(discharging)}, true, true, SourceBattery, 0},
		{"laptop unplugged, battery state unknown", []Device{adapter(false), battery(unknown)}, true, true, SourceBattery, 0},
		{"laptop with two adapters, one online (USB-C)", []Device{adapter(false), adapter(true), battery(charging)}, false, false, SourceAC, 0},
		{"laptop without an adapter device, discharging", []Device{battery(discharging)}, true, true, SourceBattery, 0},
		{"laptop without an adapter device, charging", []Device{battery(charging)}, false, false, SourceMains, 0},
		{"battery slot empty", []Device{adapter(false), {Type: TypeBattery, PowerSupply: true}}, false, false, SourceMains, 0},
		{"laptop on a UPS, adapter online", []Device{adapter(true), battery(full), ups(discharging)}, true, false, SourceAC, 0},
	} {
		got := Decide(tc.devs, tc.upower)
		if got.OnBattery != tc.want || got.Source != tc.source || got.Ignored != tc.ignored || got.UPower != tc.upower {
			t.Errorf("%s: got %+v; want on battery %v, source %q, %d ignored", tc.name, got, tc.want, tc.source, tc.ignored)
		}
	}
}

func TestRelevant(t *testing.T) {
	sig := func(path dbus.ObjectPath, iface string, changed map[string]dbus.Variant) *dbus.Signal {
		return &dbus.Signal{Path: path, Name: propsSignal, Body: []any{iface, changed, []string{}}}
	}
	bat := dbus.ObjectPath(devicePrefix + "battery_BAT0")
	for _, tc := range []struct {
		name string
		sig  *dbus.Signal
		want bool
	}{
		{"daemon OnBattery", sig(upowerPath, upowerName, map[string]dbus.Variant{"OnBattery": dbus.MakeVariant(true)}), true},
		{"daemon lid", sig(upowerPath, upowerName, map[string]dbus.Variant{"LidIsClosed": dbus.MakeVariant(true)}), false},
		{"battery state", sig(bat, deviceIface, map[string]dbus.Variant{"State": dbus.MakeVariant(uint32(2)), "Percentage": dbus.MakeVariant(80.0)}), true},
		{"adapter unplugged", sig(devicePrefix+"line_power_ACAD", deviceIface, map[string]dbus.Variant{"Online": dbus.MakeVariant(false)}), true},
		{"battery percentage only", sig(bat, deviceIface, map[string]dbus.Variant{"Percentage": dbus.MakeVariant(79.0), "Energy": dbus.MakeVariant(40.1)}), false},
		{"display device", sig(devicePrefix+"DisplayDevice", deviceIface, map[string]dbus.Variant{"State": dbus.MakeVariant(uint32(2))}), false},
		{"invalidated", &dbus.Signal{Path: bat, Name: propsSignal, Body: []any{deviceIface, map[string]dbus.Variant{}, []string{"State"}}}, true},
		{"other object", sig("/org/freedesktop/login1", upowerName, map[string]dbus.Variant{"OnBattery": dbus.MakeVariant(true)}), false},
		{"other interface", sig(bat, "org.freedesktop.UPower.KbdBacklight", map[string]dbus.Variant{"State": dbus.MakeVariant(uint32(2))}), false},
		{"device added", &dbus.Signal{Path: upowerPath, Name: upowerName + ".DeviceAdded", Body: []any{bat}}, true},
		{"device removed", &dbus.Signal{Path: upowerPath, Name: upowerName + ".DeviceRemoved", Body: []any{bat}}, true},
		{"short body", &dbus.Signal{Path: upowerPath, Name: propsSignal, Body: []any{upowerName}}, false},
		{"nil", nil, false},
	} {
		if got := relevant(tc.sig); got != tc.want {
			t.Errorf("%s: relevant = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDeviceOf(t *testing.T) {
	d := deviceOf(map[string]dbus.Variant{
		"Type": dbus.MakeVariant(uint32(TypeBattery)), "PowerSupply": dbus.MakeVariant(true),
		"IsPresent": dbus.MakeVariant(true), "State": dbus.MakeVariant(uint32(StateDischarging)),
		"Percentage": dbus.MakeVariant(50.0),
	})
	if d != (Device{Type: TypeBattery, PowerSupply: true, IsPresent: true, State: StateDischarging}) {
		t.Errorf("deviceOf = %+v", d)
	}
}
