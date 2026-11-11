package cli

import "github.com/coredev-uk/sona-go/internal/i18n/l10n"

// Every text the command line shows is here, in Go's message catalogue
// (internal/i18n/l10n): command and flag descriptions (help, the manual
// page and the shell completions), the lines commands print and the error
// messages, shown through Env.Printer. JSON output (schema.go) is not text
// and stays as it is in every language, as do the command line's syntax
// (command and option names, the usage lines) and status templates.

// Usage lines and other text that is the same in every language.
const (
	usageLine   = "sona <command> [arguments] [options]"
	launchLine  = "sona [--hidden] [<link>…]"
	versionLine = "sona --version"
	errorPrefix = "sona: "
	queueMark   = "▶"
	seeAlsoURL  = "https://github.com/coredev-uk/sona-go/blob/main/docs/cli.md"
	// stateDirMac and stateDirWindows are the state folders msgManFilesText
	// names.
	stateDirMac     = "~/Library/Application Support/Sona/State"
	stateDirWindows = `%LOCALAPPDATA%\Sona\State`
)

// Command summaries (one line) and descriptions (help and the manual page).
const (
	msgPlaySummary       l10n.Message = "Resume playback"
	msgPlayHelp          l10n.Message = "Resumes playback, or starts the queue Sona saved when it last quit. Prints the status line."
	msgPauseSummary      l10n.Message = "Pause playback"
	msgPauseHelp         l10n.Message = "Pauses playback. Prints the status line."
	msgToggleSummary     l10n.Message = "Pause when playing, play otherwise"
	msgToggleHelp        l10n.Message = "Pauses when music plays and resumes otherwise, like the play/pause key. Prints the status line."
	msgNextSummary       l10n.Message = "Play the next song"
	msgNextHelp          l10n.Message = "Plays the next song in the queue. Prints the status line."
	msgPrevSummary       l10n.Message = "Play the previous song"
	msgPrevHelp          l10n.Message = "Restarts the playing song, or plays the previous one within its first seconds, like the previous key. Prints the status line."
	msgStatusSummary     l10n.Message = "Show what's playing"
	msgStatusHelp        l10n.Message = "Shows the playback state, the song, its position, the volume, shuffle, repeat and private listening. With --follow it keeps running and prints one line each time any of them changes (the position alone doesn't count), for status bars such as Waybar and Polybar; it ends when Sona quits. When Sona isn't running, it says so and exits with status 3."
	msgLyricsSummary     l10n.Message = "Print the playing song's lyrics"
	msgLyricsHelp        l10n.Message = "Prints the lyrics of the playing song, one line to a line, with a blank line between verses. With --lrc it writes an LRC file (a [mm:ss.xx] time before each line, and the title, artist and album tags) for a lyrics player; that needs lyrics with timing. With --follow it keeps running for status bars and shells such as Quickshell, Waybar and Polybar: it prints the line being sung now and another each time it changes (an empty line while the voice rests, and when there are no lyrics), until Sona quits. The lyrics come from the sources chosen in Sona's settings, as in its lyrics view."
	msgQueueSummary      l10n.Message = "List the queue"
	msgQueueHelp         l10n.Message = "Lists the queue in play order, marking the playing song."
	msgSearchSummary     l10n.Message = "Search Apple Music"
	msgSearchHelp        l10n.Message = "Searches the Apple Music catalogue in Sona's storefront and lists the results by kind, with their links; give a link to `sona open` to open it in Sona."
	msgOpenSummary       l10n.Message = "Open an Apple Music link in Sona"
	msgOpenHelp          l10n.Message = "Opens an Apple Music link (music.apple.com, music:// or itms://) in Sona's window, as clicking it would."
	msgVolumeSummary     l10n.Message = "Show or set the volume"
	msgVolumeHelp        l10n.Message = "Without a value, shows the volume. A number from 0 to 100 sets it; +N or -N changes it by N."
	msgLikeSummary       l10n.Message = "Favourite the playing song"
	msgLikeHelp          l10n.Message = "Favourites the playing song (on, the default), removes it from your favourites (off), switches it (toggle) or shows whether it is one (status). Favouriting a song adds it to your library, as in Sona. Needs an Apple Music account signed in to Sona."
	msgPrivateSummary    l10n.Message = "Show or switch private listening"
	msgPrivateHelp       l10n.Message = "Shows private listening (status, the default), or turns it on, off or over (toggle). While it is on, nothing you play is reported to Apple, scrobbled or kept in Sona's history."
	msgHelpSummary       l10n.Message = "Show help for Sona's commands"
	msgHelpHelp          l10n.Message = "Shows the commands, or one command's arguments and options. With --man it prints the manual page in roff instead."
	msgCompletionSummary l10n.Message = "Print a shell completion script"
	msgCompletionHelp    l10n.Message = "Prints the completion script for bash, zsh or fish. Packages install them; to load one by hand: `source <(sona completion bash)`, `sona completion zsh > \"${fpath[1]}/_sona\"` or `sona completion fish | source`."
)

