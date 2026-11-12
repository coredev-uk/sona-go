package settings

import (
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// startPage is a page Sona can open on.
type startPage struct {
	Route string
	Label string
}

// startPages are the pages Sona can open on, in sidebar order (Go's
// config.StartPages).
func startPages() []startPage {
	return []startPage{
		{"/", i18n.T("For you")},
		{"/new", i18n.T("Discover")},
		{"/charts", i18n.T("Charts")},
		{"/radio", i18n.T("Radio")},
		{"/library/recently-added", i18n.T("Recently added")},
		{"/library/artists", i18n.T("Artists")},
		{"/library/albums", i18n.T("Albums")},
		{"/library/songs", i18n.T("Songs")},
		{"/library/playlists", i18n.T("Playlists")},
	}
}

// summaries is each section's state in one catalogue line ("High quality ·
// Crossfade 6 s"), for the section list: what a section is set to reads
// without opening it. Only cheap data: the settings, the capabilities and
// the scrobblers' status; Diagnostics, whose checks cost a run, says what
// it holds instead. Empty while the settings or the capabilities load
// (summaries.ts useSectionSummaries).
func (s *settings) summaries() map[string]string {
	st, caps, ok := s.ready()
	if !ok {
		return nil
	}
	d := st.Desktop

	general := ""
	if d.ResumeLastRoute {
		general = i18n.T("Opens where you left off")
	} else if _, page := s.openOn(d); page != "" {
		general = i18n.T("Opens on {page}", "page", page)
	}
	closing := ""
	if caps.Tray {
		if d.CloseAction == "quit" {
			closing = i18n.T("Quits when closed")
		} else {
			closing = i18n.T("Keeps playing when closed")
		}
	}
	general = i18n.Meta(general, closing)

	playback := i18n.T("Not available on {platform} yet", "platform", s.platformName())
	if caps.Playback.Available {
		quality := i18n.T("High quality")
		if st.AudioQuality == 64 {
			quality = i18n.T("Data saver")
		}
		sound, fade := "", ""
		if caps.Playback.SoundCheck && st.Audio.SoundCheck {
			sound = i18n.T("Sound Check")
		}
		if caps.Playback.Crossfade && st.Crossfade.Enabled {
			fade = i18n.T("Crossfade {seconds} s", "seconds", st.Crossfade.Seconds)
		}
		playback = i18n.Meta(quality, sound, fade)
	}

	// The theme and its look: "Graphite · Dark". A theme with one look
	// holds it whatever the mode says.
	active := theme.Active()
	mode := i18n.T("System")
	switch dark, pinned := active.Pinned(); {
	case pinned && dark, !pinned && st.Theme == "dark":
		mode = i18n.T("Dark")
	case pinned, st.Theme == "light":
		mode = i18n.T("Light")
	}
	themeLine := i18n.Meta(themeName(active), mode)
	effects := ""
	switch {
	case st.ReduceEffects:
		effects = i18n.T("Reduced effects")
	case !st.MotionArtwork.Enabled:
		effects = i18n.T("Motion artwork off")
	}
	appearance := i18n.Meta(themeLine, effects)

	var connected []string
	{
		sc := s.scrobbleStatus().Data
		if sc.LastFM.Connected {
			connected = append(connected, providerName("lastfm"))
		}
		if sc.ListenBrainz.Connected {
			connected = append(connected, providerName("listenbrainz"))
		}
	}
	if caps.DiscordPresence && st.Integrations.DiscordPresence {
		connected = append(connected, "Discord")
	}
	connections := i18n.T("Nothing connected")
	if len(connected) > 0 {
		connections = i18n.List(connected)
	}

	conflicts := shortcutConflicts(s.shortcuts())
	changed := len(st.Shortcuts)
	var shortcuts []string
	if conflicts > 0 {
		shortcuts = append(shortcuts, i18n.T("{conflicts, plural, one {# conflict} other {# conflicts}}", "conflicts", conflicts))
	}
	if changed > 0 {
		shortcuts = append(shortcuts, i18n.T("{changed, plural, one {# changed} other {# changed}}", "changed", changed))
	} else {
		shortcuts = append(shortcuts, i18n.T("Default keys"))
	}

	mib := st.Storage.ArtCacheMiB
	limit := i18n.T("{mib} MB", "mib", mib)
	if mib >= 1024 {
		limit = i18n.T("{gb} GB", "gb", float64(mib)/1024)
	}

	return map[string]string{
		secGeneral:     general,
		secPlayback:    playback,
		secAppearance:  appearance,
		secConnections: connections,
		secShortcuts:   i18n.Meta(shortcuts...),
		secStorage:     i18n.T("Artwork up to {limit}", "limit", limit),
		secDiagnostics: i18n.T("Checks, logs and bug reports"),
	}
}
