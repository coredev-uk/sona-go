package discord

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"golang.org/x/sys/windows"
)

// Supported: Discord listens on a named pipe here, which the client dials.
const Supported = true

// socketPaths lists the candidate pipes, \\.\pipe\discord-ipc-0…9. Pipe
// names are global to the computer, so dial checks whose each one is.
func socketPaths() []string {
	out := make([]string, 0, 10)
	for i := range 10 {
		out = append(out, `\\.\pipe\discord-ipc-`+strconv.Itoa(i))
	}
	return out
}

// trustedOnly: a pipe is checked when it is dialled (dial), whose server it
// is, so every path stays.
func trustedOnly(paths []string) []string { return paths }

// errNotOwnPipe is a pipe whose server runs as another user (or whose user
// can't be read): anyone can create a pipe of that name first, so it gets
// nothing from Sona, as F4 does for Unix sockets.
var errNotOwnPipe = errors.New("discord: the pipe's server is not this user's")

// dial opens the pipe at p and keeps it only when its server process runs
// as the current user.
func dial(ctx context.Context, p string) (ipcConn, error) {
	return dialPipe(ctx, p, processUser)
}

// dialPipe is dial with the lookup of a process's user, which the tests
// replace.
//
// The pipe is opened for overlapped I/O, so os.NewFile ties it to the
// runtime's completion port and its deadlines work. SECURITY_IDENTIFICATION
// lets the server identify the client but never impersonate it, whoever the
// server turns out to be. Nothing is written before the check.
func dialPipe(ctx context.Context, p string, userOf func(pid uint32) (*windows.SID, error)) (ipcConn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OVERLAPPED|windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION, 0)
	if err != nil {
		return nil, fmt.Errorf("discord: open %s: %w", p, err)
	}
	if err := ownServer(h, userOf); err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	return os.NewFile(uintptr(h), p), nil
}

// ownServer reports errNotOwnPipe unless the server end of the pipe h is a
// process of the current user.
func ownServer(h windows.Handle, userOf func(pid uint32) (*windows.SID, error)) error {
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(h, &pid); err != nil {
		return fmt.Errorf("%w: %w", errNotOwnPipe, err)
	}
	server, err := userOf(pid)
	if err != nil {
		return fmt.Errorf("%w: %w", errNotOwnPipe, err)
	}
	self, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("%w: %w", errNotOwnPipe, err)
	}
	if !windows.EqualSid(server, self.User.Sid) {
		return errNotOwnPipe
	}
	return nil
}

// processUser is the user the process pid runs as. A process of another
// user that this one may not query fails, which ownServer counts as not the
// user's.
func processUser(pid uint32) (*windows.SID, error) {
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(p) }()
	var tok windows.Token
	if err := windows.OpenProcessToken(p, windows.TOKEN_QUERY, &tok); err != nil {
		return nil, err
	}
	defer func() { _ = tok.Close() }()
	u, err := tok.GetTokenUser()
	if err != nil {
		return nil, err
	}
	// The SID lives in the token's buffer, which goes with tok.
	return u.User.Sid.Copy()
}
