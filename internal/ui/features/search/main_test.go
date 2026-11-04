package search

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// TestMain registers the interface font once, so the golden images are
// drawn in Schibsted Grotesk whatever test ran first.
func TestMain(m *testing.M) {
	if err := theme.RegisterFonts(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// queueStub is the rest of the playback service the queue commands ask of;
// the tests' recorders embed it and record PlayItems.
type queueStub struct{}

func (queueStub) PlayNext(context.Context, []services.TrackRef) error    { return nil }
func (queueStub) Append(context.Context, []services.TrackRef) error      { return nil }
func (queueStub) Insert(context.Context, int, []services.TrackRef) error { return nil }
func (queueStub) UndoReplace(context.Context) (bool, error)              { return false, nil }
func (queueStub) SetShuffle(bool)                                        {}

// useQueue makes the recorder the playback service behind Env.Queue.
func useQueue(env *module.Env, svc kit.QueueService) {
	env.Queue = kit.NewQueueCommands(kit.QueueDeps{Svc: svc, Play: env.Play, Post: env.Post})
}
