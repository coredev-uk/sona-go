//go:build windows

package sandbox

import (
	"fmt"
	"io"
	"sync"

	"github.com/coredev-uk/sona-go/internal/appcontainer"
)

// maxProcesses is the most processes an add-on's job may hold: a guard against
// a runaway add-on, with room for the helpers a runtime starts.
const maxProcesses = 64

// profiles are the AppContainer profiles opened by this process, by add-on ID.
var profiles sync.Map

// Start runs the add-on in an AppContainer of its own (the profile
// "sona.addon.<id>", so a grant for one add-on is no other's) inside a job
// object that kills it with Sona, or says why it can't. The AppContainer holds
// no capabilities but internetClient, and only with the `network` permission;
// its SID is granted read on the add-on's folder and on what the policy
// lets it read, and read and write on its data folder. Windows then lets it
// reach those and what ALL APPLICATION PACKAGES may read (the Windows folder
// and Program Files), nothing else of the user's, and the network stack
// refuses everything without the capability. It starts detached, with no
// console and no desktop handles.
func Start(spec Spec) (*Child, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}
	if spec.ExeFile != nil {
		return nil, fail(ReasonUnsupported, "Windows can't run the file that was checked")
	}
	if spec.ID == "" {
		return nil, fail(ReasonSetup, "the add-on has no ID to name its AppContainer")
	}
	prof, err := profile(spec.ID)
	if err != nil {
		return nil, fail(ReasonNoAppContainer, "%v", err)
	}
	if err := grant(prof, spec); err != nil {
		return nil, fail(ReasonNoAppContainer, "%v", err)
	}
	var caps []string
	if spec.Network {
		caps = append(caps, appcontainer.CapabilityInternetClient)
	}
	c, err := appcontainer.Start(appcontainer.Options{
		SID: prof.SID, Capabilities: caps,
		Exe: spec.Exe, Args: spec.Args, Env: spec.Env, Dir: spec.Dir,
		MaxProcesses: maxProcesses, ChildProcesses: true,
	})
	if err != nil {
		return nil, fail(ReasonNoAppContainer, "%v", err)
	}
	ch := &Child{In: c.Stdin, Out: c.Stdout, pid: c.Process.Pid, done: make(chan struct{}), kill: func() { _ = c.Kill() }}
	copied := make(chan struct{})
	go func() {
		defer close(copied)
		var w io.Writer = io.Discard
		if spec.Stderr != nil {
			w = spec.Stderr
		}
		_, _ = io.Copy(w, c.Stderr)
		_ = c.Stderr.Close()
	}()
	go func() {
		st, err := c.Process.Wait()
		switch {
		case err != nil:
			ch.err = err
		case !st.Success():
			ch.err = fmt.Errorf("%s", st)
		}
		_ = c.Kill() // whatever is left in the job goes with the add-on
		<-copied
		close(ch.done)
	}()
	return ch, nil
}

// profile is the add-on's AppContainer profile, created on first use.
func profile(id string) (*appcontainer.Profile, error) {
	if p, ok := profiles.Load(id); ok {
		return p.(*appcontainer.Profile), nil
	}
	p, err := appcontainer.Open("sona.addon."+id, "Sona add-on "+id, "Sona's sandbox for the add-on "+id)
	if err != nil {
		return nil, err
	}
	profiles.Store(id, p)
	return p, nil
}

// grant gives the profile's SID the folders the policy names. Its own folder
// and its data folder count only by an entry of the SID's own (not by ALL
// APPLICATION PACKAGES): they are the add-on's, whatever their parents say.
func grant(prof *appcontainer.Profile, spec Spec) error {
	exe := spec.Exe
	var sentinels []string
	if appcontainer.Within(exe, spec.Dir) {
		sentinels = []string{exe}
	}
	if _, err := appcontainer.Grant(prof.SID, spec.Dir, appcontainer.AccessRead, true, sentinels...); err != nil {
		return err
	}
	if _, err := appcontainer.Grant(prof.SID, spec.Data, appcontainer.AccessModify, true); err != nil {
		return err
	}
	for _, d := range spec.Read {
		if _, err := appcontainer.Grant(prof.SID, d, appcontainer.AccessRead, false); err != nil {
			return err
		}
	}
	return nil
}
