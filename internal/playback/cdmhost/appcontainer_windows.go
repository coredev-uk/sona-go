//go:build cgo

package cdmhost

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"golang.org/x/sys/windows"

	"github.com/coredev-uk/sona-go/internal/appcontainer"
	"github.com/coredev-uk/sona-go/internal/config"
)

// The Windows sandbox (sandbox_windows.go) starts the child in an
// AppContainer (internal/appcontainer): a token whose identity is a package SID
// of Sona's own ("sona.cdmhost") with no capabilities. Windows checks every
// access of such a process twice, against the user and against that SID (and
// ALL APPLICATION PACKAGES), so the child reaches only what the SID has been
// granted, and the network stack refuses all its traffic, the loopback
// included, because it holds none of the network capabilities.
//
// Around that, the child is in a job object that kills it when the parent
// goes away and when the job's last handle closes, allows it no other
// process, and takes away the desktop's handles, clipboard and settings; and
// it starts with the mitigations that cost it nothing (no child processes,
// no remote or low-integrity images, terminate on heap corruption).

// appContainerName is the AppContainer profile's name: per user, kept
// between runs (the registry maps it to the SID and the profile folder).
const appContainerName = "sona.cdmhost"

// profile is Sona's AppContainer.
type profile struct {
	sid *windows.SID
	// tmp is a folder only the AppContainer writes: its temporary files
	// and GStreamer's plugin registry, which survives between children.
	tmp string
}

// appContainer is the profile, created on the first spawn (or found, when
// an earlier run created it), or why the children of this process run
// unconfined.
var appContainer = sync.OnceValues(openProfile)

func openProfile() (*profile, error) { return openNamedProfile(appContainerName) }

// openNamedProfile creates the AppContainer profile profileName, or finds it
// when it exists. Sona's is appContainerName; a test makes its own.
func openNamedProfile(profileName string) (*profile, error) {
	p, err := appcontainer.Open(profileName, "Sona playback host", "Sona's sandbox for the process that decrypts and plays protected audio")
	if err != nil {
		return nil, err
	}
	tmp := filepath.Join(p.Folder, "AC", "sona-host")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return nil, fmt.Errorf("create the AppContainer's folder: %w", err)
	}
	// The folder's own access list: the profile's AC folder normally hands
	// it to the AppContainer by inheritance, which this makes certain of.
	if _, err := appcontainer.Grant(p.SID, tmp, appcontainer.AccessModify, true); err != nil {
		return nil, err
	}
	return &profile{sid: p.SID, tmp: tmp}, nil
}

// gstreamerDirs are the folders of the GStreamer the parent is linked to:
// the one holding its DLL, and the lib and libexec folders of its prefix
// (the plugins and the plugin scanner), which GStreamer finds relative to
// the DLL.
func gstreamerDirs() []string {
	name, err := windows.UTF16PtrFromString("libgstreamer-1.0-0.dll")
	if err != nil {
		return nil
	}
	var mod windows.Handle
	if err := windows.GetModuleHandleEx(windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, name, &mod); err != nil {
		return nil
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetModuleFileName(mod, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		return nil
	}
	dll := filepath.Dir(windows.UTF16ToString(buf[:n]))
	prefix := dll
	if b := filepath.Base(dll); strings.EqualFold(b, "bin") || strings.EqualFold(b, "lib") {
		prefix = filepath.Dir(dll)
	}
	return []string{dll, filepath.Join(prefix, "lib"), filepath.Join(prefix, "libexec")}
}

// readFolders are the folders the child reads: the program's (it is the
// same executable, with the shim DLL and, in a bundle, GStreamer beside it),
// GStreamer's, the plugin folders the environment names, the CDM's
// (envCDMDir) and the shim DLL's (SONA_CDM_SHIM, a development or test
// build). Only existing folders, and none inside another of the list.
func readFolders(exe string, env []string) []string {
	var dirs []string
	add := func(p string) {
		if !filepath.IsAbs(p) {
			return
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			dirs = append(dirs, filepath.Clean(p))
		}
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	add(filepath.Dir(exe))
	for _, d := range gstreamerDirs() {
		add(d)
	}
	for _, k := range []string{"GST_PLUGIN_SYSTEM_PATH", "GST_PLUGIN_SYSTEM_PATH_1_0", "GST_PLUGIN_PATH", "GST_PLUGIN_PATH_1_0"} {
		for _, d := range filepath.SplitList(appcontainer.EnvValue(env, k)) {
			add(d)
		}
	}
	add(appcontainer.EnvValue(env, envCDMDir))
	if shim := appcontainer.EnvValue(env, "SONA_CDM_SHIM"); shim != "" {
		add(filepath.Dir(shim))
	}
	slices.SortFunc(dirs, func(a, b string) int { return len(a) - len(b) })
	var out []string
	for _, d := range dirs {
		if !slices.ContainsFunc(out, func(o string) bool { return appcontainer.Within(d, o) }) {
			out = append(out, d)
		}
	}
	return out
}

// hiddenFolders are the folders the child's self-check must find
// unreadable (envHiddenDirs): Sona's own and the user's app data folders.
func hiddenFolders() []string {
	var dirs []string
	if p, err := config.ResolvePaths(os.Getenv); err == nil {
		dirs = append(dirs, p.Data, p.Config, filepath.Dir(p.Data), filepath.Dir(p.Config))
	}
	if home := os.Getenv("USERPROFILE"); filepath.IsAbs(home) {
		dirs = append(dirs, filepath.Join(home, "AppData", "Local"), filepath.Join(home, "AppData", "Roaming"))
	}
	slices.Sort(dirs)
	return slices.Compact(dirs)
}

// registryFile is where the child keeps GStreamer's plugin registry for the
// parent's setting of GST_REGISTRY_1_0 (want, "" when unset): in the
// AppContainer's own folder, one file per setting, and never the parent's
// own file. The parent loads plugins from the registry it has, and a child
// that could write that file could name a plugin to load into the parent.
func (p *profile) registryFile(want string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(want)))
	return filepath.Join(p.tmp, "gst-registry-"+hex.EncodeToString(sum[:6])+".bin")
}