// Flag descriptions.
const (
	msgFlagJSON            l10n.Message = "print the result as JSON"
	msgFlagFollow          l10n.Message = "keep printing a line each time the status changes, until Sona quits"
	msgFlagFormat          l10n.Message = "print each status with a Go text/template over the JSON fields"
	msgFlagFollowLyrics    l10n.Message = "keep printing the line being sung, a new one each time it changes, until Sona quits"
	msgFlagFormatLyrics    l10n.Message = "with --follow, print each line with a Go text/template over the JSON fields"
	msgFlagLRC             l10n.Message = "write an LRC file: a [mm:ss.xx] time before each line (needs lyrics with timing)"
	msgFlagTranslation     l10n.Message = "show each line's translation where it has one"
	msgFlagTransliteration l10n.Message = "show each line's transliteration (its sound in Latin script) where it has one"
	msgFlagType            l10n.Message = "only results of this kind"
	msgFlagLimit           l10n.Message = "at most this many results of each kind (1 to 20, default 5)"
	msgFlagMan             l10n.Message = "print the manual page in roff"
	msgFlagHelp            l10n.Message = "show help for the command"
	msgArgVolume           l10n.Message = "0 to 100, or +N or -N to change it by N"
	msgArgTerm             l10n.Message = "what to search for"
	msgArgURL              l10n.Message = "an Apple Music link"
	msgArgSwitch           l10n.Message = "on, off, toggle or status"
	msgArgShell            l10n.Message = "bash, zsh or fish"
	msgArgHelpCommand      l10n.Message = "a command"
)

// Help and manual page text.
const (
	msgUsage                 l10n.Message = "usage" // before a usage line: “usage: sona …”
	msgCommands              l10n.Message = "Commands"
	msgOptions               l10n.Message = "Options"
	msgArguments             l10n.Message = "Arguments"
	msgExitStatus            l10n.Message = "Exit status"
	msgHelpFooter            l10n.Message = "Commands control the Sona that is running; run `sona help <command>` for one command's options."
	msgManNameSection        l10n.Message = "Name"        // a manual page's section, shown in capitals
	msgManSynopsis           l10n.Message = "Synopsis"    // a manual page's section, shown in capitals
	msgManDescriptionSection l10n.Message = "Description" // a manual page's section, shown in capitals
	msgManName               l10n.Message = "control the running Sona from the command line"
	msgManDescription        l10n.Message = "Sona is an Apple Music client for the desktop. Started without a command, `sona` opens it, or hands its Apple Music links to the Sona already running. With a command, it controls the running Sona instead: on Linux over the D-Bus session bus, on macOS and Windows over a socket in Sona's state folder that only your user can reach. Commands never start Sona."
	msgManLaunch             l10n.Message = "Starting Sona"
	msgManLaunchText         l10n.Message = "`sona` starts Sona, or brings the running one to the front. Apple Music links among the arguments open in it. --hidden starts it without a window (playback, media keys and the tray only), as the start-at-login entry does. --version prints the version."
	msgManJSON               l10n.Message = "JSON output"
	msgManJSONText           l10n.Message = "With --json, a command prints one JSON object on standard output; `status --follow --json` and `lyrics --follow --json` print one per line. Fields are only ever added, never renamed or removed. Errors go to standard error as text, with the exit status below."
	msgManFormat             l10n.Message = "Status bars"
	msgManFormatText         l10n.Message = "`status --format TEMPLATE` prints each status through a Go text/template whose fields are the JSON fields (`{{.state}}`, `{{with .track}}{{.artist}} – {{.title}}{{end}}`, `{{.volume}}`); `{{json .x}}` quotes a value as JSON and `{{clock .position}}` writes seconds as m:ss. With --follow it suits Waybar's and Polybar's custom modules. `lyrics --follow` takes the same options for the line being sung (`{{.text}}`)."
	msgManFiles              l10n.Message = "Files"
	msgManFilesText          l10n.Message = "On macOS and Windows the running Sona listens on sona.sock in its state folder (%s, %s), with sona.lock beside it. Only the user running Sona may connect."
	msgManSeeAlso            l10n.Message = "See also"
)

