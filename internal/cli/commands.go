package cli

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
)

// command is one `sona <command>`. The table below is the only list of
// commands: parsing, help, the manual page and the shell completions all
// read it.
type command struct {
	name    string
	args    string       // the arguments' synopsis ("" for none)
	argName string       // what a missing argument is called in errors
	argHelp l10n.Message // the arguments' description
	summary l10n.Message
	help    l10n.Message
	flags   []flag
	// choices are the values the first argument may take (validation and
	// completion).
	choices          []string
	minArgs, maxArgs int // maxArgs -1: any number
	// local commands run in this process (help, completion); the others
	// are sent to the running Sona.
	local bool
	// ownerOnly commands are for the person at the terminal: an add-on's
	// channel may not send them (RequestPermission refuses them).
	ownerOnly bool
	// timeout bounds the round trip; commandTimeout when zero.
	timeout time.Duration
	// check validates an invocation beyond the table's rules.
	check func(*invocation) error
}

// flag is a --option. value names its value ("" for a switch); choices,
// when set, are the values it accepts.
type flag struct {
	name    string
	value   string
	help    l10n.Message
	choices []string
}

const (
	// commandTimeout bounds a command's round trip.
	commandTimeout = 10 * time.Second
	// networkTimeout bounds commands that may wait on Apple.
	networkTimeout = 30 * time.Second
	// defaultLimit and maxLimit bound search results per kind.
	defaultLimit = 5
	maxLimit     = 20
)

var (
	switches     = []string{"on", "off", "toggle", "status"}
	repeats      = []string{"off", "all", "one", "cycle", "status"}
	libraryVerbs = []string{"add", "remove", "toggle", "status", "playlists", "albums"}
	queueVerbs   = []string{"jump", "clear"}
	shells       = []string{"bash", "zsh", "fish"}
	kinds        = []string{"song", "album", "artist", "playlist", "station", "music-video", "curator"}
	flagJSON     = flag{name: "json", help: msgFlagJSON}
	volumeExpr   = regexp.MustCompile(`^[+-]?[0-9]{1,3}$`)
	seekExpr     = regexp.MustCompile(`^[+-]?[0-9]{1,5}(\.[0-9]{1,3})?$`)
	// negative is a lone dash before a number ("-10", "-2.5"): an
	// argument (a volume or seek change), not an option.
	negative = regexp.MustCompile(`^-[0-9]+(\.[0-9]+)?$`)
)

