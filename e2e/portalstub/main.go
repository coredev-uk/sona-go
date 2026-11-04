// Command portalstub owns org.freedesktop.portal.Desktop on the session bus
// and answers the Settings portal at once. In the private session of
// e2e/headless.sh there is no xdg-desktop-portal; without a name owner the
// bus tries to activate one and the toolkit waits for the attempt to fail
// (about 12 s before the first frame). The stub answers like a desktop
// with default settings: the colour scheme "no preference". It implements
// nothing else, so every other portal reports itself absent.
package main

import (
	"fmt"
	"os"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	name  = "org.freedesktop.portal.Desktop"
	path  = "/org/freedesktop/portal/desktop"
	iface = "org.freedesktop.portal.Settings"
)

// defaults are the values the stub reports, by namespace and key.
var defaults = map[string]map[string]dbus.Variant{
	"org.freedesktop.appearance": {
		"color-scheme": dbus.MakeVariant(uint32(0)), // no preference
		"contrast":     dbus.MakeVariant(uint32(0)),
	},
}

type settings struct{}

// ReadAll returns the settings of the namespaces asked for (patterns with a
// trailing * match a prefix; none matches all).
func (settings) ReadAll(namespaces []string) (map[string]map[string]dbus.Variant, *dbus.Error) {
	out := map[string]map[string]dbus.Variant{}
	for ns, kv := range defaults {
		if matches(namespaces, ns) {
			out[ns] = kv
		}
	}
	return out, nil
}

// Read returns one setting, as a variant holding the value.
func (settings) Read(ns, key string) (dbus.Variant, *dbus.Error) {
	if v, ok := defaults[ns][key]; ok {
		return dbus.MakeVariant(v), nil
	}
	return dbus.Variant{}, dbus.NewError("org.freedesktop.portal.Error.NotFound", []any{"no such setting"})
}

func matches(patterns []string, ns string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if p == ns || (len(p) > 0 && p[len(p)-1] == '*' && len(ns) >= len(p)-1 && ns[:len(p)-1] == p[:len(p)-1]) {
			return true
		}
	}
	return false
}

func main() {
	conn, err := dbus.SessionBus()
	if err != nil {
		fmt.Fprintln(os.Stderr, "portalstub:", err)
		os.Exit(1)
	}
	if err := conn.Export(settings{}, path, iface); err != nil {
		fmt.Fprintln(os.Stderr, "portalstub:", err)
		os.Exit(1)
	}
	props, err := prop.Export(conn, path, prop.Map{
		iface: {"version": {Value: uint32(2), Writable: false, Emit: prop.EmitFalse}},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "portalstub:", err)
		os.Exit(1)
	}
	_ = props
	node := &introspect.Node{
		Name: path,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{Name: iface, Methods: introspect.Methods(settings{}), Properties: props.Introspection(iface)},
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(node), path, "org.freedesktop.DBus.Introspectable"); err != nil {
		fmt.Fprintln(os.Stderr, "portalstub:", err)
		os.Exit(1)
	}
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		fmt.Fprintln(os.Stderr, "portalstub: cannot own", name, reply, err)
		os.Exit(1)
	}
	select {}
}
