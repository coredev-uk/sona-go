package main

import (
	"log/slog"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/cli"
)

// servePrimary hands later launches' links to activate and runs the commands
// sent by `sona <command>`, while the instance guard in is held.
func servePrimary(in app.Primary, activate func(app.Activation, bool), commands cli.Server) {
	in.HandleCommands(commands)
	go func() {
		for a := range in.Activations() {
			activate(a, true)
		}
	}()
}

// closePrimary releases the instance guard in.
func closePrimary(in app.Primary, log *slog.Logger) {
	if err := in.Close(); err != nil {
		log.Warn("single instance: close", "err", err)
	}
}
