package addons

import (
	"regexp"
	"slices"
	"testing"
)

// Every bundled add-on has a valid embedded manifest under its reserved ID,
// and every embedded manifest is registered: a file nobody lists would be
// code nobody reviewed the ID of.
func TestBundledManifestsAreValidAndRegistered(t *testing.T) {
	files, err := bundledFS.ReadDir("bundled")
	if err != nil {
		t.Fatal(err)
	}
	var onDisk []string
	for _, f := range files {
		onDisk = append(onDisk, f.Name())
	}
	var want []string
	for _, id := range BundledIDs {
		want = append(want, id+".json")
		m, err := BundledManifest(id)
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		if m.ID != id || !Reserved(id) {
			t.Errorf("%s: id %q, reserved %v", id, m.ID, Reserved(id))
		}
	}
	slices.Sort(onDisk)
	slices.Sort(want)
	if !slices.Equal(onDisk, want) {
		t.Errorf("embedded manifests %v, registered %v", onDisk, want)
	}
	// The reserved IDs of the folder scanner are the bundled ones (plus the
	// sona prefix).
	for _, id := range reservedIDs {
		if !IsBundled(id) {
			t.Errorf("%q is reserved but not bundled", id)
		}
	}
}

func TestBundledEntries(t *testing.T) {
	es := BundledEntries("/opt/sona/sona")
	if len(es) != len(BundledIDs) {
		t.Fatalf("%d entries, want %d", len(es), len(BundledIDs))
	}
	for _, e := range es {
		if e.Problem != nil || !e.Bundled || e.Exe != "/opt/sona/sona" || !slices.Equal(e.Args, []string{"--addon", e.ID}) || e.Dir != e.ID {
			t.Errorf("%s = %+v", e.ID, e)
		}
	}
	// Sona that can't tell where it runs from runs no bundled add-on.
	for _, e := range BundledEntries("") {
		if e.Problem == nil || e.Problem.Code != ProblemNoExecutable || e.Exe != "" {
			t.Errorf("%s with no executable: %+v", e.ID, e)
		}
	}
}

// The secrets a bundled add-on may read are keys the secret method accepts.
func TestBundledSecretKeys(t *testing.T) {
	key := regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	for id, keys := range bundledSecrets {
		if !IsBundled(id) {
			t.Errorf("secrets of %q, which is not bundled", id)
		}
		for _, k := range keys {
			if !key.MatchString(k) || !BundledSecret(id, k) {
				t.Errorf("%s: key %q", id, k)
			}
		}
	}
	if BundledSecret("lastfm", "token") || BundledSecret("other", "session") {
		t.Error("a bundled add-on may read another's secret")
	}
}
