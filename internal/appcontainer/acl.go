package appcontainer

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

// A grantAction is what a folder's read grant needs before a child starts.
type grantAction int

const (
	// grantNone: the folder's entry and the files the child needs are covered.
	grantNone grantAction = iota
	// grantFolder: the folder has no entry that its contents inherit; add it.
	grantFolder
	// grantInherit: the folder has the entry and a file below it does not;
	// hand the folder's entry down again.
	grantInherit
)

// grantNeeded decides what the sandbox's read grant on a folder needs: the
// entry on the folder itself when it has none that its contents inherit, and
// else, when one of the files the child needs (sentinels, each one's entries;
// files whose list is unreadable or empty are left out) lacks one, that the
// folder's entry is inherited again.
//
// The second case is an update. MyGo unpacks the new version in a folder
// beside the install and renames its entries in; a rename on the same volume
// keeps the access list the entry was made with, which came from that other
// folder, so the folder's own entry is still there and the files in it
// are not covered. Setting the folder's list again does not reach them
// (observed on windows-2025: the entry stayed missing, also after the entry
// was removed and added); resetting the inherited entries of the tree does
// (TreeSetNamedSecurityInfo, TREE_SEC_INFO_RESET_KEEP_EXPLICIT).
func grantNeeded(folder []aclEntry, sentinels [][]aclEntry, who []string, mask uint32) grantAction {
	if !acesAllow(folder, who, mask, true) {
		return grantFolder
	}
	for _, s := range sentinels {
		if !acesAllow(s, who, mask, false) {
			return grantInherit
		}
	}
	return grantNone
}
