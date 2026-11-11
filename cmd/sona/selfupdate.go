package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"runtime"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/update"
)

// mygoUpdater is MyGo's updater behind services.SelfUpdater. Release builds
// for macOS and Windows give MyGo its feed and key at link time (-X
// github.com/egoist/mygo.packageUpdateFeed and .packageUpdateKey, the key
// from internal/update/update.pub); without them Enabled is false and Sona
// reports updates as off.
type mygoUpdater struct{}

func (mygoUpdater) Enabled() bool { return update.PublicKey() != "" && mygo.Updater.Enabled() }

// Install serves the manifest and archive from src, which has verified
// them (internal/update.Source), to MyGo's updater, which checks the
// signature again before swapping the app.
func (mygoUpdater) Install(ctx context.Context, src *update.Source) error {
	mygo.Updater.SetSource(src)
	defer mygo.Updater.SetSource(nil)
	up, err := mygo.Updater.Check(ctx)
	if err != nil {
		return err
	}
	if up == nil {
		return errors.New("no newer version in the manifest")
	}
	return up.Install(ctx, nil)
}

// Relaunch quits and starts the installed version the way the user has
// Sona now (see relaunchArgs).
func (mygoUpdater) Relaunch() { mygo.App.Relaunch(relaunchArgs(os.Args[1:])...) }

// relaunchArgs is what the instance that follows an update or a restore
// starts with, given the arguments this one started with. MyGo's Relaunch
// repeats those unless it is given others, but two kinds are about the start
// and not about Sona:
//
//   - --hidden is how an autostarted instance began, with no window. The
//     update was installed from Settings, so the user has the window open and
//     is watching it: a restart that hid it would look like Sona quit.
//   - an Apple Music link opened the album or playlist it names. The user has
//     moved on since, and a restart that opened it again would take them back.
//
// Anything else is kept. With nothing to give while the instance did start
// with arguments, ShownFlag stands in, as a call with no arguments would
// repeat the old ones.
func relaunchArgs(started []string) []string {
	var keep []string
	for _, a := range started {
		if a == app.HiddenFlag || app.IsMusicURL(a) {
			continue
		}
		keep = append(keep, a)
	}
	if len(keep) == 0 && len(started) > 0 {
		return []string{app.ShownFlag}
	}
	return keep
}

// selfUpdater is the updater where Sona updates itself, else nil.
func selfUpdater() *mygoUpdater {
	if update.Target(runtime.GOOS, runtime.GOARCH) == "" {
		return nil
	}
	return &mygoUpdater{}
}

// rollBackFailedUpdate runs first thing at start. A version that an update
// installed gets update.MaxStarts starts to reach its first paint (which
// clears update-pending, services.UpdatesService); after that this start
// restores the version the update replaced and starts it. It reports
// whether it did, and the caller then exits.
func rollBackFailedUpdate(paths config.Paths, current string, log *slog.Logger) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	g, ok := update.NewGuard(paths.State, exe, runtime.GOOS)
	if !ok {
		return false
	}
	act, err := g.Begin(current)
	if err != nil {
		log.Warn("update guard", "err", err)
	}
	if act != update.Restore {
		return false
	}
	log.Error("the updated version never reached its first paint: restoring the previous version", "version", current)
	if err := g.Rollback(); err != nil {
		log.Error("update rollback failed", "err", err)
		return false
	}
	if err := update.Relaunch(runtime.GOOS, g.Root, exe); err != nil {
		log.Error("restarting the restored version", "err", err)
	}
	return true
}

// selfUpdaterDep is selfUpdater as the interface (a nil *mygoUpdater would
// not be a nil services.SelfUpdater).
func selfUpdaterDep() services.SelfUpdater {
	if u := selfUpdater(); u != nil {
		return u
	}
	return nil
}
