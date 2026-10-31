package playback

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUnsupportedRefusesEveryCommand(t *testing.T) {
	u := NewUnsupported()
	t.Cleanup(func() { _ = u.Close() })
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"Play":      func() error { return u.Play(ctx, "1440", 0) },
		"Preload":   func() error { return u.Preload(ctx, "1440") },
		"Pause":     func() error { return u.Pause(ctx) },
		"Resume":    func() error { return u.Resume(ctx) },
		"Seek":      func() error { return u.Seek(ctx, time.Second) },
		"SetVolume": func() error { return u.SetVolume(ctx, 0.5) },
		"Stop":      func() error { return u.Stop(ctx) },
	} {
		if err := call(); !errors.Is(err, ErrUnsupported) || !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("%s: %v, want ErrUnsupported wrapping errors.ErrUnsupported", name, err)
		}
	}
	u.SetNext(Next{ID: "1441"})
	st := u.Status()
	if st.State != StateIdle || st.ItemID != "" || st.Volume != 1 || st.UpdatedAt.IsZero() {
		t.Errorf("Status after commands: %+v", st)
	}
	if c := u.Capabilities(); c != (Capabilities{}) {
		t.Errorf("Capabilities = %+v, want none", c)
	}
}

func TestUnsupportedEventsCloseOnClose(t *testing.T) {
	u := NewUnsupported()
	select {
	case ev := <-u.Events():
		t.Fatalf("event before Close: %+v", ev)
	default:
	}
	if err := u.Close(); err != nil {
		t.Fatal(err)
	}
	if err := u.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, ok := <-u.Events(); ok {
		t.Fatal("Events delivered after Close")
	}
}
