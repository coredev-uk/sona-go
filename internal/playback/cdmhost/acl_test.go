//go:build linux || (cgo && (darwin || windows))

package cdmhost

import "testing"

const (
	sandboxSID = "S-1-15-2-1111-2222-3333-4444-5555-6666-7777"
	allAppPkgs = "S-1-15-2-1"
	userSID    = "S-1-5-21-1-2-3-1001"
	read       = uint32(0x1200a9) // FILE_GENERIC_READ | FILE_GENERIC_EXECUTE
	acePropped = 0x10             // INHERITED_ACE
	both       = aceObjectInherit | aceContainerInherit
)

var who = []string{sandboxSID}

func TestAcesAllow(t *testing.T) {
	for _, c := range []struct {
		name    string
		entries []aclEntry
		who     []string
		mask    uint32
		dir     bool
		want    bool
	}{
		{"a folder's entry that files and folders inherit", []aclEntry{{sandboxSID, read, both}}, who, read, true, true},
		{"an entry of the folder alone is not for what is under it", []aclEntry{{sandboxSID, read, 0}}, who, read, true, false},
		{"but is enough for a file", []aclEntry{{sandboxSID, read, 0}}, who, read, false, true},
		{"files only", []aclEntry{{sandboxSID, read, aceObjectInherit}}, who, read, true, false},
		{"inherit-only entries are for the children", []aclEntry{{sandboxSID, read, both | aceInheritOnly}}, who, read, true, false},
		{"less than the rights asked for", []aclEntry{{sandboxSID, read &^ 0x20, both}}, who, read, true, false},
		{"someone else's entry", []aclEntry{{userSID, read, both}}, who, read, true, false},
		{"ALL APPLICATION PACKAGES counts when asked", []aclEntry{{allAppPkgs, read, both}}, []string{sandboxSID, allAppPkgs}, read, true, true},
		{"and not when it isn't", []aclEntry{{allAppPkgs, read, both}}, who, read, true, false},
		{"a file's inherited entry", []aclEntry{{sandboxSID, read, acePropped}}, who, read, false, true},
		{"no entries", nil, who, read, true, false},
	} {
		if got := acesAllow(c.entries, c.who, c.mask, c.dir); got != c.want {
			t.Errorf("%s: acesAllow = %v, want %v", c.name, got, c.want)
		}
	}
}

// grantNeeded follows an install through its life: the first grant, a start
// after it, an update that renames new files in, and the start that covers them.
func TestGrantNeededAcrossAnUpdate(t *testing.T) {
	folder := []aclEntry{{userSID, 0x1f01ff, both | acePropped}}
	var exe []aclEntry // the program's list: the user's entry, inherited
	exe = append(exe, aclEntry{userSID, 0x1f01ff, acePropped})

	// A new install: nothing for the sandbox yet.
	if !grantNeeded(folder, [][]aclEntry{exe}, who, read) {
		t.Fatal("a folder without an entry for the sandbox needs the grant")
	}

	// The grant: an entry on the folder, which the files inherit.
	folder = append(folder, aclEntry{sandboxSID, read, both})
	exe = append(exe, aclEntry{sandboxSID, read, acePropped})
	if grantNeeded(folder, [][]aclEntry{exe}, who, read) {
		t.Error("a start after the grant sets the list again")
	}

	// The update: the folder keeps its entry, the program that was renamed
	// in has only what the folder it was unpacked in gave it.
	swapped := []aclEntry{{userSID, 0x1f01ff, acePropped}}
	if !grantNeeded(folder, [][]aclEntry{swapped}, who, read) {
		t.Fatal("files renamed in by an update are not covered and the grant is not set again")
	}

	// The start that did: the list set on the folder reached the files.
	swapped = append(swapped, aclEntry{sandboxSID, read, acePropped})
	if grantNeeded(folder, [][]aclEntry{swapped}, who, read) {
		t.Error("covered files need the grant again")
	}
}

func TestGrantNeededEdges(t *testing.T) {
	folder := []aclEntry{{sandboxSID, read, both}}
	if grantNeeded(folder, nil, who, read) {
		t.Error("nothing to check but the folder, which is covered")
	}
	// A program that can't be read, or has no list (everyone has access), is
	// not passed: the folder decides.
	if grantNeeded(folder, [][]aclEntry{}, who, read) {
		t.Error("an empty list of sentinels asks for the grant")
	}
	// Program Files: ALL APPLICATION PACKAGES reads, so files made there are
	// covered without an entry of the sandbox's own.
	pf := []aclEntry{{allAppPkgs, read, both}}
	file := []aclEntry{{allAppPkgs, read, acePropped}}
	if grantNeeded(pf, [][]aclEntry{file}, []string{sandboxSID, allAppPkgs}, read) {
		t.Error("an install that ALL APPLICATION PACKAGES reads needs the sandbox's own entry")
	}
	// One uncovered file among several is enough.
	covered := []aclEntry{{sandboxSID, read, acePropped}}
	if !grantNeeded(folder, [][]aclEntry{covered, {{userSID, read, acePropped}}}, who, read) {
		t.Error("one file the sandbox can't read was not asked for")
	}
}
