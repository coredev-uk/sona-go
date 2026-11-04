//go:build !dev

package devinstance

import "testing"

func TestProductionBuildIgnoresTheVariable(t *testing.T) {
	t.Setenv(Env, "feat")
	if Name() != "" || Suffix() != "" {
		t.Fatalf("a production build must not read %s: %q %q", Env, Name(), Suffix())
	}
}
