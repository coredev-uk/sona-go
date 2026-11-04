package settings

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// qualityRow is Settings → Playback's Audio quality: AAC 256 or Data saver.
func (s *settings) qualityRow(c *ui.Context, st config.Settings) {
	items := []kit.SelectItem{
		kit.SelectOption("256", i18n.T("High · 256 kbps")),
		kit.SelectOption("64", i18n.T("Data saver · 64 kbps")),
	}
	s.selectRow(c, rowOpts{
		ID:    "quality",
		Title: i18n.T("Audio quality"),
		Desc:  i18n.T("AAC at 256 kbps, or 64 kbps with Data saver for slow or metered connections. Applies from the next song. Lossless and Dolby Atmos albums play as AAC too."),
	}, strconv.Itoa(st.AudioQuality), items, false, func(v string) {
		n, err := strconv.Atoi(v)
		if err != nil {
			return
		}
		s.save(func(x *config.Settings) { x.AudioQuality = n })
	})
}
