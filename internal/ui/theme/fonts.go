package theme

import (
	"embed"
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"
)

// Schibsted Grotesk, the interface font: variable TTFs (upright and the
// italic that sets a duet's second voice), SIL OFL 1.1, from Google Fonts
// (fonts/OFL.txt, fonts/README.md).
//
//go:embed fonts/SchibstedGrotesk-Variable.ttf fonts/SchibstedGrotesk-Italic-Variable.ttf
var fontFiles embed.FS

var fontsOnce struct {
	sync.Once
	err error
}

// RegisterFonts makes the embedded Schibsted Grotesk usable as FontFamily.
// Call it once before the first window opens; later calls return the first
// call's result.
func RegisterFonts() error {
	fontsOnce.Do(func() {
		for _, name := range []string{"SchibstedGrotesk-Variable.ttf", "SchibstedGrotesk-Italic-Variable.ttf"} {
			data, err := fontFiles.ReadFile("fonts/" + name)
			if err != nil {
				fontsOnce.err = err
				return
			}
			if err := ui.RegisterFont(data, FontFamily); err != nil {
				fontsOnce.err = fmt.Errorf("theme: font %s: %w", name, err)
				return
			}
		}
	})
	return fontsOnce.err
}
