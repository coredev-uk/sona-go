package power

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestOnBatteryChange(t *testing.T) {
	sig := func(path dbus.ObjectPath, iface string, changed map[string]dbus.Variant) *dbus.Signal {
		return &dbus.Signal{Path: path, Name: propsSignal, Body: []any{iface, changed, []string{}}}
	}
	for _, tc := range []struct {
		name   string
		sig    *dbus.Signal
		on, ok bool
	}{
		{"unplugged", sig(upowerPath, upowerName, map[string]dbus.Variant{"OnBattery": dbus.MakeVariant(true)}), true, true},
		{"plugged in", sig(upowerPath, upowerName, map[string]dbus.Variant{"OnBattery": dbus.MakeVariant(false), "LidIsClosed": dbus.MakeVariant(false)}), false, true},
		// Other properties of UPower, other objects (a battery device) and
		// other interfaces carry no OnBattery change.
		{"other property", sig(upowerPath, upowerName, map[string]dbus.Variant{"LidIsClosed": dbus.MakeVariant(true)}), false, false},
		{"device", sig("/org/freedesktop/UPower/devices/battery_BAT0", upowerName+".Device", map[string]dbus.Variant{"OnBattery": dbus.MakeVariant(true)}), false, false},
		{"wrong type", sig(upowerPath, upowerName, map[string]dbus.Variant{"OnBattery": dbus.MakeVariant("yes")}), false, false},
		{"short body", &dbus.Signal{Path: upowerPath, Name: propsSignal, Body: []any{upowerName}}, false, false},
	} {
		on, ok := onBatteryChange(tc.sig)
		if on != tc.on || ok != tc.ok {
			t.Errorf("%s: got %v, %v; want %v, %v", tc.name, on, ok, tc.on, tc.ok)
		}
	}
}
