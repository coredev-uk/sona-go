package settings

import (
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// The sections of the page, in the order it lists them (search-params.ts).
const (
	secGeneral     = "general"
	secPlayback    = "playback"
	secAppearance  = "appearance"
	secConnections = "connections"
	secShortcuts   = "shortcuts"
	secStorage     = "storage"
	secDiagnostics = "diagnostics"
)

var sections = []string{secGeneral, secPlayback, secAppearance, secConnections, secShortcuts, secStorage, secDiagnostics}

// sectionTitle is a section's name: in the section list, its heading and
// Find a setting's results.
func sectionTitle(section string) string {
	switch section {
	case secGeneral:
		return i18n.T("General")
	case secPlayback:
		return i18n.T("Playback")
	case secAppearance:
		return i18n.T("Appearance")
	case secConnections:
		return i18n.T("Connections")
	case secShortcuts:
		return i18n.T("Shortcuts")
	case secStorage:
		return i18n.T("Storage")
	case secDiagnostics:
		return i18n.T("Diagnostics")
	}
	return section
}

// groupTitle is the heading of a shortcut group in the palette, the help
// sheet and Settings (shared/shortcuts/groups.ts).
func groupTitle(g module.ShortcutGroup) string {
	switch g {
	case "playback":
		return i18n.T("Playback")
	case "view":
		return i18n.T("View")
	case "pages":
		return i18n.T("Pages")
	case "navigation":
		return i18n.T("Navigation")
	case "general":
		return i18n.T("General")
	}
	return string(g)
}

// findContext is what decides whether a row is on its section at all.
type findContext struct {
	caps     *services.Capabilities // nil until loaded
	settings *config.Settings       // nil until loaded
	// languageRow: Settings → General shows its Language row (a second
	// language is selectable).
	languageRow bool
}

// settingEntry is one setting Find a setting can land on. ID is the row's
// find ID (the s.row ID, v0.2.0's data-setting); Title is the row's own
// title, so a result reads as the row it opens.
type settingEntry struct {
	ID      string
	Section string
	Group   string
	Title   string
	// Keywords are other words people search for it by.
	Keywords string
	// When: rows a section shows only in some states (not for
	// capabilities: those rows say why they are unavailable).
	When func(findContext) bool
}

func playable(c findContext) bool { return c.caps != nil && c.caps.Playback.Available }

// selfUpdating: the OS whose installs update themselves (macOS, Windows).
func selfUpdating(c findContext) bool {
	return c.caps != nil && (c.caps.OS == "darwin" || c.caps.OS == "windows")
}

func cdmEngine(c findContext) bool {
	return playable(c) && c.caps.Playback.Engine == "cdm"
}

// settingIndex is every setting of the page in page order, then every
// shortcut (find.ts settingIndex). A test builds each section and checks
// that each entry's row is there. The rows only WebKitGTK had (the
// Rendering switches, the web inspector's switch) do not exist natively.
func settingIndex(shortcuts []module.BoundShortcut) []settingEntry {
	general := sectionTitle(secGeneral)
	starting := i18n.T("Starting Sona")
	window := i18n.T("Window")
	playing := i18n.T("While music plays")
	updates := i18n.T("Updates")
	sound := i18n.T("Sound")
	listening := i18n.T("Listening")
	lyrics := i18n.T("Lyrics")
	advanced := i18n.T("Advanced")
	appearance := sectionTitle(secAppearance)
	scrobbling := i18n.T("Scrobbling")
	discord := i18n.T("Discord")
	storage := sectionTitle(secStorage)
	diagnostics := sectionTitle(secDiagnostics)
	developerTools := i18n.T("Developer tools")

	e := []settingEntry{
		{
			ID: "language", Section: secGeneral, Group: general, Title: i18n.T("Language"),
			When: func(c findContext) bool { return c.languageRow },
		},
		{
			ID: "autostart", Section: secGeneral, Group: starting, Title: i18n.T("Start Sona when you log in"),
			Keywords: i18n.T("login, startup, autostart"),
		},
		{
			ID: "start-hidden", Section: secGeneral, Group: starting, Title: i18n.T("Start in the tray"),
			When: func(c findContext) bool {
				return c.caps != nil && c.caps.Autostart && c.caps.Tray && c.settings != nil && c.settings.Desktop.Autostart
			},
		},
		{
			ID: "start-page", Section: secGeneral, Group: starting, Title: i18n.T("Open on"),
			Keywords: i18n.T("start page, resume"),
		},
		{
			ID: "link-handler", Section: secGeneral, Group: starting, Title: i18n.T("Open Apple Music links in Sona"),
			Keywords: i18n.T("music://, itms://, default app, Open in Music, links"),
			When:     func(c findContext) bool { return c.caps != nil && (c.caps.LinkHandler || c.caps.NeedsAppBundle) },
		},
		{
			ID: "setup-again", Section: secGeneral, Group: starting, Title: i18n.T("First-run setup"),
			Keywords: i18n.T("Set up Sona again, welcome"),
		},
		{
			ID: "close-action", Section: secGeneral, Group: window, Title: i18n.T("When the window is closed"),
			Keywords: i18n.T("tray, background, quit"),
		},
		{
			ID: "title-bar", Section: secGeneral, Group: window, Title: i18n.T("Title bar"),
			Keywords: i18n.T("window buttons, decorations"),
			When:     func(c findContext) bool { return c.caps != nil && c.caps.WindowControls.Native == "frame" },
		},
		{ID: "window-size", Section: secGeneral, Group: window, Title: i18n.T("Window size")},
		{ID: "track-notifications", Section: secGeneral, Group: playing, Title: i18n.T("Track notifications")},
		{
			ID: "inhibit-sleep", Section: secGeneral, Group: playing, Title: i18n.T("Stay awake while playing"),
			Keywords: i18n.T("sleep, suspend"),
		},
		{ID: "updates-check", Section: secGeneral, Group: updates, Title: i18n.T("Check for updates when Sona starts")},
		{
			ID: "updates-prereleases", Section: secGeneral, Group: updates, Title: i18n.T("Test versions"),
			Keywords: i18n.T("beta, release candidate, prerelease"), When: selfUpdating,
		},

		{ID: "cdm", Section: secPlayback, Group: sectionTitle(secPlayback), Title: i18n.T("Widevine"), When: cdmEngine},
		{
			ID: "quality", Section: secPlayback, Group: sound, Title: i18n.T("Audio quality"),
			Keywords: i18n.T("bitrate, data saver, lossless"), When: playable,
		},
		{
			ID: "output", Section: secPlayback, Group: sound, Title: i18n.T("Output"),
			Keywords: i18n.T("speakers, headphones, device"), When: playable,
		},
		{
			ID: "sound-check", Section: secPlayback, Group: sound, Title: i18n.T("Sound Check"),
			Keywords: i18n.T("loudness, volume"), When: playable,
		},
		{ID: "equalizer", Section: secPlayback, Group: sound, Title: i18n.T("Equaliser"), When: playable},
		{ID: "crossfade", Section: secPlayback, Group: sound, Title: i18n.T("Crossfade"), When: playable},
		{
			ID: "crossfade-seconds", Section: secPlayback, Group: sound, Title: i18n.T("Crossfade length"),
			When: func(c findContext) bool {
				return playable(c) && c.caps.Playback.Crossfade && c.settings != nil && c.settings.Crossfade.Enabled
			},
		},
		{
			ID: "private-listening", Section: secPlayback, Group: listening,
			Title: i18n.T("Private listening"), When: playable,
		},
		{ID: "replace-queue", Section: secPlayback, Group: listening, Title: i18n.T("Replacing your queue"), When: playable},
		{
			ID: "lyrics-sources", Section: secPlayback, Group: lyrics, Title: i18n.T("Lyrics sources"),
			Keywords: i18n.T("Apple Music, Musixmatch, LRCLIB, NetEase"),
		},
		{ID: "lyrics-word-timing", Section: secPlayback, Group: lyrics, Title: i18n.T("Prefer word-by-word lyrics")},
		{
			ID: "lyrics-translation", Section: secPlayback, Group: lyrics, Title: i18n.T("Lyrics translation"),
			Keywords: i18n.T("language"),
		},
		{ID: "idle", Section: secPlayback, Group: advanced, Title: i18n.T("Free memory when idle"), When: cdmEngine},

		{ID: "theme", Section: secAppearance, Group: appearance, Title: i18n.T("Theme"), Keywords: i18n.T("dark, light")},
		{
			ID: "reduce-effects", Section: secAppearance, Group: appearance, Title: i18n.T("Reduce effects"),
			Keywords: i18n.T("animation, motion, blur"),
		},
		{
			ID: "motion-artwork", Section: secAppearance, Group: appearance, Title: i18n.T("Motion artwork"),
			Keywords: i18n.T("video, animated covers"),
		},

		{ID: "scrobble-lastfm", Section: secConnections, Group: scrobbling, Title: i18n.T("Last.fm")},
		{ID: "scrobble-listenbrainz", Section: secConnections, Group: scrobbling, Title: i18n.T("ListenBrainz")},
		{ID: "discord-presence", Section: secConnections, Group: discord, Title: i18n.T("Discord status")},

		{ID: "art-cache", Section: secStorage, Group: storage, Title: i18n.T("Artwork cache"), Keywords: i18n.T("disk, space")},
		{
			ID: "clear-cache", Section: secStorage, Group: storage, Title: i18n.T("Catalogue cache"),
			Keywords: i18n.T("Clear caches, offline"),
		},
		{ID: "compact", Section: secStorage, Group: storage, Title: i18n.T("Library database"), Keywords: i18n.T("Compact now")},
		{
			ID: "settings-file", Section: secStorage, Group: storage, Title: i18n.T("Settings file"),
			Keywords: i18n.T("export, import, backup"),
		},

		{
			ID: "diagnostics-copy", Section: secDiagnostics, Group: diagnostics, Title: i18n.T("Copy diagnostics"),
			Keywords: i18n.T("bug report"),
		},
		{
			ID: "diagnostics-logs", Section: secDiagnostics, Group: diagnostics, Title: i18n.T("Open log folder"),
			Keywords: i18n.T("logs"),
		},
		// The web inspector is MyGo's now: one row, in dev builds started with
		// SONA_INSPECTOR=1; elsewhere the row says it is unavailable.
		{
			ID: "developer-tools", Section: secDiagnostics, Group: developerTools, Title: i18n.T("Open the inspector"),
			Keywords: i18n.T("devtools, console, inspect element, debug"),
		},
	}
	for _, s := range shortcuts {
		label := s.ID
		if s.Label != nil {
			label = s.Label()
		}
		e = append(e, settingEntry{ID: "shortcut-" + s.ID, Section: secShortcuts, Group: groupTitle(s.Group), Title: label})
	}
	return e
}

// fold is lower case without accents, so "equaliser" finds "Égaliseur" and
// the other way round.
func fold(s, locale string) string {
	tag, err := language.Parse(locale)
	if err != nil {
		tag = language.Und
	}
	s = cases.Lower(tag).String(s)
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, norm.NFD.String(s))
}