// Exit status descriptions.
const (
	msgExitOK         l10n.Message = "success"
	msgExitFailed     l10n.Message = "the command failed in Sona (the message says why)"
	msgExitUsage      l10n.Message = "the command line is wrong: an unknown command, argument or option"
	msgExitNotRunning l10n.Message = "Sona isn't running (status still prints it), or can't be reached"
)

// Errors.
const (
	msgNotRunning       l10n.Message = "Sona isn't running; start it first"
	msgUnreachable      l10n.Message = "can't reach the running Sona: %v"
	msgUnknownCommand   l10n.Message = "unknown command %q (run `sona help`)"
	msgUnknownFlag      l10n.Message = "%s: unknown option %s" // the command, then the option
	msgFlagValue        l10n.Message = "%s: %s needs a value"
	msgFlagNoValue      l10n.Message = "%s: %s takes no value"
	msgFlagChoice       l10n.Message = "%s: %s must be one of %s"
	msgBadLimit         l10n.Message = "%s: --limit must be a number from 1 to %d"
	msgFlagConflict     l10n.Message = "%s: %s and %s can't be used together"
	msgFlagNeeds        l10n.Message = "%s: %s needs %s"
	msgTooManyArgs      l10n.Message = "%s: too many arguments"
	msgMissingArg       l10n.Message = "%s: missing %s"
	msgBadChoice        l10n.Message = "%s: %q isn't one of %s"
	msgBadLink          l10n.Message = "that isn't an Apple Music link Sona can open"
	msgStatusNotRunning l10n.Message = "Sona isn't running"
	msgBadVolume        l10n.Message = "volume: %q isn't 0 to 100, +N or -N"
	msgBadTemplate      l10n.Message = "%s: --format: %v"
	msgStarting         l10n.Message = "Sona is starting; try again in a moment"
	msgShuttingDown     l10n.Message = "Sona is shutting down"
	msgNothingPlaying   l10n.Message = "nothing is playing"
	msgNoLyrics         l10n.Message = "this song has no lyrics"
	msgNotSynced        l10n.Message = "these lyrics have no timing, so they can't be written as an LRC file; leave out --lrc for the plain text"
	msgAccountLoading   l10n.Message = "Sona is still reading your Apple Music account; try again in a moment"
	msgNotSignedIn      l10n.Message = "Apple Music needs you signed in to Sona for this"
	msgPrivateForced    l10n.Message = "private listening is forced on for this run (SONA_PRIVATE=1)"
	msgIncompatible     l10n.Message = "the running Sona is a different version; quit and start it again"
	msgFollowEnded      l10n.Message = "lost the connection to Sona: %v"
	msgUnsupportedOnOS  l10n.Message = "this isn't available on this system"
	msgSignInAgain      l10n.Message = "Apple Music no longer accepts your sign-in; sign in again in Sona"
	msgNetwork          l10n.Message = "Sona couldn't reach the network; check your connection and try again"
	msgTimeout          l10n.Message = "that took too long and timed out; try again"
	msgNotFound         l10n.Message = "it couldn't be found; it may have been removed"
	msgNotPlayable      l10n.Message = "this song isn't available to stream in your storefront"
	msgNotReleased      l10n.Message = "this song isn't out yet; it plays from its release date"
	msgLiveStation      l10n.Message = "that isn't available while listening to a live station"
	msgStation          l10n.Message = "that isn't available while listening to a station"
	msgNoSubscription   l10n.Message = "this Apple Account doesn't have an active Apple Music subscription"
	msgNoCDM            l10n.Message = "Sona needs Google's Widevine to play songs, and it isn't ready yet (Settings → Playback shows its progress)"
	msgKeyring          l10n.Message = "your keyring didn't answer or refused; unlock it, then try again"
	msgFailed           l10n.Message = "Sona couldn't do that: %s" // Sona's own message follows, in English
)

