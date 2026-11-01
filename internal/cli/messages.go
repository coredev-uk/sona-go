package cli

// Every text the command line shows is here, so a message catalogue can
// take them over in one place: command and flag descriptions (help, the
// manual page and the shell completions), the lines commands print and
// the error messages. JSON output (schema.go) is not text and stays as it
// is in every language.

// Command summaries (one line) and descriptions (help and the manual page).
const (
	msgPlaySummary       = "Resume playback"
	msgPlayHelp          = "Resumes playback, or starts the queue Sona saved when it last quit. Prints the status line."
	msgPauseSummary      = "Pause playback"
	msgPauseHelp         = "Pauses playback. Prints the status line."
	msgToggleSummary     = "Pause when playing, play otherwise"
	msgToggleHelp        = "Pauses when music plays and resumes otherwise, like the play/pause key. Prints the status line."
	msgNextSummary       = "Play the next song"
	msgNextHelp          = "Plays the next song in the queue. Prints the status line."
	msgPrevSummary       = "Play the previous song"
	msgPrevHelp          = "Restarts the playing song, or plays the previous one within its first seconds, like the previous key. Prints the status line."
	msgStatusSummary     = "Show what's playing"
	msgStatusHelp        = "Shows the playback state, the song, its position, the volume, shuffle, repeat and private listening. With --follow it keeps running and prints one line each time any of them changes (the position alone doesn't count), for status bars such as Waybar and Polybar; it ends when Sona quits. When Sona isn't running, it says so and exits with status 3."
	msgQueueSummary      = "List the queue"
	msgQueueHelp         = "Lists the queue in play order, marking the playing song."
	msgSearchSummary     = "Search Apple Music"
	msgSearchHelp        = "Searches the Apple Music catalogue in Sona's storefront and lists the results by kind, with their links; give a link to `sona open` to open it in Sona."
	msgOpenSummary       = "Open an Apple Music link in Sona"
	msgOpenHelp          = "Opens an Apple Music link (music.apple.com, music:// or itms://) in Sona's window, as clicking it would."
	msgVolumeSummary     = "Show or set the volume"
	msgVolumeHelp        = "Without a value, shows the volume. A number from 0 to 100 sets it; +N or -N changes it by N."
	msgLikeSummary       = "Favourite the playing song"
	msgLikeHelp          = "Favourites the playing song (on, the default), removes it from your favourites (off), switches it (toggle) or shows whether it is one (status). Favouriting a song adds it to your library, as in Sona. Needs an Apple Music account signed in to Sona."
	msgPrivateSummary    = "Show or switch private listening"
	msgPrivateHelp       = "Shows private listening (status, the default), or turns it on, off or over (toggle). While it is on, nothing you play is reported to Apple, scrobbled or kept in Sona's history."
	msgHelpSummary       = "Show help for Sona's commands"
	msgHelpHelp          = "Shows the commands, or one command's arguments and options. With --man it prints the manual page in roff instead."
	msgCompletionSummary = "Print a shell completion script"
	msgCompletionHelp    = "Prints the completion script for bash, zsh or fish. Packages install them; to load one by hand: `source <(sona completion bash)`, `sona completion zsh > \"${fpath[1]}/_sona\"` or `sona completion fish | source`."
)

// Flag descriptions.
const (
	msgFlagJSON       = "print the result as JSON"
	msgFlagFollow     = "keep printing a line each time the status changes, until Sona quits"
	msgFlagFormat     = "print each status with a Go text/template over the JSON fields"
	msgFlagType       = "only results of this kind"
	msgFlagLimit      = "at most this many results of each kind (1 to 20, default 5)"
	msgFlagMan        = "print the manual page in roff"
	msgFlagHelp       = "show help for the command"
	msgArgVolume      = "0 to 100, or +N or -N to change it by N"
	msgArgTerm        = "what to search for"
	msgArgURL         = "an Apple Music link"
	msgArgSwitch      = "on, off, toggle or status"
	msgArgShell       = "bash, zsh or fish"
	msgArgHelpCommand = "a command"
)

