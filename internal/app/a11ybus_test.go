package app

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/coredev-uk/sona-go/internal/app/dbustest"
)

// a11yBus serves org.a11y.Bus.GetAddress; with hang set it never answers.
type a11yBus struct {
	addr string
	hang chan struct{}
}

func (b a11yBus) GetAddress() (string, *dbus.Error) {
	if b.hang != nil {
		<-b.hang
	}
	return b.addr, nil
}

// serveA11yBus owns org.a11y.Bus on the bus at addr.
func serveA11yBus(t *testing.T, addr string, b a11yBus) {
	t.Helper()
	conn := dbustest.Conn(t, addr)
	if err := conn.Export(b, "/org/a11y/bus", "org.a11y.Bus"); err != nil {
		t.Fatal(err)
	}
	if reply, err := conn.RequestName("org.a11y.Bus", dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("own org.a11y.Bus: %v %v", reply, err)
	}
}

// unsetenv unsets k for the test and restores it afterwards.
func unsetenv(t *testing.T, k string) {
	t.Helper()
	t.Setenv(k, "")
	if err := os.Unsetenv(k); err != nil {
		t.Fatal(err)
	}
}

func TestHideMissingA11yBus(t *testing.T) {
	cases := []struct {
		name   string
		bus    *a11yBus // nil: nobody owns org.a11y.Bus
		noConn bool     // no session bus at all
		env    map[string]string
		hide   bool
	}{
		{name: "no a11y bus", hide: true},
		{name: "no session bus", noConn: true, hide: true},
		{name: "a11y bus", bus: &a11yBus{addr: "unix:path=/run/user/1000/at-spi/bus"}},
		{name: "a11y bus without an address", bus: &a11yBus{}, hide: true},
		{name: "AT_SPI_BUS_ADDRESS set", env: map[string]string{envATSPIBus: "unix:path=/tmp/a11y"}},
		{name: "user's WEBKIT_A11Y_BUS_ADDRESS", env: map[string]string{envWebKitA11yBus: "unix:path=/tmp/proxy"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			unsetenv(t, envWebKitA11yBus)
			unsetenv(t, envATSPIBus)
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			var conn *dbus.Conn
			if !c.noConn {
				addr := dbustest.Start(t)
				if c.bus != nil {
					serveA11yBus(t, addr, *c.bus)
				}
				conn = dbustest.Conn(t, addr)
			}
			hid, err := HideMissingA11yBus(context.Background(), conn)
			if err != nil {
				t.Fatal(err)
			}
			got, set := os.LookupEnv(envWebKitA11yBus)
			want, wantSet := c.env[envWebKitA11yBus]
			if c.hide {
				want, wantSet = "", true
			}
			if hid != c.hide || got != want || set != wantSet {
				t.Fatalf("hid %v, %s = %q (set %v); want hid %v, %q (set %v)", hid, envWebKitA11yBus, got, set, c.hide, want, wantSet)
			}
		})
	}
}

// A bus that does not answer in time is not proof there is none: WebKit
// is left to look it up itself.
func TestHideMissingA11yBusTimeout(t *testing.T) {
	unsetenv(t, envWebKitA11yBus)
	unsetenv(t, envATSPIBus)
	addr := dbustest.Start(t)
	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	serveA11yBus(t, addr, a11yBus{addr: "unix:path=/tmp/a11y", hang: hang})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	hid, err := HideMissingA11yBus(ctx, dbustest.Conn(t, addr))
	if err == nil || hid {
		t.Fatalf("hid %v, err %v; want an error", hid, err)
	}
	if v, set := os.LookupEnv(envWebKitA11yBus); set {
		t.Fatalf("%s = %q after a timeout", envWebKitA11yBus, v)
	}
}
