//go:build cgo

package cdmhost

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The Windows sandbox (sandbox_windows.go) starts the child in an
// AppContainer: a token whose identity is a package SID of Sona's own
// ("sona.cdmhost") with no capabilities. Windows checks every access of such
// a process twice, against the user and against that SID (and ALL
// APPLICATION PACKAGES), so the child reaches only what the SID has been
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

var (
	modUserenv  = windows.NewLazySystemDLL("userenv.dll")
	modAdvapi32 = windows.NewLazySystemDLL("advapi32.dll")

	procCreateAppContainerProfile = modUserenv.NewProc("CreateAppContainerProfile")
	procDeriveAppContainerSid     = modUserenv.NewProc("DeriveAppContainerSidFromAppContainerName")
	procGetAppContainerFolderPath = modUserenv.NewProc("GetAppContainerFolderPath")
	procFreeSid                   = modAdvapi32.NewProc("FreeSid")
)

// hrAlreadyExists is HRESULT_FROM_WIN32(ERROR_ALREADY_EXISTS).
const hrAlreadyExists = 0x800700B7

// hresult is the error for a failed HRESULT of op.
func hresult(op string, hr uintptr) error {
	if hr>>16 == 0x8007 { // FACILITY_WIN32
		return fmt.Errorf("%s: %w", op, syscall.Errno(hr&0xffff))
	}
	return fmt.Errorf("%s: HRESULT %#x", op, uint32(hr))
}

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

func openProfile() (*profile, error) {
	if err := procCreateAppContainerProfile.Find(); err != nil {
		return nil, fmt.Errorf("AppContainers are not available: %w", err)
	}
	name, err := windows.UTF16PtrFromString(appContainerName)
	if err != nil {
		return nil, err
	}
	display, _ := windows.UTF16PtrFromString("Sona playback host")
	desc, _ := windows.UTF16PtrFromString("Sona's sandbox for the process that decrypts and plays protected audio")
	var psid *windows.SID
	hr, _, _ := procCreateAppContainerProfile.Call(
		uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(display)), uintptr(unsafe.Pointer(desc)),
		0, 0, uintptr(unsafe.Pointer(&psid)))
	switch uint32(hr) {
	case 0:
	case hrAlreadyExists:
		hr, _, _ = procDeriveAppContainerSid.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&psid)))
		if hr != 0 {
			return nil, hresult("DeriveAppContainerSidFromAppContainerName", hr)
		}
	default:
		return nil, hresult("CreateAppContainerProfile", hr)
	}
	sid, err := psid.Copy()
	_, _, _ = procFreeSid.Call(uintptr(unsafe.Pointer(psid)))
	if err != nil {
		return nil, fmt.Errorf("copy the AppContainer SID: %w", err)
	}
	sidText, err := windows.UTF16PtrFromString(sid.String())
	if err != nil {
		return nil, err
	}
	var folder *uint16
	hr, _, _ = procGetAppContainerFolderPath.Call(uintptr(unsafe.Pointer(sidText)), uintptr(unsafe.Pointer(&folder)))
	if hr != 0 {
		return nil, hresult("GetAppContainerFolderPath", hr)
	}
	base := windows.UTF16PtrToString(folder)
	windows.CoTaskMemFree(unsafe.Pointer(folder))
	tmp := filepath.Join(base, "AC", "sona-host")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return nil, fmt.Errorf("create the AppContainer's folder: %w", err)
	}
	// The folder's own access list: the profile's AC folder normally hands
	// it to the AppContainer by inheritance, which this makes certain of.
	if err := grantAccess(sid, tmp, accessModify, true); err != nil {
		return nil, err
	}
	return &profile{sid: sid, tmp: tmp}, nil
}

// Access rights granted to the AppContainer's SID.
const (
	// accessRead lets the child read and execute: load the program, its
	// DLLs, the CDM and GStreamer's plugins.
	accessRead = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE
	// accessModify is the child's own folder: read, write, delete.
	accessModify = accessRead | windows.FILE_GENERIC_WRITE | 0x00010000 // DELETE
)

// allApplicationPackages is S-1-15-2-1, which Windows grants read access to
// in its own folders and Program Files.
var allApplicationPackages = sync.OnceValue(func() *windows.SID {
	sid, err := windows.StringToSid("S-1-15-2-1")
	if err != nil {
		return nil
	}
	return sid
})

// grantMu serialises changes to access lists, so that two children starting
// at once do not overwrite each other's change.
var grantMu sync.Mutex