// findSettings are the entries a query finds: every word of it somewhere in
// the title, the group, the section or the keywords; titles that start with
// it first, then titles with a word that starts with its first word, then
// titles holding it, then the rest, each in section then page order.
func findSettings(entries []settingEntry, query string, ctx findContext, locale string) []settingEntry {
	q := fold(strings.TrimSpace(query), locale)
	if q == "" {
		return nil
	}
	words := strings.Fields(q)
	type ranked struct {
		e    settingEntry
		i, r int
	}
	var found []ranked
	for i, e := range entries {
		if e.When != nil && !e.When(ctx) {
			continue
		}
		hay := fold(strings.Join([]string{e.Title, e.Group, sectionTitle(e.Section), e.Keywords}, " "), locale)
		if !slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(hay, w) }) {
			title := fold(e.Title, locale)
			r := 3
			switch {
			case strings.HasPrefix(title, q):
				r = 0
			case slices.ContainsFunc(strings.Fields(title), func(w string) bool { return strings.HasPrefix(w, words[0]) }):
				r = 1
			case strings.Contains(title, q):
				r = 2
			}
			found = append(found, ranked{e, i, r})
		}
	}
	slices.SortStableFunc(found, func(a, b ranked) int {
		if a.r != b.r {
			return a.r - b.r
		}
		if x, y := slices.Index(sections, a.e.Section), slices.Index(sections, b.e.Section); x != y {
			return x - y
		}
		return a.i - b.i
	})
	out := make([]settingEntry, len(found))
	for i, f := range found {
		out[i] = f.e
	}
	return out
}