// commands is the command table, in help order.
var commands = []*command{
	{
		name: "play", args: "[<link>]", argName: "link", argHelp: msgArgPlayLink,
		summary: msgPlaySummary, help: msgPlayHelp, maxArgs: 1, timeout: networkTimeout,
		flags: []flag{flagJSON, {name: "shuffle", help: msgFlagShuffle}},
		check: func(in *invocation) error {
			if in.shuffle && len(in.args) == 0 {
				return Usage(msgFlagNeeds, "play", "--shuffle", "<link>")
			}
			return nil
		},
	},
	{name: "pause", summary: msgPauseSummary, help: msgPauseHelp, flags: []flag{flagJSON}},
	{name: "toggle", summary: msgToggleSummary, help: msgToggleHelp, flags: []flag{flagJSON}},
	{name: "next", summary: msgNextSummary, help: msgNextHelp, flags: []flag{flagJSON}},
	{name: "prev", summary: msgPrevSummary, help: msgPrevHelp, flags: []flag{flagJSON, {name: "force", help: msgFlagForce}}},
	{
		name: "seek", args: "<seconds>|+N|-N", argName: "position", argHelp: msgArgSeek,
		summary: msgSeekSummary, help: msgSeekHelp, minArgs: 1, maxArgs: 1, flags: []flag{flagJSON},
		check: func(in *invocation) error {
			_, _, err := ParseSeek(in.args[0])
			return err
		},
	},
	{
		name: "shuffle", args: "[on|off|toggle|status]", argHelp: msgArgSwitch, choices: switches,
		summary: msgShuffleSummary, help: msgShuffleHelp, maxArgs: 1, flags: []flag{flagJSON},
	},
	{
		name: "repeat", args: "[off|all|one|cycle|status]", argHelp: msgArgRepeat, choices: repeats,
		summary: msgRepeatSummary, help: msgRepeatHelp, maxArgs: 1, flags: []flag{flagJSON},
	},
	{
		name: "crossfade", args: "[on|off|toggle|status]", argHelp: msgArgSwitch, choices: switches,
		summary: msgCrossfadeSummary, help: msgCrossfadeHelp, maxArgs: 1, flags: []flag{flagJSON},
	},
	{
		name: "status", summary: msgStatusSummary, help: msgStatusHelp,
		flags: []flag{
			flagJSON,
			{name: "follow", help: msgFlagFollow},
			{name: "format", value: "TEMPLATE", help: msgFlagFormat},
		},
		check: func(in *invocation) error {
			if in.json && in.format != "" {
				return Usage(msgFlagConflict, "status", "--json", "--format")
			}
			if in.format != "" {
				if _, err := statusTemplate(in.format); err != nil {
					return Usage(msgBadTemplate, "status", err)
				}
			}
			return nil
		},
	},
	{
		name: "lyrics", summary: msgLyricsSummary, help: msgLyricsHelp, timeout: networkTimeout,
		flags: []flag{
			flagJSON,
			{name: "lrc", help: msgFlagLRC},
			{name: "follow", help: msgFlagFollowLyrics},
			{name: "format", value: "TEMPLATE", help: msgFlagFormatLyrics},
			{name: "translation", help: msgFlagTranslation},
			{name: "transliteration", help: msgFlagTransliteration},
		},
		check: checkLyrics,
	},
	{
		name: "queue", args: "[jump <n>|clear]", argName: "verb", argHelp: msgArgQueueVerb, choices: queueVerbs,
		summary: msgQueueSummary, help: msgQueueHelp, maxArgs: 2, flags: []flag{flagJSON}, check: checkQueue,
	},
	{
		name: "search", args: "<term>…", argName: "term", argHelp: msgArgTerm,
		summary: msgSearchSummary, help: msgSearchHelp, minArgs: 1, maxArgs: -1, timeout: networkTimeout,
		flags: []flag{
			flagJSON,
			{name: "type", value: "KIND", help: msgFlagType, choices: kinds},
			{name: "limit", value: "N", help: msgFlagLimit},
		},
	},
	{
		name: "open", args: "<link>", argName: "link", argHelp: msgArgURL,
		summary: msgOpenSummary, help: msgOpenHelp, minArgs: 1, maxArgs: 1, flags: []flag{flagJSON},
	},
	{
		name: "volume", args: "[<0-100>|+N|-N]", argName: "volume", argHelp: msgArgVolume,
		summary: msgVolumeSummary, help: msgVolumeHelp, maxArgs: 1, flags: []flag{flagJSON},
		check: func(in *invocation) error {
			if len(in.args) == 1 {
				if _, _, err := ParseVolume(in.args[0]); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		name: "like", args: "[on|off|toggle|status]", argHelp: msgArgSwitch, choices: switches,
		summary: msgLikeSummary, help: msgLikeHelp, maxArgs: 1, timeout: networkTimeout, flags: []flag{flagJSON},
	},
	{
		name: "dislike", args: "[on|off|toggle|status]", argHelp: msgArgSwitch, choices: switches,
		summary: msgDislikeSummary, help: msgDislikeHelp, maxArgs: 1, timeout: networkTimeout, flags: []flag{flagJSON},
	},
	{
		name: "library", args: "[add|remove|toggle|status|playlists|albums]", argHelp: msgArgLibraryVerb, choices: libraryVerbs,
		summary: msgLibrarySummary, help: msgLibraryHelp, maxArgs: 1, timeout: networkTimeout, flags: []flag{flagJSON},
	},
	{
		name: "private", args: "[on|off|toggle|status]", argHelp: msgArgSwitch, choices: switches,
		summary: msgPrivateSummary, help: msgPrivateHelp, maxArgs: 1, flags: []flag{flagJSON},
	},
	{
		name: "addons", args: "dev <folder>", argName: "verb", argHelp: msgArgAddonVerb,
		summary: msgAddonsSummary, help: msgAddonsHelp, choices: []string{addonsDev}, minArgs: 1, maxArgs: 3,
		ownerOnly: true, timeout: devAskTimeout, check: checkAddons,
	},
	// connect runs until its program hangs up; a request can't carry it.
	{name: "connect", summary: msgConnectSummary, help: msgConnectHelp, ownerOnly: true},
	{
		name: "help", args: "[<command>]", argHelp: msgArgHelpCommand, summary: msgHelpSummary, help: msgHelpHelp,
		maxArgs: 1, local: true, flags: []flag{{name: "man", help: msgFlagMan}},
	},
	{
		name: "completion", args: "bash|zsh|fish", argName: "shell", argHelp: msgArgShell, choices: shells,
		summary: msgCompletionSummary, help: msgCompletionHelp, minArgs: 1, maxArgs: 1, local: true,
	},
}

// checkLyrics validates `lyrics`'s options as a set: the formats exclude
// each other, --format belongs to --follow, and the one-shot --json carries
// every localization already (so shows none instead of the line).
func checkLyrics(in *invocation) error {
	switch {
	case in.json && in.format != "":
		return Usage(msgFlagConflict, "lyrics", "--json", "--format")
	case in.lrc && in.json:
		return Usage(msgFlagConflict, "lyrics", "--lrc", "--json")
	case in.lrc && in.follow:
		return Usage(msgFlagConflict, "lyrics", "--lrc", "--follow")
	case in.lrc && in.format != "":
		return Usage(msgFlagConflict, "lyrics", "--lrc", "--format")
	case in.translation && in.transliteration:
		return Usage(msgFlagConflict, "lyrics", "--translation", "--transliteration")
	case in.format != "" && !in.follow:
		return Usage(msgFlagNeeds, "lyrics", "--format", "--follow")
	case in.json && !in.follow && in.translation:
		return Usage(msgFlagConflict, "lyrics", "--json", "--translation")
	case in.json && !in.follow && in.transliteration:
		return Usage(msgFlagConflict, "lyrics", "--json", "--transliteration")
	}
	if in.format != "" {
		if _, err := statusTemplate(in.format); err != nil {
			return Usage(msgBadTemplate, "lyrics", err)
		}
	}
	return nil
}

// checkQueue validates `queue`'s verbs: none lists the queue, `clear`
// takes nothing more and `jump` a place in the queue.
func checkQueue(in *invocation) error {
	if len(in.args) == 0 {
		return nil
	}
	switch in.args[0] {
	case "clear":
		if len(in.args) > 1 {
			return Usage(msgTooManyArgs, "queue")
		}
	case "jump":
		if len(in.args) < 2 {
			return Usage(msgMissingArg, "queue", "n")
		}
		if _, err := ParseQueuePlace(in.args[1]); err != nil {
			return err
		}
	default:
		return Usage(msgBadChoice, "queue", in.args[0], strings.Join(queueVerbs, ", "))
	}
	return nil
}

func init() {
	// help lists the commands, so its own choices come from the table.
	help := lookup("help")
	for _, c := range commands {
		help.choices = append(help.choices, c.name)
	}
}

// lookup returns the named command, or nil.
func lookup(name string) *command {
	for _, c := range commands {
		if c.name == name {
			return c
		}
	}
	return nil
}

func (c *command) flag(name string) *flag {
	for i := range c.flags {
		if c.flags[i].name == name {
			return &c.flags[i]
		}
	}
	return nil
}

func (c *command) roundTrip() time.Duration {
	if c.timeout > 0 {
		return c.timeout
	}
	return commandTimeout
}

// IsCommand reports whether a launch's arguments (os.Args[1:]) are a
// command: the first names one, or asks for help. Anything else (no
// arguments, --hidden, Apple Music links) starts Sona.
func IsCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == "--help" || args[0] == "-h" || lookup(args[0]) != nil
}

// invocation is a parsed command line.
type invocation struct {
	cmd    *command
	args   []string
	help   bool // --help: show the command's help instead
	json   bool
	follow bool
	man    bool
	format string
	kind   string
	limit  int
	// lyrics' options: --lrc, and the localization shown instead of the
	// line.
	lrc, translation, transliteration bool
	// fromRequest: the invocation is a request the running Sona received, not
	// a command line (the verbs the client sends for itself are valid).
	fromRequest bool
	// shuffle: `play <link> --shuffle`; force: `prev --force`.
	shuffle, force bool
}

// parse reads a command line whose first argument is a command (see
// IsCommand). Options may come anywhere after the command; `--` ends them.
// A lone dash before a number ("-10", "-2.5") is an argument, not an option.
func parse(argv []string) (*invocation, error) {
	name := argv[0]
	if name == "--help" || name == "-h" {
		name = "help"
	}
	c := lookup(name)
	if c == nil {
		return nil, Usage(msgUnknownCommand, name)
	}
	in := &invocation{cmd: c}
	rest := argv[1:]
	done := false
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case done || !strings.HasPrefix(a, "-") || a == "-" || negative.MatchString(a):
			in.args = append(in.args, a)
			continue
		case a == "--":
			done = true
			continue
		case a == "-h" || a == "--help":
			in.help = true
			continue
		case !strings.HasPrefix(a, "--"):
			return nil, Usage(msgUnknownFlag, c.name, a)
		}
		key, val, hasVal := strings.Cut(a[2:], "=")
		f := c.flag(key)
		if f == nil {
			return nil, Usage(msgUnknownFlag, c.name, "--"+key)
		}
		if f.value == "" {
			if hasVal {
				return nil, Usage(msgFlagNoValue, c.name, "--"+key)
			}
		} else if !hasVal {
			if i+1 >= len(rest) {
				return nil, Usage(msgFlagValue, c.name, "--"+key)
			}
			i++
			val = rest[i]
		}
		if len(f.choices) > 0 && !slices.Contains(f.choices, val) {
			return nil, Usage(msgFlagChoice, c.name, "--"+key, strings.Join(f.choices, ", "))
		}
		if err := in.set(key, val); err != nil {
			return nil, err
		}
	}
	if in.help {
		return in, nil
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	return in, nil
}

