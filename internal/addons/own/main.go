package own

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/coredev-uk/sona-go/internal/addons"
)

// Main is `sona --addon <id>`: it runs the bundled add-on id over in and
// out until Sona stops it, and returns the process's exit status. Nothing
// else of Sona starts in this process: no window, no log file, no settings,
// no keyring. What it knows of Sona is what the supervisor sends over the
// channel.
func Main(id string, in io.Reader, out io.Writer, stderr io.Writer) int {
	if !addons.IsBundled(id) {
		_, _ = fmt.Fprintf(stderr, "sona: --addon: %q is not one of Sona's own add-ons\n", id)
		return 2
	}
	m, err := addons.BundledManifest(id)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sona: --addon %s: %v\n", id, err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	c := NewClient(id, in, out, log)
	if err := c.Serve(ctx, m.Permissions, setupFor(id)); err != nil {
		log.Error("stopped", "err", err)
		return 1
	}
	return 0
}

// setupFor is what add-on id does once Sona has answered its hello. The
// scrobblers and Discord add what they do as they move onto the API.
func setupFor(string) Setup { return nil }
