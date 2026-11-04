package song

import (
	"os"
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// TestMain registers the interface font, so goldens render as in CI.
func TestMain(m *testing.M) {
	if err := theme.RegisterFonts(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
