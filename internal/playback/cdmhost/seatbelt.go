//go:build linux || darwin

package cdmhost

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// The macOS sandbox (sandbox_darwin.go) runs the child under sandbox-exec
// with seatbeltProfile. Everything here is plain Go, so Linux runs its
// tests too.

// seatbeltPaths are the paths seatbeltProfile names, resolved to real
// paths (Seatbelt matches the path after symlinks: /tmp is /private/tmp).
type seatbeltPaths struct {
	// Hidden can be neither read nor written: Sona's own folder (settings,
	// the credentials fallback, the database, the log) and the credential
	// stores (the keychains, ~/.ssh, ~/.gnupg, ~/.password-store).
	Hidden []string
	// Writable are writable besides /dev and the temporary folders:
	// GStreamer's registry cache.
	Writable []string
	// Readable are read-only even inside Hidden: the folder of the CDM
	// library this child loads (with its .sig), which lives in Sona's own
	// folder (<data>/widevine/<version>/<platform>).
	Readable []string
}

// seatbeltKeychainServices are the Mach services behind the keychain. The
// child reaches the keychain through them even with its files hidden, and
// it is the same executable as Sona, which the keychain item trusts.
var seatbeltKeychainServices = []string{
	"com.apple.SecurityServer",
	"com.apple.securityd",
	"com.apple.securityd.xpc",
	"com.apple.security.agent",
	"com.apple.security.authhost",
}

// seatbeltProfile is the Seatbelt (SBPL) profile of the CDM host child on
// macOS. It allows what an ordinary process may do (dyld, the CDM and
// GStreamer's libraries and plugins, Core Audio's Mach services, the pipes
// to the parent), then takes away what the child never needs:
//
//   - the network: no TCP, UDP or Unix-socket connection, bind or
//     listen (network*; the child gets everything from the parent);
//   - file writes outside /dev, the temporary folders and p.Writable;
//   - reading or writing p.Hidden, except reading p.Readable (and the
//     metadata of the folders above it, to reach it);
//   - the keychain's Mach services (seatbeltKeychainServices).
//
// Later rules win over earlier ones, so each exception follows its rule.
func seatbeltProfile(p seatbeltPaths) string {
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("; Sona's CDM host child (internal/playback/cdmhost/seatbelt.go).\n")
	b.WriteString("(allow default)\n")
	b.WriteString("(deny network*)\n")
	b.WriteString("(deny file-write*)\n")
	b.WriteString("(allow file-write*\n")
	for _, d := range []string{"/dev", "/private/tmp", "/private/var/tmp", "/private/var/folders"} {
		fmt.Fprintf(&b, "  (subpath %s)\n", sbplString(d))
	}
	for _, d := range p.Writable {
		fmt.Fprintf(&b, "  (subpath %s)\n", sbplString(d))
	}
	b.WriteString(")\n")
	if len(p.Hidden) > 0 {
		b.WriteString("(deny file-read* file-write*\n")
		for _, d := range p.Hidden {
			fmt.Fprintf(&b, "  (subpath %s)\n", sbplString(d))
		}
		b.WriteString(")\n")
	}
	if len(p.Readable) > 0 {
		b.WriteString("(allow file-read*\n")
		for _, d := range p.Readable {
			fmt.Fprintf(&b, "  (subpath %s)\n", sbplString(d))
		}
		b.WriteString(")\n(allow file-read-metadata\n")
		seen := map[string]bool{}
		for _, d := range p.Readable {
			for a := filepath.Dir(d); !seen[a]; a = filepath.Dir(a) {
				seen[a] = true
				fmt.Fprintf(&b, "  (literal %s)\n", sbplString(a))
				if a == "/" || a == "." {
					break
				}
			}
		}
		b.WriteString(")\n")
	}
	b.WriteString("(deny mach-lookup\n")
	for _, s := range seatbeltKeychainServices {
		fmt.Fprintf(&b, "  (global-name %s)\n", sbplString(s))
	}
	b.WriteString(")\n")
	return b.String()
}

// sbplString quotes s as an SBPL string literal (Scheme syntax: backslash
// escapes; strconv.Quote's escapes are a subset that SBPL reads alike).
func sbplString(s string) string { return strconv.Quote(s) }

// seatbeltChoice decides whether the child runs under sandbox-exec: not
// when the developer turned the sandbox off (off), when sandbox-exec is
// missing (lookErr) or when it refuses the profile (probe, run only if the
// others pass). reason says why it doesn't.
func seatbeltChoice(off bool, lookErr error, probe func() error) (use bool, reason string) {
	switch {
	case off:
		return false, "the macOS sandbox is turned off (" + EnvSeatbelt + "=0)"
	case lookErr != nil:
		return false, "no macOS sandbox: " + lookErr.Error()
	}
	if err := probe(); err != nil {
		return false, "no macOS sandbox: sandbox-exec refused Sona's profile: " + err.Error()
	}
	return true, ""
}

// EnvSeatbelt=0 runs the macOS CDM host child without its sandbox, for
// telling a sandbox problem apart from a playback one during development.
// Diagnostics then says the child is not sandboxed. Linux ignores it.
const EnvSeatbelt = "SONA_SEATBELT"

// inetRefused checks that the child cannot reach the network: a UDP
// socket's connect() to the loopback discard port must fail with EPERM or
// EACCES. It sends nothing: a UDP connect only sets the peer address, which
// is where Seatbelt's network rule applies.
func inetRefused() error {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) {
			return nil
		}
		return fmt.Errorf("socket(AF_INET): %w", err)
	}
	defer func() { _ = unix.Close(fd) }()
	err = unix.Connect(fd, &unix.SockaddrInet4{Port: 9, Addr: [4]byte{127, 0, 0, 1}})
	switch {
	case err == nil:
		return errors.New("connect(udp 127.0.0.1:9) succeeded")
	case errors.Is(err, unix.EPERM), errors.Is(err, unix.EACCES):
		return nil
	}
	return fmt.Errorf("connect(udp 127.0.0.1:9): %w", err)
}