// validate checks the arguments against the table. The running Sona
// checks every request the same way (Handle), so its side may trust them.
func (in *invocation) validate() error {
	c := in.cmd
	switch {
	case len(in.args) < c.minArgs:
		return Usage(msgMissingArg, c.name, c.argName)
	case c.maxArgs >= 0 && len(in.args) > c.maxArgs:
		return Usage(msgTooManyArgs, c.name)
	case len(c.choices) > 0 && len(in.args) == 1 && !slices.Contains(c.choices, in.args[0]):
		return Usage(msgBadChoice, c.name, in.args[0], strings.Join(c.choices, ", "))
	case in.kind != "" && !slices.Contains(kinds, in.kind):
		return Usage(msgFlagChoice, c.name, "--type", strings.Join(kinds, ", "))
	case in.limit != 0 && (in.limit < 1 || in.limit > maxLimit):
		return Usage(msgBadLimit, c.name, maxLimit)
	}
	if c.check != nil {
		return c.check(in)
	}
	return nil
}

// checkRequest validates a request as parse validates a command line.
func checkRequest(req Request) error {
	c := lookup(req.Command)
	if c == nil || c.local {
		return &Error{Code: CodeUnknownCommand, Message: req.Command}
	}
	in := &invocation{
		cmd: c, args: req.Args, kind: req.Kind, limit: req.Limit, shuffle: req.Shuffle, force: req.Force,
		fromRequest: true,
	}
	switch {
	case req.Shuffle && c.flag("shuffle") == nil:
		return Usage(msgUnknownFlag, c.name, "--shuffle")
	case req.Force && c.flag("force") == nil:
		return Usage(msgUnknownFlag, c.name, "--force")
	}
	return in.validate()
}

