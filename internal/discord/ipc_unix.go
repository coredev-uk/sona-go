//go:build unix

package discord

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// Supported: Discord listens on a Unix socket here, which the client dials.
const Supported = true

// socketPaths lists the candidate IPC sockets: discord-ipc-0…9 in the
// runtime directory, and in the Flatpak and Snap sandboxes' runtime
// directories under it. The shared temp directories are probed only when
// XDG_RUNTIME_DIR is unset, since anyone can create a discord-ipc-0 there.
func socketPaths() []string {
	dirs := []string{os.Getenv("XDG_RUNTIME_DIR")}
	if dirs[0] == "" {
		dirs = nil
		for _, env := range []string{"TMPDIR", "TMP", "TEMP"} {
			if d := os.Getenv(env); d != "" {
				dirs = append(dirs, d)
			}
		}
		dirs = append(dirs, "/tmp")
	}
	return findSockets(dirs, os.Getuid())
}

// findSockets lists the discord-ipc-N sockets under dirs that belong to
// uid (see trusted).
func findSockets(dirs []string, uid int) []string {
	subdirs := []string{
		"",
		"app/com.discordapp.Discord",
		"app/com.discordapp.DiscordCanary",
		"app/dev.vencord.Vesktop",
		".flatpak/dev.vencord.Vesktop/xdg-run",
		"snap.discord",
		"snap.discord-canary",
	}
	var out []string
	seen := map[string]bool{}
	for _, d := range dirs {
		for _, sub := range subdirs {
			base := filepath.Join(d, sub)
			if seen[base] {
				continue
			}
			seen[base] = true
			for i := range 10 {
				p := filepath.Join(base, "discord-ipc-"+strconv.Itoa(i))
				if trusted(p, uid) {
					out = append(out, p)
				}
			}
		}
	}
	return out
}

// trusted reports whether p is a socket (not a symlink) owned by uid in a
// directory no other user can replace it in: one owned by uid, or a
// root-owned one that is not group/world-writable or is sticky (/tmp).
// Without this, another local user could plant a socket that answers
// READY and collect the now-playing track.
func trusted(p string, uid int) bool {
	fi, err := os.Lstat(p)
	if err != nil || fi.Mode().Type() != fs.ModeSocket || owner(fi) != uid {
		return false
	}
	di, err := os.Lstat(filepath.Dir(p))
	if err != nil || !di.IsDir() {
		return false
	}
	switch owner(di) {
	case uid:
		return true
	case 0:
		return di.Mode().Perm()&0o022 == 0 || di.Mode()&fs.ModeSticky != 0
	}
	return false
}

// owner is fi's owning uid, -1 if unknown.
func owner(fi fs.FileInfo) int {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid)
	}
	return -1
}
