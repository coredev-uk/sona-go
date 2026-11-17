package sandbox

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The macOS sandbox (darwin.go) runs the add-on under sandbox-exec with the
// profile seatbeltProfile writes, in the style of the playback host's
// (internal/playback/cdmhost/seatbelt.go). The profile is plain text built
// in plain Go, so Linux runs its tests too.

// seatbeltInput is what a profile is made of, with every path resolved
// (Seatbelt matches the path after symlinks: /tmp is /private/tmp).
type seatbeltInput struct {
	Policy
	// Hidden are the places an add-on can neither read nor write unless the
	// policy names a place inside them: the user's home and everything the
	// user keeps on this Mac outside it.
	Hidden []string
}

// seatbeltKeychainServices are the Mach services behind the keychain. An
// add-on reaches the keychain through them even with its files hidden; a
// bundled one is Sona's own program, which the keychain item trusts.
var seatbeltKeychainServices = []string{
	"com.apple.SecurityServer",
	"com.apple.securityd",
	"com.apple.securityd.xpc",
	"com.apple.security.agent",
	"com.apple.security.authhost",
}

// seatbeltProfile is the Seatbelt (SBPL) profile of an add-on on macOS. It
// allows what an ordinary process may do (dyld, the system's libraries and
// frameworks, the pipes to Sona), then takes away what an add-on has no
// business with:
//
//   - the network, unless the policy has it: no TCP, UDP or Unix-socket
//     connection, bind or listen, but the sockets the policy names;
//   - every file write but the data folder and /dev/null (and its kin);
//   - reading or writing the user's home and the rest of the user's data
//     (Hidden), except what is inside the add-on's own folder, the data
//     folder and the policy's other read-only folders (and the metadata of
//     the folders above them, to reach them);
//   - the terminals: tty, ttys, ptys and the console;
//   - the keychain's Mach services.
//
// Later rules win over earlier ones, so each exception follows its rule.
// This is a deny-list over `(allow default)`, as the playback host's: what it
// doesn't name stays reachable (docs/security-review.md, "Add-ons").
func seatbeltProfile(in seatbeltInput) string {
	var b strings.Builder
	list := func(op string, paths []string, form string) {
		if len(paths) == 0 {
			return
		}
		fmt.Fprintf(&b, "(%s\n", op)
		for _, p := range paths {
			fmt.Fprintf(&b, "  (%s %s)\n", form, sbplString(p))
		}
		b.WriteString(")\n")
	}
	b.WriteString("(version 1)\n")
	b.WriteString("; An add-on of Sona's (internal/addons/sandbox/seatbelt.go).\n")
	b.WriteString("(allow default)\n")
	if !in.Network {
		b.WriteString("(deny network*)\n")
		list("allow network-outbound", in.Sockets, "literal")
	}
	b.WriteString("(deny file-write*)\n")
	list("allow file-write*", []string{"/dev/null", "/dev/zero", "/dev/dtracehelper"}, "literal")
	list("deny file-read* file-write*", in.Hidden, "subpath")
	readable := append(slices.Clone(in.Read), in.Dir, in.Data)
	list("allow file-read*", readable, "subpath")
	list("allow file-write*", []string{in.Data}, "subpath")
	var ancestors []string
	seen := map[string]bool{}
	for _, d := range readable {
		for a := filepath.Dir(d); !seen[a]; a = filepath.Dir(a) {
			seen[a] = true
			ancestors = append(ancestors, a)
			if a == "/" || a == "." {
				break
			}
		}
	}
	list("allow file-read-metadata", ancestors, "literal")
	// No terminal, to read or to push keystrokes into with TIOCSTI.
	b.WriteString("(deny file-read* file-write* file-ioctl\n  (regex #\"^/dev/(tty|ttys[0-9]+|pty[a-z0-9]+|console)$\"))\n")
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
