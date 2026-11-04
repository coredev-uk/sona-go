package search

import (
	"fmt"
	"os"
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// TestMain registers the interface font once, so the golden images are
// drawn in Schibsted Grotesk whatever test ran first.
func TestMain(m *testing.M) {
	if err := theme.RegisterFonts(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
