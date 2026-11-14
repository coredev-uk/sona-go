//go:build linux || (cgo && (darwin || windows))

package cdmhost

// The decisions behind the Windows sandbox's access grants
// (appcontainer_windows.go), on plain values so that Linux tests can run them:
// the Windows code reads an access list into []aclEntry and acts on the answer.

// An aclEntry is the part of an ACCESS_ALLOWED_ACE a grant is judged by.
type aclEntry struct {
	sid   string // the trustee, as SID.String()
	mask  uint32
	flags uint8 // the ACE header's AceFlags
}

// The AceFlags (winnt.h) the decisions read.
const (
	aceObjectInherit    = 0x01
	aceContainerInherit = 0x02
	aceInheritOnly      = 0x08
)

// acesAllow reports whether entries has an entry for one of who that allows
// all of mask on the object itself and, for a directory, on what is under it
// (the entry is inherited by both files and folders). An entry that only
// applies to children does not count for the object.
func acesAllow(entries []aclEntry, who []string, mask uint32, dir bool) bool {
	const both = aceObjectInherit | aceContainerInherit
	for _, e := range entries {
		if e.flags&aceInheritOnly != 0 || e.mask&mask != mask {
			continue
		}
		if dir && e.flags&both != both {
			continue
		}
		for _, w := range who {
			if e.sid == w {
				return true
			}
		}
	}
	return false
}

// grantNeeded reports whether the sandbox's read grant on a folder must be
// (re)applied before a child starts: the folder has no entry that its
// contents inherit, or one of the files the child needs (sentinels, each
// one's entries; files whose list is unreadable or empty are left out) lacks
// one.
//
// The second case is an update. MyGo unpacks the new version in a folder
// beside the install and renames its entries in; a rename on the same volume
// keeps the access list the entry was made with, which came from that other
// folder, so the folder's own entry is still there and the files in it
// are not covered. Setting the folder's list again hands its entry down to
// every file below it.
func grantNeeded(folder []aclEntry, sentinels [][]aclEntry, who []string, mask uint32) bool {
	if !acesAllow(folder, who, mask, true) {
		return true
	}
	for _, s := range sentinels {
		if !acesAllow(s, who, mask, false) {
			return true
		}
	}
	return false
}
