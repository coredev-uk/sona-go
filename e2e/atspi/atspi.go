// Package atspi reads the accessibility tree an app publishes on the AT-SPI
// bus, as a screen reader does: names, roles and children of every
// accessible, with no toolkit library (D-Bus only). The e2e smoke uses it to
// check that Sona's landmarks are visible to assistive technology.
package atspi

import (
	"fmt"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	registryName = "org.a11y.atspi.Registry"
	registryPath = "/org/a11y/atspi/accessible/root"
	accessible   = "org.a11y.atspi.Accessible"
)

// Node is an accessible and what is below it.
type Node struct {
	Name     string
	Role     string // AT-SPI's role name: "frame", "push button", "heading", …
	Children []*Node
}

// Walk calls fn for n and every node below it, parents first.
func (n *Node) Walk(fn func(n *Node)) {
	fn(n)
	for _, c := range n.Children {
		c.Walk(fn)
	}
}

// Find is the first node below (or at) n with this role and name.
func (n *Node) Find(role, name string) *Node {
	var found *Node
	n.Walk(func(m *Node) {
		if found == nil && m.Role == role && m.Name == name {
			found = m
		}
	})
	return found
}

// Dump is the tree as indented "role name" lines.
func (n *Node) Dump() string {
	var b strings.Builder
	var rec func(n *Node, depth int)
	rec = func(n *Node, depth int) {
		fmt.Fprintf(&b, "%s%s %q\n", strings.Repeat("  ", depth), n.Role, n.Name)
		for _, c := range n.Children {
			rec(c, depth+1)
		}
	}
	rec(n, 0)
	return b.String()
}

// Bus is a connection to the AT-SPI bus of the session.
type Bus struct{ conn *dbus.Conn }

// Connect finds the AT-SPI bus through the session bus (org.a11y.Bus) and
// connects to it.
func Connect() (*Bus, error) {
	session, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("atspi: session bus: %w", err)
	}
	var addr string
	if err := session.Object("org.a11y.Bus", "/org/a11y/bus").Call("org.a11y.Bus.GetAddress", 0).Store(&addr); err != nil {
		return nil, fmt.Errorf("atspi: the accessibility bus: %w", err)
	}
	conn, err := dbus.Dial(addr)
	if err != nil {
		return nil, fmt.Errorf("atspi: dial %s: %w", addr, err)
	}
	if err := conn.Auth(nil); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.Hello(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &Bus{conn: conn}, nil
}

// Close ends the connection.
func (b *Bus) Close() error { return b.conn.Close() }

type ref struct {
	Bus  string
	Path dbus.ObjectPath
}

func (b *Bus) read(r ref, depth int) (*Node, error) {
	obj := b.conn.Object(r.Bus, r.Path)
	n := &Node{}
	if v, err := obj.GetProperty(accessible + ".Name"); err == nil {
		n.Name, _ = v.Value().(string)
	}
	if err := obj.Call(accessible+".GetRoleName", 0).Store(&n.Role); err != nil {
		return nil, err
	}
	if depth == 0 {
		return n, nil
	}
	var kids []ref
	if err := obj.Call(accessible+".GetChildren", 0).Store(&kids); err != nil {
		return n, nil
	}
	for _, k := range kids {
		if c, err := b.read(k, depth-1); err == nil {
			n.Children = append(n.Children, c)
		}
	}
	return n, nil
}

// Desktop is the registry's root and the applications below it, to depth
// levels (an application, its windows, their elements…).
func (b *Bus) Desktop(depth int) (*Node, error) {
	return b.read(ref{registryName, registryPath}, depth)
}

// App is the application accessible whose name is name, read to depth
// levels below it; it waits up to wait for the application to register.
func (b *Bus) App(name string, depth int, wait time.Duration) (*Node, error) {
	var apps []string
	for deadline := time.Now().Add(wait); ; time.Sleep(200 * time.Millisecond) {
		desk, err := b.Desktop(1)
		if err == nil {
			apps = apps[:0]
			for _, a := range desk.Children {
				apps = append(apps, a.Name)
			}
		}
		if err == nil {
			// Re-read the application with its subtree.
			var kids []ref
			if e := b.conn.Object(registryName, registryPath).Call(accessible+".GetChildren", 0).Store(&kids); e == nil {
				for i, k := range kids {
					if i < len(apps) && apps[i] == name {
						return b.read(k, depth)
					}
				}
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("atspi: no application %q on the accessibility bus (found %q)", name, apps)
		}
	}
}
