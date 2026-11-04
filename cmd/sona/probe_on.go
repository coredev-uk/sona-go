//go:build probe

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/ui/probe"
)

// `sona -probe` and `sona -startup` (probe builds only; internal/ui/probe).
// Everything is wired from init so main stays what a release build runs:
// the flags are taken out of the arguments first, the app restarts itself
// with the environment MyGo reads while its packages initialise (frame
// statistics, the automation socket), and the measurement starts when MyGo
// is ready, after main installed Sona's logger.
func init() {
	started := time.Now()
	o, rest, err := probe.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "sona:", err)
		os.Exit(2)
	}
	if !o.On() {
		return
	}
	status, restarted, err := probe.Reexec(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "sona -probe:", err)
		os.Exit(1)
	}
	if restarted {
		os.Exit(status)
	}
	os.Args = append(os.Args[:1:1], rest...)
	mygo.App.WhenReady(func() { probe.Begin(o, started) })
}
