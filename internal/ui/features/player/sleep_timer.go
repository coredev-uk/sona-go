package player

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// sleepUI is the sleep timer's state (v0.2.0's SleepTimer.tsx).
type sleepUI struct {
	menu kit.Menu
	// chosen is the minutes the user picked this session ("15"…): the radio
	// that shows while a timer they set runs.
	chosen string
}

// sleepChoice is a row of the timer's menu: minutes, or the end of the
// song or of the queue.
type sleepChoice struct {
	value string // "15", "track", "queue"
	label string
}

// sleepChoices are the timer choices, in the menu's order.
func sleepChoices() []sleepChoice {
	return []sleepChoice{
		{"15", i18n.T("In 15 minutes")},
		{"30", i18n.T("In 30 minutes")},
		{"45", i18n.T("In 45 minutes")},
		{"60", i18n.T("In 1 hour")},
		{"120", i18n.T("In 2 hours")},
		{"track", i18n.T("At the end of this song")},
		{"queue", i18n.T("At the end of the queue")},
	}
}

// minutesLeft is the minutes (rounded up) until endsAt (Unix ms).
func minutesLeft(endsAt int64, now time.Time) int {
	left := endsAt - now.UnixMilli()
	return int(max((left+59_999)/60_000, 0))
}

// setSleep sets the sleep timer: mode "after" (in minutes), "track" or
// "queue"; "" turns it off (v0.2.0's setSleep).
func (p *player) setSleep(mode string, minutes int) {
	p.run(func() string { return i18n.T("Couldn't set the sleep timer") }, func(context.Context) error {
		return p.svc.SetSleep(mode, minutes)
	})
}

// sleepButton is the moon in the bar and its menu: pauses playback after a
// while, or at the end of the song or the queue, fading out over the last
// 10 seconds. While it runs the moon is solid, and shows the minutes left.
func (p *player) sleepButton(c *ui.Context) {
	s := p.env.Play.State()
	sleep := s.Sleep
	now := p.now()
	after := sleep != nil && sleep.Mode == "after"
	minutes := 0
	until := ""
	if after {
		minutes = minutesLeft(sleep.EndsAt, now)
		until = i18n.FormatClock(time.UnixMilli(sleep.EndsAt))
		if left := sleep.EndsAt - now.UnixMilli(); left > 0 {
			// Tick on the minute boundary of the countdown, not every second.
			c.After(time.Duration(left%60_000+50) * time.Millisecond)
		}
	}
	status := i18n.T("Sleep timer")
	switch {
	case sleep == nil:
	case after:
		status = i18n.T("Sleep timer: {minutes}m left, pauses at {until}", "minutes", minutes, "until", until)
	case sleep.Mode == "track":
		status = i18n.T("Sleep timer: pauses after this song")
	default:
		status = i18n.T("Sleep timer: pauses after the queue")
	}
	text := ""
	if after {
		text = i18n.T("{minutes}m", "minutes", minutes)
	}
	btn := kit.Button(c.Key("sleep"), text, kit.ButtonOpts{
		Variant: kit.Ghost, Size: kit.SizeSM, Icon: "moon", Filled: sleep != nil, Muted: sleep == nil,
		Disabled: s.Current == nil && sleep == nil, A11yLabel: status, Tip: status,
	})
	btn.Role(ui.RoleMenuButton).HasPopup(ui.PopupMenu).Expanded(p.sleep.menu.IsOpen()) // it opens a menu
	if btn.Clicked() {
		p.sleep.menu.OpenFrom(btn.FocusVisible())
	}
	p.sleep.menu.Show(c, kit.MenuOpts{ID: "sleep-menu", Label: i18n.T("Stop playing"), Width: 240}, btn, p.sleepItems(sleep, until))
}

// sleepItems are the menu's rows.
func (p *player) sleepItems(sleep *services.SleepTimer, until string) []kit.MenuItem {
	// The radio: the minutes the user picked while the timer they set runs.
	value := ""
	if sleep != nil {
		value = sleep.Mode
		if sleep.Mode == "after" {
			value = p.sleep.chosen
		}
	}
	items := []kit.MenuItem{{Key: "heading", Kind: kit.MenuLabel, Label: i18n.T("Stop playing")}}
	for i, ch := range sleepChoices() {
		if i == 5 {
			items = append(items, kit.MenuItem{Key: "sep-choices", Kind: kit.MenuSeparator})
		}
		items = append(items, kit.MenuItem{
			Key: ch.value, Kind: kit.MenuRadio, Label: ch.label, Checked: value == ch.value,
			Run: func() {
				p.sleep.chosen = ch.value
				if mins, err := strconv.Atoi(ch.value); err == nil {
					p.setSleep("after", mins)
					return
				}
				p.setSleep(ch.value, 0)
			},
		})
	}
	if sleep == nil {
		return items
	}
	var note string
	switch sleep.Mode {
	case "after":
		note = strings.NewReplacer("<0>", "", "</0>", "").Replace(i18n.T("Pauses at <0>{until}</0>, fading out over the last 10 seconds.", "until", until))
	case "track":
		note = i18n.T("Pauses when this song ends, fading out over the last 10 seconds.")
	default:
		note = i18n.T("Pauses when the last song you queued ends, fading out over the last 10 seconds.")
	}
	return append(items,
		kit.MenuItem{Key: "sep-off", Kind: kit.MenuSeparator},
		kit.MenuItem{Key: "note", Kind: kit.MenuLabel, Label: note},
		kit.MenuItem{Key: "off", Kind: kit.MenuAction, Label: i18n.T("Turn off sleep timer"), Inset: true, Run: func() { p.setSleep("", 0) }},
	)
}