// grantAccess gives sid the rights in mask on path, and for a directory on
// everything under it, unless it has them already: by an entry of its own
// or, for a read, by ALL APPLICATION PACKAGES (Program Files, the Windows
// folder). Nothing else of the list changes: the entry names only sid, and
// stays until the folder goes. own: only an entry for sid counts.
func grantAccess(sid *windows.SID, path string, mask windows.ACCESS_MASK, own bool) error {
	if filepath.Dir(path) == path {
		return fmt.Errorf("refusing to change the access list of %s, a drive root", path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	dir := st.IsDir()
	grantMu.Lock()
	defer grantMu.Unlock()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read the access list of %s: %w", path, err)
	}
	old, _, err := sd.DACL()
	if errors.Is(err, windows.ERROR_OBJECT_NOT_FOUND) || (err == nil && old == nil) {
		return nil // no list: everyone has full access
	}
	if err != nil {
		return fmt.Errorf("read the access list of %s: %w", path, err)
	}
	who := []*windows.SID{sid}
	if !own {
		if all := allApplicationPackages(); all != nil {
			who = append(who, all)
		}
	}
	if aclAllows(old, who, mask, dir) {
		runtime.KeepAlive(sd)
		return nil
	}
	inherit := uint32(windows.NO_INHERITANCE)
	if dir {
		inherit = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	var pin runtime.Pinner
	pin.Pin(sid)
	defer pin.Unpin()
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: mask,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inherit,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_UNKNOWN,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}}, old)
	runtime.KeepAlive(sd)
	if err != nil {
		return fmt.Errorf("build the access list of %s: %w", path, err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return fmt.Errorf("grant the sandbox access to %s: %w", path, err)
	}
	return nil
}

// aclAllows reports whether acl has an entry for one of who that allows all
// of mask on the object itself and, for a directory, on what is under it.
func aclAllows(acl *windows.ACL, who []*windows.SID, mask windows.ACCESS_MASK, dir bool) bool {
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 || ace.Mask&mask != mask {
			continue
		}
		const both = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
		if dir && ace.Header.AceFlags&both != both {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		for _, w := range who {
			if sid.Equals(w) {
				return true
			}
		}
	}
	return false
}

// envValue is the value of the variable key in env (names are
// case-insensitive on Windows), "" when unset.
func envValue(env []string, key string) string {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// withEnv sets the variables kv ("NAME=value") in env, replacing any of the
// same name.
func withEnv(env []string, kv ...string) []string {
	out := make([]string, 0, len(env)+len(kv))
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if !slices.ContainsFunc(kv, func(n string) bool {
			nk, _, _ := strings.Cut(n, "=")
			return strings.EqualFold(k, nk)
		}) {
			out = append(out, e)
		}
	}
	return append(out, kv...)
}

// envBlock is env as CreateProcess takes it: UTF-16 strings, each ended by
// a NUL, sorted by name without regard to case, and a final NUL.
func envBlock(env []string) ([]uint16, error) {
	sorted := slices.Clone(env)
	slices.SortFunc(sorted, func(a, b string) int {
		return strings.Compare(strings.ToUpper(a), strings.ToUpper(b))
	})
	var block []uint16
	for _, kv := range sorted {
		u, err := windows.UTF16FromString(kv)
		if err != nil {
			return nil, fmt.Errorf("environment variable %q: %w", kv, err)
		}
		block = append(block, u...)
	}
	if len(block) == 0 {
		block = append(block, 0)
	}
	return append(block, 0), nil
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
		for _, d := range filepath.SplitList(envValue(env, k)) {
			add(d)
		}
	}
	add(envValue(env, envCDMDir))
	if shim := envValue(env, "SONA_CDM_SHIM"); shim != "" {
		add(filepath.Dir(shim))
	}
	slices.SortFunc(dirs, func(a, b string) int { return len(a) - len(b) })
	var out []string
	for _, d := range dirs {
		if !slices.ContainsFunc(out, func(o string) bool { return within(d, o) }) {
			out = append(out, d)
		}
	}
	return out
}

