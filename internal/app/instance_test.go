package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/coredev-uk/sona-go/internal/app/dbustest"
)

func TestSingleInstanceForwarding(t *testing.T) {
	addr := dbustest.Start(t)
	ctx := context.Background()

	primary, err := Acquire(ctx, dbustest.Conn(t, addr), Activation{Args: []string{"--first"}}, nil)
	if err != nil {
		t.Fatalf("primary Acquire: %v", err)
	}
	t.Cleanup(func() { _ = primary.Close() })

	second := Activation{
		Args:            []string{"music://music.apple.com/us/album/abbey-road/1441164426?i=1441164430"},
		ActivationToken: "tok-123",
		WorkingDir:      "/tmp",
	}
	secondConn := dbustest.Conn(t, addr)
	if inst, err := Acquire(ctx, secondConn, second, nil); !errors.Is(err, ErrForwarded) || inst != nil {
		t.Fatalf("second Acquire = %v, %v; want ErrForwarded", inst, err)
	}
	select {
	case got := <-primary.Activations():
		if !reflect.DeepEqual(got, second) {
			t.Errorf("activation = %+v, want %+v", got, second)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("primary received no activation")
	}

	// No args and no platform data still round-trip.
	if _, err := Acquire(ctx, secondConn, Activation{}, nil); !errors.Is(err, ErrForwarded) {
		t.Fatalf("empty forward: %v", err)
	}
	if got := <-primary.Activations(); len(got.Args) != 0 || got.ActivationToken != "" || got.WorkingDir != "" {
		t.Errorf("empty activation = %+v", got)
	}

	// After the primary closes, the next launch becomes primary.
	if err := primary.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-primary.Activations(); ok {
		t.Error("Activations not closed")
	}
	next, err := Acquire(ctx, dbustest.Conn(t, addr), Activation{}, nil)
	if err != nil {
		t.Fatalf("Acquire after Close: %v", err)
	}
	_ = next.Close()
}

func TestForwardToUnresponsiveOwner(t *testing.T) {
	addr := dbustest.Start(t)
	// Owns the name but exports nothing, like a wedged or foreign process.
	squatter := dbustest.Conn(t, addr)
	if _, err := squatter.RequestName(BusName, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	inst, err := Acquire(ctx, dbustest.Conn(t, addr), Activation{Args: []string{"x"}}, nil)
	if err == nil || errors.Is(err, ErrForwarded) || inst != nil {
		t.Fatalf("Acquire = %v, %v; want a forwarding error", inst, err)
	}
}
