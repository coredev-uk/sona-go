// Command profile writes the fixture profile (the synthetic library of
// internal/ui/uitest/fixture, a signed-out user with the first-run setup
// done) into the directories the XDG variables name, as Sona resolves them:
//
//	XDG_CONFIG_HOME=… XDG_DATA_HOME=… XDG_STATE_HOME=… XDG_CACHE_HOME=… go run ./e2e/profile
//
// Run it inside `headless` (or with private XDG variables): it never
// touches anything but those directories, and refuses to run when they are
// not all set, so it cannot write into a real profile by default.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

func main() {
	firstRun := flag.Bool("first-run", false, "leave the first-run setup undone, so the app opens on it")
	flag.Parse()
	for _, v := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		if os.Getenv(v) == "" {
			fmt.Fprintf(os.Stderr, "profile: %s is not set; refusing to write into the default profile\n", v)
			os.Exit(2)
		}
	}
	paths, err := config.ResolvePaths(os.Getenv)
	if err == nil {
		err = fixture.WriteProfile(context.Background(), paths)
	}
	if err == nil && *firstRun {
		err = undoSetup(paths)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "profile:", err)
		os.Exit(1)
	}
	fmt.Println("fixture profile written:", paths.Config, paths.Data)
}

// undoSetup marks the first-run setup not done in the written profile.
func undoSetup(paths config.Paths) error {
	s, err := config.OpenStore(paths.SettingsFile())
	if err != nil {
		return err
	}
	_, err = s.Update(func(s *config.Settings) { s.Onboarding.Completed = false })
	return err
}
