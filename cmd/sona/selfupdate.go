package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"time"

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

// rollBackFailedUpdate runs first thing at start. A version that an update
// installed gets update.MaxStarts starts to reach its first paint (which
// clears update-pending, services.UpdatesService); after that this start
// restores the version the update replaced and starts it. It reports
// whether it did, when the caller exits, and whether this is one of those
// starts of an update on trial (onTrial).
func rollBackFailedUpdate(paths config.Paths, current string, log *slog.Logger) (restored, onTrial bool) {
	exe, err := os.Executable()
	if err != nil {
		return false, false
	}
	g, ok := update.NewGuard(paths.State, exe, runtime.GOOS)
	if !ok {
		return false, false
	}
	act, err := g.Begin(current)
	if err != nil {
		log.Warn("update guard", "err", err)
	}
	if act != update.Restore {
		return false, act == update.Counted
	}
	log.Error("the updated version never reached its first paint: restoring the previous version", "version", current)
	if err := g.Rollback(); err != nil {
		log.Error("update rollback failed", "err", err)
		return false, false
	}
	if err := update.Relaunch(runtime.GOOS, g.Root, exe); err != nil {
		log.Error("restarting the restored version", "err", err)
	}
	return true, false
}

// replacedQuitWait is how long the first starts of an update wait for the
// version it replaced to let go of the single instance, and replacedQuitPoll
// how often they look.
const (
	replacedQuitWait = 10 * time.Second
	replacedQuitPoll = 250 * time.Millisecond
)

// openPlatformAfterUpdate is openPlatform for a start of an update on trial.
// The version the update replaced starts this one as it quits, after its
// quit handlers (MyGo's Relaunch), and Sona before 0.6.1 let go of the
// single instance only after that, once run returned: the first launch of
// the new version went to the instance on its way out and exited, and no
// Sona was left. So a start of an update that finds an instance running
// waits, up to replacedQuitWait, for it to quit and takes over.
func openPlatformAfterUpdate(paths config.Paths, act app.Activation, log *slog.Logger, open func(config.Paths, app.Activation, *slog.Logger) (*platform, error)) (*platform, error) {
	plat, err := open(paths, act, log)
	if !errors.Is(err, app.ErrForwarded) {
		return plat, err
	}
	log.Info("update: the version it replaced is still running; waiting for it to quit")
	for deadline := time.Now().Add(replacedQuitWait); time.Now().Before(deadline); {
		time.Sleep(replacedQuitPoll)
		if plat, err = open(paths, act, log); !errors.Is(err, app.ErrForwarded) {
			return plat, err
		}
	}
	return nil, err
}

// selfUpdater is the updater where Sona updates itself, else a nil
// interface (a nil *mygoUpdater would not be a nil services.SelfUpdater).
func selfUpdater() services.SelfUpdater {
	if update.Target(runtime.GOOS, runtime.GOARCH) == "" {
		return nil
	}
	return &mygoUpdater{}
}