// set records option key (a flag of the command) with its value.
func (in *invocation) set(key, val string) error {
	switch key {
	case "json":
		in.json = true
	case "follow":
		in.follow = true
	case "man":
		in.man = true
	case "format":
		in.format = val
	case "lrc":
		in.lrc = true
	case "translation":
		in.translation = true
	case "transliteration":
		in.transliteration = true
	case "shuffle":
		in.shuffle = true
	case "force":
		in.force = true
	case "type":
		in.kind = val
	case "limit":
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 || n > maxLimit {
			return Usage(msgBadLimit, in.cmd.name, maxLimit)
		}
		in.limit = n
	}
	return nil
}

// request is what the invocation sends to the running Sona.
func (in *invocation) request() Request {
	req := Request{Command: in.cmd.name, Args: in.args, Shuffle: in.shuffle, Force: in.force}
	if in.cmd.name == "search" {
		req.Args = []string{strings.Join(in.args, " ")}
		req.Kind = in.kind
		req.Limit = cmp.Or(in.limit, defaultLimit)
	}
	return req
}

// ParseVolume reads `sona volume`'s argument: an absolute percentage
// (0 to 100), or a change (+N, -N) when relative.
func ParseVolume(arg string) (n int, relative bool, err error) {
	if !volumeExpr.MatchString(arg) {
		return 0, false, Usage(msgBadVolume, arg)
	}
	n, _ = strconv.Atoi(arg)
	relative = arg[0] == '+' || arg[0] == '-'
	if !relative && n > 100 {
		return 0, false, Usage(msgBadVolume, arg)
	}
	return n, relative, nil
}

// ParseSeek reads `sona seek`'s argument: a position in seconds, or a
// change (+N, -N seconds) when relative.
func ParseSeek(arg string) (seconds float64, relative bool, err error) {
	if !seekExpr.MatchString(arg) {
		return 0, false, Usage(msgBadSeek, arg)
	}
	seconds, _ = strconv.ParseFloat(arg, 64)
	return seconds, arg[0] == '+' || arg[0] == '-', nil
}

// ParseQueuePlace reads `sona queue jump`'s argument: a place in the queue
// as `sona queue` numbers it, from 1. It returns the index, from 0.
func ParseQueuePlace(arg string) (int, error) {
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 || strings.HasPrefix(arg, "+") {
		return 0, Usage(msgBadQueuePlace, arg)
	}
	return n - 1, nil
}

// SearchLimit is the number of results per kind a search request asks
// for, defaulted and bounded.
func SearchLimit(n int) int {
	if n < 1 {
		return defaultLimit
	}
	return min(n, maxLimit)
}