// within reports whether path is dir or inside it (Windows paths are
// case-insensitive).
func within(path, dir string) bool {
	p, d := strings.ToLower(path), strings.ToLower(dir)
	return p == d || strings.HasPrefix(p, strings.TrimSuffix(d, `\`)+`\`)
}

// securityCapabilities is SECURITY_CAPABILITIES: the AppContainer's SID,
// and no capabilities.
type securityCapabilities struct {
	AppContainerSid *windows.SID
	Capabilities    *windows.SIDAndAttributes
	CapabilityCount uint32
	Reserved        uint32
}

// Process attributes (PROC_THREAD_ATTRIBUTE_*) the x/sys package has no name
// for.
const (
	procThreadAttributeSecurityCapabilities = 0x00020009
	procThreadAttributeChildProcessPolicy   = 0x0002000E
)

// childProcessRestricted is PROCESS_CREATION_CHILD_PROCESS_RESTRICTED.
const childProcessRestricted = 1

// mitigations are the PROCESS_CREATION_MITIGATION_POLICY_*_ALWAYS_ON flags
// the child starts with: heap corruption ends the process, bottom-up ASLR,
// no legacy extension points, no fonts from outside, no images from remote
// shares and none from files with a low integrity label (a file the child
// wrote).
const mitigations uint64 = 1<<12 | 1<<16 | 1<<32 | 1<<48 | 1<<52 | 1<<56

// uiRestrictions are every JOB_OBJECT_UILIMIT_* limit.
const uiRestrictions = windows.JOB_OBJECT_UILIMIT_DESKTOP | windows.JOB_OBJECT_UILIMIT_DISPLAYSETTINGS |
	windows.JOB_OBJECT_UILIMIT_EXITWINDOWS | windows.JOB_OBJECT_UILIMIT_GLOBALATOMS | windows.JOB_OBJECT_UILIMIT_HANDLES |
	windows.JOB_OBJECT_UILIMIT_READCLIPBOARD | windows.JOB_OBJECT_UILIMIT_SYSTEMPARAMETERS | windows.JOB_OBJECT_UILIMIT_WRITECLIPBOARD

// newJob creates the child's job object: killed when its last handle (the
// parent's) closes, one process at most, no UI access. A process in it cannot
// break away.
func newJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("CreateJobObject: %w", err)
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
	limits.BasicLimitInformation.ActiveProcessLimit = 1
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("limit the job object: %w", err)
	}
	ui := windows.JOBOBJECT_BASIC_UI_RESTRICTIONS{UIRestrictionsClass: uiRestrictions}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectBasicUIRestrictions,
		uintptr(unsafe.Pointer(&ui)), uint32(unsafe.Sizeof(ui))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("restrict the job object's UI: %w", err)
	}
	return job, nil
}

// winChild is the parent's handle on a child it created: Go's own handle
// (os.FindProcess) cannot terminate it.
type winChild struct {
	mu           sync.Mutex
	process, job windows.Handle // 0 once the child has exited
}

// kill terminates the child.
func (c *winChild) kill() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.process == 0 {
		return os.ErrProcessDone
	}
	return windows.TerminateProcess(c.process, 1)
}

// reap waits for the child to exit and closes the handles; closing the job's
// is what kills anything still in it.
func (c *winChild) reap() {
	_, _ = windows.WaitForSingleObject(c.process, windows.INFINITE)
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = windows.CloseHandle(c.job)
	_ = windows.CloseHandle(c.process)
	c.process, c.job = 0, 0
}

// startInAppContainer starts exe as the child in prof's AppContainer: the
// folders it reads are granted first (readFolders), its environment points
// its temporary files and GStreamer's registry at prof's own folder, and its
// standard handles are fresh pipes (the only handles it inherits).
func startInAppContainer(prof *profile, exe string, env []string) (proc, error) {
	for _, d := range readFolders(exe, env) {
		if err := grantAccess(prof.sid, d, accessRead, false); err != nil {
			return proc{}, err
		}
	}
	tmp := filepath.Join(prof.tmp, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return proc{}, fmt.Errorf("create the AppContainer's temporary folder: %w", err)
	}
	env = withEnv(env,
		envAppContainerChild+"=1",
		"TEMP="+tmp, "TMP="+tmp,
		// The scanner is a process of its own, which the child may not
		// start: GStreamer scans the plugins in the process, once, into a
		// registry that later children read.
		"GST_REGISTRY_1_0="+filepath.Join(prof.tmp, "gst-registry.bin"),
		"GST_REGISTRY_FORK=no",
	)
	return createInAppContainer(prof.sid, exe, env)
}

// createInAppContainer creates the child's process, with fresh pipes for its
// standard handles and a job object around it.
func createInAppContainer(sid *windows.SID, exe string, env []string) (p proc, err error) {
	// The pipes' child ends are inheritable, and only the three handles of
	// the attribute list are inherited.
	var parentEnds, childEnds []windows.Handle
	defer func() {
		for _, h := range childEnds {
			_ = windows.CloseHandle(h)
		}
		if err != nil {
			for _, h := range parentEnds {
				_ = windows.CloseHandle(h)
			}
		}
	}()
	// pipe makes a pipe; childReads says which end the child gets.
	pipe := func(childReads bool) (parent, child windows.Handle, err error) {
		var r, w windows.Handle
		if err = windows.CreatePipe(&r, &w, nil, 0); err != nil {
			return 0, 0, fmt.Errorf("CreatePipe: %w", err)
		}
		parent, child = r, w
		if childReads {
			parent, child = w, r
		}
		parentEnds, childEnds = append(parentEnds, parent), append(childEnds, child)
		if err = windows.SetHandleInformation(child, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
			return 0, 0, fmt.Errorf("make a pipe inheritable: %w", err)
		}
		return parent, child, nil
	}
	pIn, cIn, err := pipe(true)
	if err != nil {
		return proc{}, err
	}
	pOut, cOut, err := pipe(false)
	if err != nil {
		return proc{}, err
	}
	pErr, cErr, err := pipe(false)
	if err != nil {
		return proc{}, err
	}

	al, err := windows.NewProcThreadAttributeList(4)
	if err != nil {
		return proc{}, fmt.Errorf("NewProcThreadAttributeList: %w", err)
	}
	defer al.Delete()
	caps := securityCapabilities{AppContainerSid: sid}
	inherited := []windows.Handle{cIn, cOut, cErr}
	childPolicy := uint32(childProcessRestricted)
	policy := mitigations
	for _, a := range []struct {
		name string
		attr uintptr
		val  unsafe.Pointer
		size uintptr
	}{
		{"the AppContainer", procThreadAttributeSecurityCapabilities, unsafe.Pointer(&caps), unsafe.Sizeof(caps)},
		{"the handle list", windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&inherited[0]), uintptr(len(inherited)) * unsafe.Sizeof(inherited[0])},
		{"the mitigation policy", windows.PROC_THREAD_ATTRIBUTE_MITIGATION_POLICY, unsafe.Pointer(&policy), unsafe.Sizeof(policy)},
		{"the child process policy", procThreadAttributeChildProcessPolicy, unsafe.Pointer(&childPolicy), unsafe.Sizeof(childPolicy)},
	} {
		if err := al.Update(a.attr, a.val, a.size); err != nil {
			return proc{}, fmt.Errorf("set %s: %w", a.name, err)
		}
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: al.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = cIn, cOut, cErr

	exe16, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return proc{}, err
	}
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{exe}))
	if err != nil {
		return proc{}, err
	}
	dir := childDir()
	dir16, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return proc{}, err
	}
	block, err := envBlock(env)
	if err != nil {
		return proc{}, err
	}
	job, err := newJob()
	if err != nil {
		return proc{}, err
	}
	// Suspended, so that it is in the job before it runs a thread; with no
	// console, because a console process starts conhost.exe, which the
	// child-process restriction refuses (the child then fails to initialise,
	// 0xc0000142). Its standard handles are the pipes.
	var pi windows.ProcessInformation
	const flags = windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_SUSPENDED | windows.CREATE_UNICODE_ENVIRONMENT | windows.DETACHED_PROCESS
	if err = windows.CreateProcess(exe16, cmdline, nil, nil, true, flags, &block[0], dir16, &si.StartupInfo, &pi); err != nil {
		_ = windows.CloseHandle(job)
		return proc{}, fmt.Errorf("CreateProcess in the AppContainer: %w", err)
	}
	fail := func(err error) (proc, error) {
		_ = windows.TerminateProcess(pi.Process, 1)
		_ = windows.CloseHandle(pi.Thread)
		_ = windows.CloseHandle(pi.Process)
		_ = windows.CloseHandle(job)
		return proc{}, err
	}
	if err = windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		return fail(fmt.Errorf("put the child in its job object: %w", err))
	}
	if _, err = windows.ResumeThread(pi.Thread); err != nil {
		return fail(fmt.Errorf("start the child: %w", err))
	}
	_ = windows.CloseHandle(pi.Thread)
	osProc, err := os.FindProcess(int(pi.ProcessId))
	if err != nil {
		_ = windows.TerminateProcess(pi.Process, 1)
		_ = windows.CloseHandle(pi.Process)
		_ = windows.CloseHandle(job)
		return proc{}, err
	}
	wc := &winChild{process: pi.Process, job: job}
	go wc.reap()
	return proc{
		cmd:    &exec.Cmd{Path: exe, Args: []string{exe}, Env: env, Dir: dir, Process: osProc},
		stdin:  os.NewFile(uintptr(pIn), "cdmhost-stdin"),
		stdout: os.NewFile(uintptr(pOut), "cdmhost-stdout"),
		stderr: os.NewFile(uintptr(pErr), "cdmhost-stderr"),
		kill:   wc.kill,
	}, nil
}
