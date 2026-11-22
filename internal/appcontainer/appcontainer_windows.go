//go:build windows

package appcontainer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUserenv  = windows.NewLazySystemDLL("userenv.dll")
	modAdvapi32 = windows.NewLazySystemDLL("advapi32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procCreateAppContainerProfile = modUserenv.NewProc("CreateAppContainerProfile")
	procDeleteAppContainerProfile = modUserenv.NewProc("DeleteAppContainerProfile")
	procDeriveAppContainerSid     = modUserenv.NewProc("DeriveAppContainerSidFromAppContainerName")
	procGetAppContainerFolderPath = modUserenv.NewProc("GetAppContainerFolderPath")
	procFreeSid                   = modAdvapi32.NewProc("FreeSid")

	// procSetInformationJobObject is called through LazyProc.Call, which keeps
	// the pointers passed as uintptr alive and unmoved across the syscall;
	// windows.SetInformationJobObject takes a plain uintptr, which does not.
	procSetInformationJobObject = modKernel32.NewProc("SetInformationJobObject")
)

// setJobInfo is SetInformationJobObject for the info struct at info, size
// bytes long.
func setJobInfo(job windows.Handle, class uint32, info unsafe.Pointer, size uintptr) error {
	r, _, err := procSetInformationJobObject.Call(uintptr(job), uintptr(class), uintptr(info), size)
	if r != 0 {
		return nil
	}
	if err == windows.ERROR_SUCCESS {
		return syscall.EINVAL
	}
	return err
}

// hrAlreadyExists is HRESULT_FROM_WIN32(ERROR_ALREADY_EXISTS).
const hrAlreadyExists = 0x800700B7

// hrNotFound is HRESULT_FROM_WIN32(ERROR_NOT_FOUND): DeleteAppContainerProfile
// on a profile that does not exist.
const hrNotFound = 0x80070490

// hresult is the error for a failed HRESULT of op.
func hresult(op string, hr uintptr) error {
	if hr>>16 == 0x8007 { // FACILITY_WIN32
		return fmt.Errorf("%s: %w", op, syscall.Errno(hr&0xffff))
	}
	return fmt.Errorf("%s: HRESULT %#x", op, uint32(hr))
}

// Profile is an AppContainer profile: per user, kept between runs (the
// registry maps it to the SID and the package folder).
type Profile struct {
	SID *windows.SID
	// Folder is the profile's package folder, %LOCALAPPDATA%\Packages\<name>.
	Folder string
}

