package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// queueRow is Settings → Playback's Replacing your queue: what happens
// when playing something would replace a queue you built.
func (s *settings) queueRow(c *ui.Context, st config.Settings) {
	items := []kit.SelectItem{
		kit.SelectOption(config.ReplaceUndo, i18n.T("Replace, offer Undo")),
		kit.SelectOption(config.ReplaceConfirm, i18n.T("Ask first")),
	}
	s.selectRow(c, rowOpts{
		ID:    "replace-queue",
		Title: i18n.T("Replacing your queue"),
		Desc:  i18n.T("When you play an album, playlist or station while more than three songs you added are waiting in the queue."),
	}, st.Queue.ReplaceConfirm, items, false, func(v string) {
		s.save(func(x *config.Settings) { x.Queue.ReplaceConfirm = v })
	})
}
