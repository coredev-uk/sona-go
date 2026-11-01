// Command notifyprobe shows Sona's track notification through Sona's own
// notification client (internal/desktop/notify, with the options
// platform_linux.go gives it), for the desktop VM tests (tests/vm): the
// app announces a track only when it starts playing, and the VMs have no
// playback (no network, no CDM).
//
//	notifyprobe [-image FILE] [-gap 2s] [-hold 2m] TITLE...
//
// Each TITLE is shown in turn, replacing the one before, as a track change
// does. It prints "caps …", "shown TITLE" and "action KEY" for each action
// the server reports, and exits after -hold (the server keeps showing the
// notification only while its sender is on the bus).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/coredev-uk/sona-go/internal/desktop/notify"
	"github.com/coredev-uk/sona-go/internal/mpris"
)

func main() {
	image := flag.String("image", "", "artwork file shown as the notification's picture")
	gap := flag.Duration("gap", 2*time.Second, "time between titles")
	hold := flag.Duration("hold", 2*time.Minute, "how long to stay on the bus after the last title")
	flag.Parse()
	if err := run(flag.Args(), *image, *gap, *hold); err != nil {
		fmt.Fprintln(os.Stderr, "notifyprobe:", err)
		os.Exit(1)
	}
}

func run(titles []string, image string, gap, hold time.Duration) error {
	if len(titles) == 0 {
		return fmt.Errorf("no titles")
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	actions := make(chan string, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	n, err := notify.New(ctx, conn, notify.Options{
		AppName:      "Sona",
		AppIcon:      "sona",
		DesktopEntry: mpris.DesktopEntry,
		OnAction:     func(key string) { actions <- key },
	})
	cancel()
	if err != nil {
		return err
	}
	defer n.Close()

	var caps []string
	for _, c := range []string{"actions", "body", "body-markup", "body-images", "persistence"} {
		if n.Has(c) {
			caps = append(caps, c)
		}
	}
	fmt.Println("caps", strings.Join(caps, ","))

	for i, title := range titles {
		if i > 0 {
			time.Sleep(gap)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := n.Show(ctx, notify.Notification{
			Summary: title,
			Body:    "Sona VM Artist — Offline Album",
			Image:   image,
			Actions: []notify.Action{
				{Key: notify.DefaultAction, Label: "Show Sona"},
				{Key: "next", Label: "Next"},
			},
		})
		cancel()
		if err != nil {
			return err
		}
		fmt.Println("shown", title)
	}

	deadline := time.After(hold)
	for {
		select {
		case key := <-actions:
			fmt.Println("action", key)
		case <-deadline:
			return nil
		}
	}
}