// Help and manual page text.
const (
	msgUsage          = "usage"
	msgUsageLine      = "sona <command> [arguments] [options]"
	msgLaunchLine     = "sona [--hidden] [<link>…]"
	msgVersionLine    = "sona --version"
	msgCommands       = "Commands"
	msgOptions        = "Options"
	msgArguments      = "Arguments"
	msgExitStatus     = "Exit status"
	msgHelpFooter     = "Commands control the Sona that is running; run `sona help <command>` for one command's options."
	msgManName        = "control the running Sona from the command line"
	msgManDescription = "Sona is an Apple Music client for the desktop. Started without a command, `sona` opens it, or hands its Apple Music links to the Sona already running. With a command, it controls the running Sona instead: on Linux over the D-Bus session bus, on macOS and Windows over a socket in Sona's state folder that only your user can reach. Commands never start Sona."
	msgManLaunch      = "Starting Sona"
	msgManLaunchText  = "`sona` starts Sona, or brings the running one to the front. Apple Music links among the arguments open in it. --hidden starts it without a window (playback, media keys and the tray only), as the start-at-login entry does. --version prints the version."
	msgManJSON        = "JSON output"
	msgManJSONText    = "With --json, a command prints one JSON object on standard output; `status --follow --json` prints one per line. Fields are only ever added, never renamed or removed. Errors go to standard error as text, with the exit status below."
	msgManFormat      = "Status bars"
	msgManFormatText  = "`status --format TEMPLATE` prints each status through a Go text/template whose fields are the JSON fields (`{{.state}}`, `{{with .track}}{{.artist}} – {{.title}}{{end}}`, `{{.volume}}`); `{{json .x}}` quotes a value as JSON and `{{clock .position}}` writes seconds as m:ss. With --follow it suits Waybar's and Polybar's custom modules."
	msgManFiles       = "Files"
	msgManFilesText   = "On macOS and Windows the running Sona listens on sona.sock in its state folder (~/Library/Application Support/Sona/State, %LOCALAPPDATA%\\Sona\\State), with sona.lock beside it. Only the user running Sona may connect."
	msgManSeeAlso     = "See also"
	msgManSeeAlsoText = "https://github.com/coredev-uk/sona-go/blob/main/docs/cli.md"
)

// Exit status descriptions.
const (
	msgExitOK         = "success"
	msgExitFailed     = "the command failed in Sona (the message says why)"
	msgExitUsage      = "the command line is wrong: an unknown command, argument or option"
	msgExitNotRunning = "Sona isn't running (status still prints it), or can't be reached"
)

// Errors.
const (
	msgNotRunning       = "Sona isn't running; start it first"
	msgUnreachable      = "can't reach the running Sona: %v"
	msgUnknownCommand   = "unknown command %q (run `sona help`)"
	msgUnknownFlag      = "%s: unknown option %s"
	msgFlagValue        = "%s: %s needs a value"
	msgFlagNoValue      = "%s: %s takes no value"
	msgFlagChoice       = "%s: %s must be one of %s"
	msgBadLimit         = "%s: --limit must be a number from 1 to %d"
	msgFlagConflict     = "%s: %s and %s can't be used together"
	msgTooManyArgs      = "%s: too many arguments"
	msgMissingArg       = "%s: missing %s"
	msgBadChoice        = "%s: %q isn't one of %s"
	msgBadLink          = "that isn't an Apple Music link Sona can open"
	msgStatusNotRunning = "Sona isn't running"
	msgBadVolume        = "volume: %q isn't 0 to 100, +N or -N"
	msgBadTemplate      = "status: --format: %v"
	msgStarting         = "Sona is starting; try again in a moment"
	msgShuttingDown     = "Sona is shutting down"
	msgNothingPlaying   = "nothing is playing"
	msgAccountLoading   = "Sona is still reading your Apple Music account; try again in a moment"
	msgNotSignedIn      = "Apple Music needs you signed in to Sona for this"
	msgPrivateForced    = "private listening is forced on for this run (SONA_PRIVATE=1)"
	msgIncompatible     = "the running Sona is a different version; quit and start it again"
	msgFollowEnded      = "lost the connection to Sona: %v"
	msgErrorPrefix      = "sona: "
	msgUnsupportedOnOS  = "this isn't available on this system"
)

// Output.
const (
	msgStateIdle       = "stopped"
	msgStateLoading    = "loading"
	msgStatePlaying    = "playing"
	msgStatePaused     = "paused"
	msgStateEnded      = "ended"
	msgStateError      = "error"
	msgNotRunningState = "not running"
	msgTrackLine       = "%s: %s"
	msgPreviewSuffix   = " (preview)"

	msgFieldState    = "state"
	msgFieldTitle    = "title"
	msgFieldArtist   = "artist"
	msgFieldAlbum    = "album"
	msgFieldPosition = "position"
	msgFieldVolume   = "volume"
	msgFieldShuffle  = "shuffle"
	msgFieldRepeat   = "repeat"
	msgFieldPrivate  = "private listening"
	msgFieldPreview  = "preview"
	msgFieldError    = "error"
	msgOn            = "on"
	msgOff           = "off"
	msgYes           = "yes"
	msgRepeatAll     = "all"
	msgRepeatOne     = "one"
	msgPositionOf    = "%s / %s"
	msgPercent       = "%d%%"

	msgQueueEmpty    = "the queue is empty"
	msgQueueCurrent  = "▶"
	msgQueueAutoPlay = "(AutoPlay)"
	msgTrackByArtist = "%s — %s"

	msgSearchNone = "no results for %q"

	msgOpened = "opening %s %s"

	msgLiked    = "favourite: on (%s)"
	msgNotLiked = "favourite: off (%s)"

	msgPrivateLine      = "private listening: %s"
	msgPrivateDev       = " (development build: this session only; saved setting %s)"
	msgPrivateForcedEnv = " (forced by SONA_PRIVATE=1)"
)

// Search result kind headings (text output).
var msgKinds = map[string]string{
	"song":        "Songs",
	"album":       "Albums",
	"artist":      "Artists",
	"playlist":    "Playlists",
	"station":     "Stations",
	"music-video": "Music videos",
	"curator":     "Curators",
}
