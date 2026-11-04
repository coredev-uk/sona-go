package onboarding

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// playbackStep is Playback: Widevine first, then the sound (quality as two
// cards, Sound Check, crossfade) and private listening, each only where the
// platform's playback can do it. Settings → Playback has these and more
// (PlaybackStep.tsx).
func (o *onboarding) playbackStep(c *ui.Context) {
	settings, caps := o.settings(), o.caps()
	priv := store.UsePrivate(o.env.Queries, o.env.Core)
	ui.Column(c.Key("playback")).FillWidth().Gap(20).Children(func() {
		o.widevineBlock(c)
		switch {
		case settings.Err != nil:
			o.settingsError(c, settings)
		case !settings.HasData || !caps.HasData:
			loading(c, 256)
		case caps.Data.Playback.Available:
			o.soundControls(c, settings.Data, caps.Data.Playback, priv)
		}
	})
}

// settingsError is ErrorBlock "Couldn't read Sona's settings" with its retry.
func (o *onboarding) settingsError(c *ui.Context, r store.Result[config.Settings]) {
	kit.ErrorBlock(c, i18n.T("Couldn't read Sona's settings"), r.Err, kit.ErrorBlockOpts{
		OnRetry:  func() { o.env.Queries.Invalidate(store.KeySettings) },
		Retrying: r.Fetching,
	})
}

func (o *onboarding) soundControls(c *ui.Context, s config.Settings, pb services.PlaybackCapabilities, priv store.Result[services.PrivateState]) {
	forced := priv.HasData && store.PrivateForced(priv.Data)
	privateHint := i18n.T("Plays stay out of your Apple Music history and recommendations, and aren't scrobbled.")
	switch {
	case forced:
		privateHint = i18n.T("On because Sona was started with SONA_PRIVATE=1.")
	case priv.HasData && priv.Data.Source == "development":
		privateHint = i18n.T("On in development builds; switching it here lasts until Sona quits.")
	}

	stepGroup(c, "quality", i18n.T("Audio quality"), func() {
		choiceCards(c, "quality", i18n.T("Audio quality"), strconv.Itoa(s.AudioQuality), []choice{
			{value: "256", label: i18n.T("High quality"), hint: i18n.T("AAC at 256 kbps")},
			{value: "64", label: i18n.T("Data saver"), hint: i18n.T("AAC at 64 kbps, for metered connections")},
		}, false, func(v string) {
			kbps, _ := strconv.Atoi(v)
			o.save(func(s *config.Settings) { s.AudioQuality = kbps })
		})
	})

	var rows []func()
	if pb.SoundCheck {
		rows = append(rows, func() {
			switchRow(c, "sound-check", i18n.T("Sound Check"), i18n.T("Plays every song at about the same loudness."),
				s.Audio.SoundCheck, false, func(on bool) {
					o.save(func(s *config.Settings) { s.Audio.SoundCheck = on })
				}, nil)
		})
	}
	if pb.Crossfade {
		rows = append(rows, func() {
			switchRow(c, "crossfade", i18n.T("Crossfade"), i18n.T("Fades each song into the next. Songs of one album still play without a gap."),
				s.Crossfade.Enabled, false, func(on bool) {
					o.save(func(s *config.Settings) { s.Crossfade.Enabled = on })
				}, func() {
					if s.Crossfade.Enabled {
						o.crossfadeLength(c, s)
					}
				})
		})
	}
	if pb.PrivateListening {
		rows = append(rows, func() {
			switchRow(c, "private", i18n.T("Private listening"), privateHint,
				priv.HasData && priv.Data.On, !priv.HasData || forced, func(on bool) {
					kit.SetPrivate(o.env.Queries, o.env.Core, on)
				}, nil)
		})
	}
	list(c, "sound", rows)
}

// crossfadeLength is crossfade's length under its switch while it is on;
// saved when the thumb is let go (CrossfadeLength).
func (o *onboarding) crossfadeLength(c *ui.Context, s config.Settings) {
	seconds := float64(s.Crossfade.Seconds)
	if o.fadeHeld {
		seconds = o.fade
	}
	ui.Row(c.Key("fade")).FillWidth().Gap(12).AlignItems(ui.Center).Margin(8, 0, 0, 0).Children(func() {
		text(c, i18n.T("Length"), "sub").TextColor(theme.C("ink-muted"))
		sl := kit.Slider(c.Key("length"), &seconds, config.MinCrossfadeSeconds, config.MaxCrossfadeSeconds, kit.SliderOpts{
			A11yLabel: i18n.T("Length"),
			ValueText: i18n.T("{seconds, plural, one {# second} other {# seconds}}", "seconds", int(seconds)),
			Step:      1,
			Range:     theme.C("ink-muted"),
			Width:     224,
		})
		if sl.Changed() {
			o.fade, o.fadeHeld = seconds, true
		}
		if o.fadeHeld && !sl.Dragging() && !sl.Pressed() {
			o.fadeHeld = false
			if n := int(o.fade); n != s.Crossfade.Seconds {
				o.save(func(s *config.Settings) { s.Crossfade.Seconds = n })
			}
		}
		text(c, i18n.T("{seconds} s", "seconds", int(seconds)), "meta").TextColor(theme.C("ink-muted")).Width(36)
	})
}
