package app

import (
	"context"
	"errors"
	"os"

	"github.com/godbus/dbus/v5"
)

// Environment variables WebKitGTK reads for its web processes'
// accessibility (AT-SPI) bus.
const (
	envWebKitA11yBus = "WEBKIT_A11Y_BUS_ADDRESS"
	envATSPIBus      = "AT_SPI_BUS_ADDRESS"
)

// HideMissingA11yBus keeps WebKitGTK from warning "Can't connect to a11y
// bus" on every start of a session that has no accessibility bus.
//
// With its bubblewrap sandbox, WebKitGTK (2.54) always hands the web
// process the address of an a11y bus proxy inside the sandbox, but only
// starts that proxy when the session has an a11y bus. Without one the web
// process (PID 2 in its sandbox) fails to connect and prints the warning.
// WebKit looks the bus up once per process, so its web content has no
// a11y bus for the app's lifetime either way; an empty
// WEBKIT_A11Y_BUS_ADDRESS says so without the failed connection.
//
// HideMissingA11yBus asks the session bus for org.a11y.Bus's address as
// WebKit does (starting the bus if it is activatable) and sets the
// variable to "" only when there definitively is none: no session bus
// (conn nil), an error reply, or an empty address. It changes nothing
// when the user set WEBKIT_A11Y_BUS_ADDRESS or AT_SPI_BUS_ADDRESS, or when
// the call fails otherwise (a timeout: WebKit then asks again itself). It
// reports whether it set the variable. Call it before the first web view
// exists, while no other thread reads the environment (unsetting it later
// would race WebKit's getenv on older glibc), so programs Sona starts
// inherit it like the WEBKIT_* diagnostics variables.
func HideMissingA11yBus(ctx context.Context, conn *dbus.Conn) (bool, error) {
	if _, ok := os.LookupEnv(envWebKitA11yBus); ok || os.Getenv(envATSPIBus) != "" {
		return false, nil
	}
	if conn != nil {
		var addr string
		err := conn.Object("org.a11y.Bus", "/org/a11y/bus").CallWithContext(ctx, "org.a11y.Bus.GetAddress", 0).Store(&addr)
		if err != nil && !errors.As(err, new(dbus.Error)) {
			return false, err
		}
		if err == nil && addr != "" {
			return false, nil
		}
	}
	if err := os.Setenv(envWebKitA11yBus, ""); err != nil {
		return false, err
	}
	return true, nil
}