// Open creates the AppContainer profile name, or finds it when it exists. The
// display name and description show where Windows lists the package.
func Open(name, display, description string) (*Profile, error) {
	if err := procCreateAppContainerProfile.Find(); err != nil {
		return nil, fmt.Errorf("AppContainers are not available: %w", err)
	}
	name16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	display16, err := windows.UTF16PtrFromString(display)
	if err != nil {
		return nil, err
	}
	desc16, err := windows.UTF16PtrFromString(description)
	if err != nil {
		return nil, err
	}
	var psid *windows.SID
	hr, _, _ := procCreateAppContainerProfile.Call(
		uintptr(unsafe.Pointer(name16)), uintptr(unsafe.Pointer(display16)), uintptr(unsafe.Pointer(desc16)),
		0, 0, uintptr(unsafe.Pointer(&psid)))
	switch uint32(hr) {
	case 0:
	case hrAlreadyExists:
		hr, _, _ = procDeriveAppContainerSid.Call(uintptr(unsafe.Pointer(name16)), uintptr(unsafe.Pointer(&psid)))
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
	base, err := containerFolder(sid)
	if err != nil {
		return nil, err
	}
	return &Profile{SID: sid, Folder: base}, nil
}

// containerFolder is the profile folder of the AppContainer sid
// (%LOCALAPPDATA%\Packages\<name>).
func containerFolder(sid *windows.SID) (string, error) {
	sidText, err := windows.UTF16PtrFromString(sid.String())
	if err != nil {
		return "", err
	}
	var folder *uint16
	hr, _, _ := procGetAppContainerFolderPath.Call(uintptr(unsafe.Pointer(sidText)), uintptr(unsafe.Pointer(&folder)))
	if hr != 0 {
		return "", hresult("GetAppContainerFolderPath", hr)
	}
	base := windows.UTF16PtrToString(folder)
	windows.CoTaskMemFree(unsafe.Pointer(folder))
	return base, nil
}

// Access rights granted to an AppContainer's SID.
const (
	// AccessRead lets a process read and execute: load a program and its
	// DLLs.
	AccessRead = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE
	// AccessModify is a folder of the AppContainer's own: read, write,
	// delete.
	AccessModify = AccessRead | windows.FILE_GENERIC_WRITE | 0x00010000 // DELETE
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

// Grant gives sid the rights in mask on path, and for a directory on
// everything under it, unless it has them already: by an entry of its own
// or, for a read, by ALL APPLICATION PACKAGES (Program Files, the Windows
// folder). Nothing else of the list changes: the entry names only sid, and
// stays until the folder goes or Remove (the uninstaller) removes it.
// own: only an entry for sid counts.
//
// sentinels are files under a directory that the grant must cover too. Where
// the folder has the entry and one of them lacks it (an update renamed new
// files in: grantNeeded says why), the entries the tree inherits are
// recomputed from the folder's list (resetInherited). refreshed reports that
// a list was set.
func Grant(sid *windows.SID, path string, mask windows.ACCESS_MASK, own bool, sentinels ...string) (refreshed bool, err error) {
	if filepath.Dir(path) == path {
		return false, fmt.Errorf("refusing to change the access list of %s, a drive root", path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	dir := st.IsDir()
	grantMu.Lock()
	defer grantMu.Unlock()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, fmt.Errorf("read the access list of %s: %w", path, err)
	}
	old, _, err := sd.DACL()
	if errors.Is(err, windows.ERROR_OBJECT_NOT_FOUND) || (err == nil && old == nil) {
		return false, nil // no list: everyone has full access
	}
	if err != nil {
		return false, fmt.Errorf("read the access list of %s: %w", path, err)
	}
	who := []*windows.SID{sid}
	if !own {
		if all := allApplicationPackages(); all != nil {
			who = append(who, all)
		}
	}
	switch grantNeededFor(old, who, mask, dir, sentinels) {
	case grantNone:
		runtime.KeepAlive(sd)
		return false, nil
	case grantInherit:
		err := resetInherited(path, old, sd)
		runtime.KeepAlive(sd)
		if err != nil {
			return false, fmt.Errorf("hand the sandbox's access down in %s: %w", path, err)
		}
		return true, nil
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
		return false, fmt.Errorf("build the access list of %s: %w", path, err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return false, fmt.Errorf("grant the sandbox access to %s: %w", path, err)
	}
	return true, nil
}

// procTreeSetNamedSecurityInfo is advapi32's TreeSetNamedSecurityInfoW, which
// x/sys/windows does not wrap.
var procTreeSetNamedSecurityInfo = modAdvapi32.NewProc("TreeSetNamedSecurityInfoW")

// The TreeSetNamedSecurityInfo arguments resetInherited uses.
const (
	treeSecInfoResetKeepExplicit = 3 // TREE_SEC_INFO_RESET_KEEP_EXPLICIT
	progressInvokeNever          = 1 // PROG_INVOKE_SETTING: ProgressInvokeNever
)

// resetInherited has everything below the directory path inherit its list
// again: the inherited entries of each child are recomputed from its parent's
// and its explicit entries stay, and the directory's own list, acl (what sd
// holds), is set as it is. A rename within a volume keeps a file's access
// list, so files renamed in carry the entries of the folder they came from.
// Setting the folder's list again does not reach them (observed on
// windows-2025, where taking the entry off and adding it afresh did not
// either); this does, as Explorer's "replace all child object permissions"
// does. The directory keeps whether it inherits from its own parent (the
// protected flag of sd).
func resetInherited(path string, acl *windows.ACL, sd *windows.SECURITY_DESCRIPTOR) error {
	info := uint32(windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION)
	if ctl, _, err := sd.Control(); err == nil && ctl&windows.SE_DACL_PROTECTED != 0 {
		info = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	r, _, _ := procTreeSetNamedSecurityInfo.Call(
		uintptr(unsafe.Pointer(name)), uintptr(windows.SE_FILE_OBJECT), uintptr(info),
		0, 0, uintptr(unsafe.Pointer(acl)), 0, // owner, group, DACL, SACL
		treeSecInfoResetKeepExplicit, 0, progressInvokeNever, 0) // action, no progress callback
	if r != 0 {
		return windows.Errno(r)
	}
	return nil
}

// grantNeededFor is grantNeeded on Windows access lists: acl is the list of
// the object being granted (a directory when dir) and sentinels are paths of
// files whose lists are read (one that can't be read, or has no list, is
// left out: there is nothing to cover or nothing to refuse).
func grantNeededFor(acl *windows.ACL, who []*windows.SID, mask windows.ACCESS_MASK, dir bool, sentinels []string) grantAction {
	whoText := make([]string, len(who))
	for i, w := range who {
		whoText[i] = w.String()
	}
	if !dir {
		if acesAllow(entriesOf(acl), whoText, uint32(mask), false) {
			return grantNone
		}
		return grantFolder
	}
	var files [][]aclEntry
	for _, f := range sentinels {
		sd, err := windows.GetNamedSecurityInfo(f, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			continue
		}
		if fileACL, _, err := sd.DACL(); err == nil && fileACL != nil {
			files = append(files, entriesOf(fileACL))
		}
		runtime.KeepAlive(sd)
	}
	return grantNeeded(entriesOf(acl), files, whoText, uint32(mask))
}

// entriesOf is the allow entries of acl.
func entriesOf(acl *windows.ACL) []aclEntry {
	var out []aclEntry
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		out = append(out, aclEntry{
			sid:   (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String(),
			mask:  uint32(ace.Mask),
			flags: ace.Header.AceFlags,
		})
	}
	return out
}

// aclAllows reports whether acl has an entry for one of who that allows all
// of mask on the object itself and, for a directory, on what is under it.
func aclAllows(acl *windows.ACL, who []*windows.SID, mask windows.ACCESS_MASK, dir bool) bool {
	whoText := make([]string, len(who))
	for i, w := range who {
		whoText[i] = w.String()
	}
	return acesAllow(entriesOf(acl), whoText, uint32(mask), dir)
}

// EnvValue is the value of the variable key in env (names are
// case-insensitive on Windows), "" when unset.
func EnvValue(env []string, key string) string {
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// WithEnv sets the variables kv ("NAME=value") in env, replacing any of the
// same name.
func WithEnv(env []string, kv ...string) []string {
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

// Within reports whether path is dir or inside it (Windows paths are
// case-insensitive).
func Within(path, dir string) bool {
	p, d := strings.ToLower(path), strings.ToLower(dir)
	return p == d || strings.HasPrefix(p, strings.TrimSuffix(d, `\`)+`\`)
}

// Capability SIDs an AppContainer may be given (winnt.h' well-known SIDs).
const (
	// CapabilityInternetClient is internetClient: outbound connections to the
	// internet. The loopback and the private network are other capabilities,
	// and so out of reach.
	CapabilityInternetClient = "S-1-15-3-1"
)

// securityCapabilities is SECURITY_CAPABILITIES: the AppContainer's SID and
// the capabilities it holds.
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

// seGroupEnabled is SE_GROUP_ENABLED: the attribute of an enabled capability.
const seGroupEnabled = 4

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

// newJob creates a process's job object: killed when its last handle (the
// parent's) closes, at most maxProcesses processes (0: any number), no UI
// access. A process in it cannot break away.
func newJob(maxProcesses uint32) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("CreateJobObject: %w", err)
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if maxProcesses > 0 {
		limits.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
		limits.BasicLimitInformation.ActiveProcessLimit = maxProcesses
	}
	if err := setJobInfo(job, windows.JobObjectExtendedLimitInformation,
		unsafe.Pointer(&limits), unsafe.Sizeof(limits)); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("limit the job object: %w", err)
	}
	ui := windows.JOBOBJECT_BASIC_UI_RESTRICTIONS{UIRestrictionsClass: uiRestrictions}
	if err := setJobInfo(job, windows.JobObjectBasicUIRestrictions,
		unsafe.Pointer(&ui), unsafe.Sizeof(ui)); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("restrict the job object's UI: %w", err)
	}
	return job, nil
}

// Options say how Start starts a process.
type Options struct {
	// SID is the AppContainer's (Profile.SID); Capabilities are the
	// capabilities it holds besides, as SID strings (CapabilityInternetClient).
	// A nil SID starts a plain process in the job, with no AppContainer.
	SID          *windows.SID
	Capabilities []string
	// Exe is the program, Args follow it in the command line, Env is the
	// whole environment and Dir the working directory.
	Exe  string
	Args []string
	Env  []string
	Dir  string
	// MaxProcesses limits the processes of the job (0: any number). With
	// ChildProcesses false, the process may start none at all.
	MaxProcesses   uint32
	ChildProcesses bool
}

// Child is a process started in an AppContainer, with the parent's ends of its
// three standard pipes.
type Child struct {
	Stdin, Stdout, Stderr *os.File
	// Process is a handle on it for waiting; it cannot terminate an
	// AppContainer process (Kill does).
	Process *os.Process
	win     *winChild
}

// Kill terminates the process and everything in its job.
func (c *Child) Kill() error { return c.win.kill() }

// winChild is the parent's handle on a child it created: Go's own handle
// (os.FindProcess) cannot terminate it.
type winChild struct {
	mu           sync.Mutex
	process, job windows.Handle // 0 once the child has exited
}

// kill terminates the child and every process of its job.
func (c *winChild) kill() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.process == 0 {
		return os.ErrProcessDone
	}
	return windows.TerminateJobObject(c.job, 1)
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

// Start creates the process in the AppContainer, with fresh pipes for its
// standard handles (the only handles it inherits) and a job object around it.
func Start(o Options) (p *Child, err error) {
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
		return nil, err
	}
	pOut, cOut, err := pipe(false)
	if err != nil {
		return nil, err
	}
	pErr, cErr, err := pipe(false)
	if err != nil {
		return nil, err
	}

	nAttrs := uint32(2)
	if o.SID != nil {
		nAttrs++
	}
	if !o.ChildProcesses {
		nAttrs++
	}
	al, err := windows.NewProcThreadAttributeList(nAttrs)
	if err != nil {
		return nil, fmt.Errorf("NewProcThreadAttributeList: %w", err)
	}
	defer al.Delete()
	caps := securityCapabilities{AppContainerSid: o.SID}
	var held []windows.SIDAndAttributes
	for _, s := range o.Capabilities {
		sid, err := windows.StringToSid(s)
		if err != nil {
			return nil, fmt.Errorf("capability %s: %w", s, err)
		}
		held = append(held, windows.SIDAndAttributes{Sid: sid, Attributes: seGroupEnabled})
	}
	if len(held) > 0 {
		caps.Capabilities, caps.CapabilityCount = &held[0], uint32(len(held))
	}
	var pin runtime.Pinner
	if o.SID != nil {
		pin.Pin(o.SID)
	}
	for i := range held {
		pin.Pin(held[i].Sid)
	}
	if len(held) > 0 {
		pin.Pin(&held[0])
	}
	defer pin.Unpin()
	inherited := []windows.Handle{cIn, cOut, cErr}
	childPolicy := uint32(childProcessRestricted)
	policy := mitigations
	type attribute struct {
		name string
		attr uintptr
		val  unsafe.Pointer
		size uintptr
	}
	var attrs []attribute
	if o.SID != nil {
		attrs = append(attrs, attribute{"the AppContainer", procThreadAttributeSecurityCapabilities, unsafe.Pointer(&caps), unsafe.Sizeof(caps)})
	}
	attrs = append(attrs, []attribute{
		{"the handle list", windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&inherited[0]), uintptr(len(inherited)) * unsafe.Sizeof(inherited[0])},
		{"the mitigation policy", windows.PROC_THREAD_ATTRIBUTE_MITIGATION_POLICY, unsafe.Pointer(&policy), unsafe.Sizeof(policy)},
	}...)
	if !o.ChildProcesses {
		attrs = append(attrs, attribute{"the child process policy", procThreadAttributeChildProcessPolicy, unsafe.Pointer(&childPolicy), unsafe.Sizeof(childPolicy)})
	}
	for _, a := range attrs {
		if err := al.Update(a.attr, a.val, a.size); err != nil {
			return nil, fmt.Errorf("set %s: %w", a.name, err)
		}
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: al.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = cIn, cOut, cErr

	exe16, err := windows.UTF16PtrFromString(o.Exe)
	if err != nil {
		return nil, err
	}
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{o.Exe}, o.Args...)))
	if err != nil {
		return nil, err
	}
	dir16, err := windows.UTF16PtrFromString(o.Dir)
	if err != nil {
		return nil, err
	}
	block, err := envBlock(o.Env)
	if err != nil {
		return nil, err
	}
	job, err := newJob(o.MaxProcesses)
	if err != nil {
		return nil, err
	}
	// Suspended, so that it is in the job before it runs a thread; with no
	// console, because a console process starts conhost.exe, which the
	// child-process restriction refuses (the child then fails to initialise,
	// 0xc0000142). Its standard handles are the pipes.
	var pi windows.ProcessInformation
	const flags = windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_SUSPENDED | windows.CREATE_UNICODE_ENVIRONMENT | windows.DETACHED_PROCESS
	if err = windows.CreateProcess(exe16, cmdline, nil, nil, true, flags, &block[0], dir16, &si.StartupInfo, &pi); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("CreateProcess in the AppContainer: %w", err)
	}
	fail := func(err error) (*Child, error) {
		_ = windows.TerminateProcess(pi.Process, 1)
		_ = windows.CloseHandle(pi.Thread)
		_ = windows.CloseHandle(pi.Process)
		_ = windows.CloseHandle(job)
		return nil, err
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
		return nil, err
	}
	wc := &winChild{process: pi.Process, job: job}
	go wc.reap()
	return &Child{
		Stdin:   os.NewFile(uintptr(pIn), "appcontainer-stdin"),
		Stdout:  os.NewFile(uintptr(pOut), "appcontainer-stdout"),
		Stderr:  os.NewFile(uintptr(pErr), "appcontainer-stderr"),
		Process: osProc,
		win:     wc,
	}, nil
}

// Remove removes the profile profileName, the entries of its SID from the
// access lists of folders (and of everything under each one, which inherited
// them) and from those of every folder inside the trees. It finds nothing wrong
// with what is not there, so it can run again; a step that fails does not stop
// the others, and the error says each.
func Remove(profileName string, folders, trees []string) error {
	if err := procDeleteAppContainerProfile.Find(); err != nil {
		return nil // a Windows without AppContainers: Sona made none
	}
	name, err := windows.UTF16PtrFromString(profileName)
	if err != nil {
		return err
	}
	var psid *windows.SID
	if hr, _, _ := procDeriveAppContainerSid.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&psid))); hr != 0 {
		return hresult("DeriveAppContainerSidFromAppContainerName", hr)
	}
	sid, err := psid.Copy()
	_, _, _ = procFreeSid.Call(uintptr(unsafe.Pointer(psid)))
	if err != nil {
		return fmt.Errorf("copy the AppContainer SID: %w", err)
	}

	var errs []error
	for _, d := range folders {
		if err := revoke(sid, d); err != nil {
			errs = append(errs, err)
		}
	}
	for _, tree := range trees {
		// A grant may be on any folder of the tree (a version's platform
		// folder, say): every folder has its entry looked at.
		walkErr := filepath.WalkDir(tree, func(path string, d fs.DirEntry, err error) error {
			switch {
			case errors.Is(err, fs.ErrNotExist):
			case err != nil:
				errs = append(errs, err)
			case d.IsDir():
				if err := revoke(sid, path); err != nil {
					errs = append(errs, err)
				}
			}
			return nil
		})
		if walkErr != nil {
			errs = append(errs, walkErr)
		}
	}

	// The package folder's path is known only while the profile exists.
	folder, folderErr := containerFolder(sid)
	if hr, _, _ := procDeleteAppContainerProfile.Call(uintptr(unsafe.Pointer(name))); hr != 0 && uint32(hr) != hrNotFound {
		errs = append(errs, hresult("DeleteAppContainerProfile", hr))
	} else if folderErr == nil && strings.EqualFold(filepath.Base(folder), profileName) {
		// Windows deletes the folder with the profile; make sure of it
		// (and of nothing else: only a folder named for the profile).
		if err := os.RemoveAll(folder); err != nil {
			errs = append(errs, fmt.Errorf("remove the AppContainer's folder: %w", err))
		}
	}
	return errors.Join(errs...)
}

// revoke removes the explicit entries for sid from path's access list
// (Grant adds them), which takes away the entries a directory's
// objects inherited from them. A path that is gone, has no list or has no
// entry for sid is left alone.
func revoke(sid *windows.SID, path string) error {
	if filepath.Dir(path) == path {
		return fmt.Errorf("refusing to change the access list of %s, a drive root", path)
	}
	grantMu.Lock()
	defer grantMu.Unlock()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the access list of %s: %w", path, err)
	}
	if sd == nil {
		return nil
	}
	old, _, err := sd.DACL()
	if errors.Is(err, windows.ERROR_OBJECT_NOT_FOUND) || (err == nil && old == nil) {
		return nil // no list: everyone has full access
	}
	if err != nil {
		return fmt.Errorf("read the access list of %s: %w", path, err)
	}
	if !aclHasEntry(old, sid, true) {
		runtime.KeepAlive(sd)
		return nil
	}
	var pin runtime.Pinner
	pin.Pin(sid)
	defer pin.Unpin()
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessMode:  windows.REVOKE_ACCESS,
		Inheritance: windows.NO_INHERITANCE,
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
		return fmt.Errorf("remove the sandbox's access to %s: %w", path, err)
	}
	return nil
}

// aclHasEntry reports whether acl has an allow or deny entry for sid;
// explicit: only one not inherited from a parent.
func aclHasEntry(acl *windows.ACL, sid *windows.SID, explicit bool) bool {
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil {
			continue
		}
		if t := ace.Header.AceType; t != windows.ACCESS_ALLOWED_ACE_TYPE && t != windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if explicit && ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			continue
		}
		if (*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(sid) {
			return true
		}
	}
	return false
}
