package own

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons"
)

// Main is `sona --addon <id>`: it runs the bundled add-on id over in and
// out until Sona stops it, and returns the process's exit status. version is
// Sona's. Nothing else of Sona starts in this process: no window, no log
// file, no settings, no keyring. What it knows of Sona is what the supervisor
// sends over the channel.
func Main(id, version string, in io.Reader, out io.Writer, stderr io.Writer) int {
	if !addons.IsBundled(id) {
		_, _ = fmt.Fprintf(stderr, "sona: --addon: %q is not one of Sona's own add-ons\n", id)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	opts := Options{Version: version, Endpoints: devEndpoints(log)}
	if err := Run(ctx, id, in, out, log, opts); err != nil {
		log.Error("stopped", "err", err)
		return 1
	}
	return 0
}

// Run serves the bundled add-on id over in and out until Sona stops it or
// ctx ends; a clean stop returns nil.
func Run(ctx context.Context, id string, in io.Reader, out io.Writer, log *slog.Logger, opts Options) error {
	m, err := addons.BundledManifest(id)
	if err != nil {
		return err
	}
	c := NewClient(id, in, out, log)
	register(c, id, opts)
	return c.Serve(ctx, m.Permissions, setupFor(id))
}

// register sets the answers to the requests of Sona that add-on id gets
// (cli.HostMethods): the handlers exist before hello, and the client holds
// each request until hello is answered and Setup has run.
func register(c *Client, id string, opts Options) {
	switch id {
	case "listenbrainz":
		serveScrobbler(c, newListenBrainz(opts))
	case "lastfm":
		serveScrobbler(c, newLastFM(opts))
	}
}

// setupFor is what add-on id does once Sona has answered its hello: Discord
// subscribes to the topics; the scrobblers only answer Sona's requests
// (register).
func setupFor(id string) Setup {
	if id == "discord" {
		return discordSetup(discordSockets(), time.Now)
	}
	return nil
}