// loggedRegistries are the registry files startInAppContainer has logged.
var loggedRegistries sync.Map

// startInAppContainer starts exe as the child in prof's AppContainer: the
// folders it reads are granted first (readFolders), its environment points
// its temporary files and GStreamer's registry at prof's own folder, and its
// standard handles are fresh pipes (the only handles it inherits).
func startInAppContainer(prof *profile, exe string, env []string, log *slog.Logger) (proc, error) {
	// The program is the file that must be covered: an update replaces it
	// with the rest of the folder (grantNeeded).
	program := exe
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		program = r
	}
	for _, d := range readFolders(exe, env) {
		var sentinels []string
		if appcontainer.Within(program, d) {
			sentinels = []string{program}
		}
		refreshed, err := appcontainer.Grant(prof.sid, d, appcontainer.AccessRead, false, sentinels...)
		if err != nil {
			return proc{}, err
		}
		if refreshed {
			log.Info("cdmhost: granted the sandbox read access to a folder (first run, or an update replaced its files)", "folder", d)
		}
	}
	tmp := filepath.Join(prof.tmp, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return proc{}, fmt.Errorf("create the AppContainer's temporary folder: %w", err)
	}
	registry := prof.registryFile(appcontainer.EnvValue(env, "GST_REGISTRY_1_0"))
	if _, seen := loggedRegistries.LoadOrStore(registry, true); !seen {
		// The packaging smoke test reads the registry from here.
		log.Info("cdmhost: the host's GStreamer plugin registry", "path", registry)
	}
	env = appcontainer.WithEnv(env,
		envAppContainerChild+"=1",
		envHiddenDirs+"="+strings.Join(hiddenFolders(), string(os.PathListSeparator)),
		"TEMP="+tmp, "TMP="+tmp,
		// The scanner is a process of its own, which the child may not
		// start: GStreamer scans the plugins in the process, once, into a
		// registry that later children read.
		"GST_REGISTRY_1_0="+registry,
		"GST_REGISTRY_FORK=no",
	)
	dir := childDir()
	c, err := appcontainer.Start(appcontainer.Options{SID: prof.sid, Exe: exe, Env: env, Dir: dir, MaxProcesses: 1})
	if err != nil {
		return proc{}, err
	}
	return proc{
		cmd:    &exec.Cmd{Path: exe, Args: []string{exe}, Env: env, Dir: dir, Process: c.Process},
		stdin:  c.Stdin,
		stdout: c.Stdout,
		stderr: c.Stderr,
		kill:   c.Kill,
	}, nil
}
