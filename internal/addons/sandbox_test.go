package addons

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDataMakesAPrivateFolderAndLeavesWhatIsThere(t *testing.T) {
	data := filepath.Join(t.TempDir(), "state", "addons", "demo", "data")
	if err := EnsureData(data); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{data, filepath.Join(data, TempName)} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
			t.Errorf("%s: %v, %v", d, fi, err)
		}
	}
	if err := os.WriteFile(filepath.Join(data, "state.json"), []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureData(data); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(data, "state.json")); err != nil || string(b) != "kept" {
		t.Errorf("a second EnsureData touched the data: %q, %v", b, err)
	}
}

// The add-on fills its data folder, so it may replace the temporary folder with
// a link: Sona removes the link, never follows it.
func TestEnsureDataReplacesALinkedTempFolderWithoutFollowingIt(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(data, TempName)); err != nil {
		t.Skip(err)
	}
	if err := EnsureData(data); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(filepath.Join(data, TempName)); err != nil || !fi.IsDir() {
		t.Errorf("the temporary folder is not a folder again: %v, %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Errorf("EnsureData followed the link: %v", err)
	}
}

func TestEnsureDataRefusesALinkInPlaceOfTheFolder(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "elsewhere"), 0o700); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "data")
	if err := os.Symlink(filepath.Join(root, "elsewhere"), data); err != nil {
		t.Skip(err)
	}
	if err := EnsureData(data); err == nil {
		t.Error("a link in place of the data folder was accepted")
	}
}

func TestEverySandboxReasonHasWords(t *testing.T) {
	fallback := SandboxText("zzz")
	for _, r := range []string{
		SandboxUserNamespaceRestricted, SandboxNoUserNamespace, SandboxNoSeccomp, SandboxNoSeatbelt, SandboxNoAppContainer, SandboxUnsupported,
	} {
		if SandboxText(r) == fallback {
			t.Errorf("reason %q has no words", r)
		}
	}
}
