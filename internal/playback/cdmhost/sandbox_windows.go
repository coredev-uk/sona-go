//go:build cgo

package cdmhost

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// Sandbox describes how the CDM host child is confined on Windows: the
// parent starts it in an AppContainer (the profile "sona.cdmhost", with no
// capabilities, so no network of any kind and no access to the user's files
// beyond the folders granted to it, appcontainer_windows.go) inside a job
// object that kills it with the parent and lets it start no process, with
// the process mitigations that fit. Where the parent cannot set that up
// (the profile, the folder grants or the process creation fail), the child
// runs unconfined and Reason says why. Unlike on macOS, an unconfined child
// does not load the CDM (loadsCDM), as on Linux.
type Sandbox struct {
	AppContainer  bool // the child's token is an AppContainer's, at low integrity
	NetworkDenied bool // its own TCP connect() to the loopback was refused
	// FilesHidden: the child could not list Sona's folders or the user's
	// local app data folder.
	FilesHidden bool
	Reason      string // why a layer is missing, for the log; "" when Active
	// Unconfined is why the child runs outside an AppContainer ("" when it
	// runs in one); the parent sets it.
	Unconfined playback.SandboxGap
}

// Gaps are the missing layers (Diagnostics), or why the child runs
// unconfined.
func (s Sandbox) Gaps() []playback.SandboxGap {
	if !s.AppContainer {
		if s.Unconfined == "" {
			return []playback.SandboxGap{playback.SandboxGapNoAppContainer}
		}
		return []playback.SandboxGap{s.Unconfined}
	}
	var gaps []playback.SandboxGap
	if !s.NetworkDenied {
		gaps = append(gaps, playback.SandboxGapNetwork)
	}
	if !s.FilesHidden {
		gaps = append(gaps, playback.SandboxGapUserFiles)
	}
	return gaps
}

// Active reports whether the child runs in the AppContainer and its
// self-checks found the network and the user's folders out of reach.
func (s Sandbox) Active() bool { return s.AppContainer && s.NetworkDenied && s.FilesHidden }

// Kind is the sandbox the child runs in (Diagnostics):
// playback.SandboxAppContainer, or playback.SandboxNone when it is
// unconfined.
func (s Sandbox) Kind() string {
	if s.AppContainer {
		return playback.SandboxAppContainer
	}
	return playback.SandboxNone
}

// Attrs are the layers as key-value pairs for a log line (playspike).
func (s Sandbox) Attrs() []any {
	return []any{
		"appcontainer", s.AppContainer, "network_denied", s.NetworkDenied, "files_hidden", s.FilesHidden,
		"reason", strconv.Quote(s.Reason),
	}
}

// envAllowUnsandboxed lets a test child load the CDM outside the
// AppContainer (a machine that refuses AppContainers). Only a test binary
// honours it (testing.Testing): a Sona build never loads the CDM outside
// its sandbox.
const envAllowUnsandboxed = "SONA_CDMHOST_ALLOW_UNSANDBOXED"

// loadsCDM reports whether the child may load the CDM: only in the
// AppContainer, with the network out of reach (as Linux's seccomp filter).
func (s Sandbox) loadsCDM() bool {
	return (s.AppContainer && s.NetworkDenied) || (testing.Testing() && os.Getenv(envAllowUnsandboxed) == "1")
}

// envAppContainerChild tells the child it was started in the AppContainer.
const envAppContainerChild = "SONA_CDMHOST_APPCONTAINER"

// sandboxEnv are the sandbox's own variables, never inherited by a child.
var sandboxEnv = []string{envAppContainerChild, envHiddenDirs}

// pluginPathEnv are the variables that add plugin folders to the system's,
// which the child does not inherit: it needs no video plugin (music videos
// are decoded by the CDM's own decoder), and the UI process names the video
// plugins' folder in GST_PLUGIN_PATH_1_0 (internal/playback/gstbundle), so a
// child that inherited it would load Direct3D 11 and the video plugins in its
// plugin scan. Its plugins are the system folder's, which GStreamer reads
// from GST_PLUGIN_SYSTEM_PATH_1_0.
var pluginPathEnv = []string{"GST_PLUGIN_PATH", "GST_PLUGIN_PATH_1_0"}

// parentSandbox is the parent's part of the sandbox: why the child runs
// unconfined (gap; reason in the system's words, for the log), "" when it
// runs in the AppContainer.
type parentSandbox struct {
	gap    playback.SandboxGap
	reason string
}

// startSandboxed starts the child in the AppContainer (appcontainer_windows.go),
// or unconfined when that cannot be set up (parentSandbox.reason).
func startSandboxed(exe string, env []string, log *slog.Logger) (proc, parentSandbox, error) {
	prof, err := appContainer()
	if err == nil {
		var p proc
		if p, err = startInAppContainer(prof, exe, env, log); err == nil {
			return p, parentSandbox{}, nil
		}
	}
	ps := parentSandbox{gap: playback.SandboxGapNoAppContainer, reason: "no Windows sandbox: " + err.Error()}
	log.Warn("cdmhost: starting the playback host without the Windows sandbox", "reason", ps.reason)
	p, err := startChild([]string{exe}, env, nil)
	return p, ps, err
}

