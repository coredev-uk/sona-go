package artifacts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirUsesTheVariableAndKeepsSubtestsApart(t *testing.T) {
	base := t.TempDir()
	t.Setenv(Env, base)
	t.Run("a/b", func(t *testing.T) {
		got := Dir(t)
		want := filepath.Join(base, "TestDirUsesTheVariableAndKeepsSubtestsApart_a_b")
		if got != want {
			t.Fatalf("Dir = %q, want %q", got, want)
		}
		if st, err := os.Stat(got); err != nil || !st.IsDir() {
			t.Fatalf("directory not created: %v", err)
		}
	})
}
