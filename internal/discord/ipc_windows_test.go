package discord

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedOnWindows(t *testing.T) {
	c := New("123")
	err := c.SetActivity(context.Background(), nil)
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("SetActivity = %v, want ErrUnsupported", err)
	}
	if c.Connected() {
		t.Fatal("connected without a transport")
	}
}