// withParent adds the parent's reason for an unconfined child.
func (s Sandbox) withParent(ps parentSandbox) Sandbox {
	s.Unconfined = ps.gap
	if ps.reason != "" {
		if s.Reason != "" {
			s.Reason = ps.reason + "; " + s.Reason
		} else {
			s.Reason = ps.reason
		}
	}
	return s
}

// enterSandbox runs in the child before the CDM or GStreamer load. The
// parent already confined it (CreateProcess applies the AppContainer before
// the child's first instruction); it checks the confinement holds.
func enterSandbox() Sandbox {
	if os.Getenv(envAppContainerChild) != "1" {
		return Sandbox{Reason: "not started in an AppContainer"} // the parent says why (withParent)
	}
	var sb Sandbox
	var reasons []string
	if err := inAppContainer(); err != nil {
		reasons = append(reasons, "not in an AppContainer: "+err.Error())
	} else {
		sb.AppContainer = true
	}
	if err := networkRefused(); err != nil {
		reasons = append(reasons, "network allowed: "+err.Error())
	} else {
		sb.NetworkDenied = true
	}
	if err := filesHidden(); err != nil {
		reasons = append(reasons, "user folders readable: "+err.Error())
	} else {
		sb.FilesHidden = true
	}
	sb.Reason = strings.Join(reasons, "; ")
	return sb
}

// tokenIsAppContainer is TOKEN_INFORMATION_CLASS's TokenIsAppContainer.
const tokenIsAppContainer = 29

// mandatoryLowRID is SECURITY_MANDATORY_LOW_RID: the integrity level of an
// AppContainer process.
const mandatoryLowRID = 0x1000

// inAppContainer checks that the process's token is an AppContainer's and
// that its integrity level is low.
func inAppContainer() error {
	tok := windows.GetCurrentProcessToken()
	var isAC, n uint32
	if err := windows.GetTokenInformation(tok, tokenIsAppContainer, (*byte)(unsafe.Pointer(&isAC)), uint32(unsafe.Sizeof(isAC)), &n); err != nil {
		return fmt.Errorf("TokenIsAppContainer: %w", err)
	}
	if isAC == 0 {
		return errors.New("the token is not an AppContainer's")
	}
	buf := make([]byte, 128)
	for {
		err := windows.GetTokenInformation(tok, windows.TokenIntegrityLevel, &buf[0], uint32(len(buf)), &n)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || int(n) <= len(buf) {
			return fmt.Errorf("TokenIntegrityLevel: %w", err)
		}
		buf = make([]byte, n)
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buf[0]))
	sid := label.Label.Sid
	if sid == nil || sid.SubAuthorityCount() == 0 {
		return errors.New("the token has no integrity level")
	}
	if rid := sid.SubAuthority(uint32(sid.SubAuthorityCount()) - 1); rid > mandatoryLowRID {
		return fmt.Errorf("integrity level %#x is above low", rid)
	}
	return nil
}

// networkRefused checks that the child cannot reach the network: a process
// of the AppContainer holds none of the network capabilities, so the network
// stack refuses its outbound TCP connect() to an address off the machine at
// once, with WSAEACCES (an unconfined process gets a connection, or a
// timeout or an ICMP error after seconds). 192.0.2.1 is TEST-NET-1 (RFC
// 5737): nothing answers there, so an unconfined child leaks one SYN to an
// address that goes nowhere. A UDP connect() proves nothing here (it only
// sets the peer address), nor does a connection to a socket of the child's
// own on the loopback (the same package may talk to itself).
func networkRefused() error {
	c, err := net.DialTimeout("tcp", "192.0.2.1:9", 2*time.Second)
	switch {
	case err == nil:
		_ = c.Close()
		return errors.New("connect(tcp 192.0.2.1:9) succeeded")
	case errors.Is(err, windows.WSAEACCES):
		return nil
	}
	return fmt.Errorf("connect(tcp 192.0.2.1:9) was not refused by the sandbox: %w", err)
}

// envHiddenDirs lists the folders the AppContainer must not be able to list
// (os.PathListSeparator-separated): Sona's own and the user's app data
// folders. The parent names them because the AppContainer's own environment
// points %LOCALAPPDATA% and %APPDATA% into its package folder.
const envHiddenDirs = "SONA_CDMHOST_HIDDEN_DIRS"

// filesHidden checks that the child cannot list the folders the parent names
// (envHiddenDirs), which the AppContainer has no access to: at least one must
// exist, and none may be listable.
func filesHidden() error {
	checked := 0
	for _, d := range filepath.SplitList(os.Getenv(envHiddenDirs)) {
		_, err := os.ReadDir(d)
		switch {
		case err == nil:
			return fmt.Errorf("listed %s", d)
		case errors.Is(err, fs.ErrPermission):
			checked++
		case errors.Is(err, fs.ErrNotExist):
		default:
			return fmt.Errorf("list %s: %w", d, err)
		}
	}
	if checked == 0 {
		return errors.New("no folder of the user's to check")
	}
	return nil
}