// Output.
const (
	msgStateIdle       l10n.Message = "stopped"
	msgStateLoading    l10n.Message = "loading"
	msgStatePlaying    l10n.Message = "playing"
	msgStatePaused     l10n.Message = "paused"
	msgStateEnded      l10n.Message = "ended"
	msgStateError      l10n.Message = "error"
	msgNotRunningState l10n.Message = "not running"
	// A label and its value: a status field (“volume: 50%”), or the state
	// and the song (“playing: Song — Artist”).
	msgFieldLine     l10n.Message = "%s: %s"
	msgPreviewSuffix l10n.Message = " (preview)"

	msgFieldState    l10n.Message = "state"
	msgFieldTitle    l10n.Message = "title"
	msgFieldArtist   l10n.Message = "artist"
	msgFieldAlbum    l10n.Message = "album"
	msgFieldPosition l10n.Message = "position"
	msgFieldVolume   l10n.Message = "volume"
	msgFieldShuffle  l10n.Message = "shuffle"
	msgFieldRepeat   l10n.Message = "repeat"
	msgFieldPrivate  l10n.Message = "private listening"
	msgFieldPreview  l10n.Message = "preview"
	msgFieldError    l10n.Message = "error"
	msgOn            l10n.Message = "on"  // a switch (shuffle, private listening) is on
	msgOff           l10n.Message = "off" // a switch (shuffle, repeat, private listening) is off
	msgYes           l10n.Message = "yes"
	msgRepeatAll     l10n.Message = "all"     // repeat: the whole queue
	msgRepeatOne     l10n.Message = "one"     // repeat: the playing song
	msgPositionOf    l10n.Message = "%s / %s" // the position, then the song's length (m:ss)
	msgPercent       l10n.Message = "%d%%"    // the volume

	msgQueueEmpty    l10n.Message = "the queue is empty"
	msgQueueAutoPlay l10n.Message = "(AutoPlay)"
	msgTrackByArtist l10n.Message = "%s — %s" // a song, then its artist

	msgSearchNone l10n.Message = "no results for %q"

	msgOpened l10n.Message = "opening %s %s" // the kind of item (album, song…), then its ID

	msgLiked    l10n.Message = "favourite: on (%s)"
	msgNotLiked l10n.Message = "favourite: off (%s)"

	msgPrivateLine      l10n.Message = "private listening: %s"
	msgPrivateDev       l10n.Message = " (development build: this session only; saved setting %s)"
	msgPrivateForcedEnv l10n.Message = " (forced by SONA_PRIVATE=1)"
)

// Search result kind headings (text output).
var msgKinds = map[string]l10n.Message{
	"song":        "Songs",
	"album":       "Albums",
	"artist":      "Artists",
	"playlist":    "Playlists",
	"station":     "Stations",
	"music-video": "Music videos",
	"curator":     "Curators",
}
