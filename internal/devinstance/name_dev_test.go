//go:build dev

package devinstance

import "testing"

func TestDevBuildUsesTheVariable(t *testing.T) {
	t.Setenv(Env, "")
	if Name() != "" || Suffix() != "" {
		t.Fatalf("unset: %q %q", Name(), Suffix())
	}
	t.Setenv(Env, "feat/x")
	if Name() != "feat_x" || Suffix() != ".instance_feat_x" {
		t.Fatalf("set: %q %q", Name(), Suffix())
	}
}
