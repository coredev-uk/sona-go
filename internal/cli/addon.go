package cli

import (
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Protocol v2 is what add-ons speak: newline-delimited JSON frames over
// the channel the supervisor gives the add-on (its standard input and
// output), never over the D-Bus bus or the command socket, which carry the
// v1 commands of this package and refuse every v2 method. docs/addons.md
// documents every frame; the Go side of it is here (types, limits,
// permissions) and in addon_session.go (the session).

// Protocol versions Sona speaks: major.minor.patch, of which the patch is
// ignored (it never changes a frame). A session settles on the highest
// version both sides speak, and is refused when there is none.
const (
	AddonProtocolMin = "2.0.0"
	AddonProtocolMax = "2.0.0"
)

// Methods an add-on sends (AddonMethodInvoke is the one Sona sends).
const (
	AddonMethodHello       = "hello"
	AddonMethodSubscribe   = "subscribe"
	AddonMethodUnsubscribe = "unsubscribe"
	AddonMethodCall        = "call"
	AddonMethodSecret      = "secret"
	AddonMethodInvoke      = "invoke"
)

// EventShutdown is the one event every session gets without subscribing:
// Sona is stopping the add-on.
const EventShutdown = "shutdown"

// Permissions an add-on may ask for and the user may grant. The protocol
// checks each at every call (Allowed); `network` is the OS sandbox's.
const (
	PermPlaybackRead    = "playback.read"
	PermPlaybackControl = "playback.control"
	PermQueueRead       = "queue.read"
	PermQueueControl    = "queue.control"
	PermLyricsRead      = "lyrics.read"
	PermSearch          = "search"
	PermOpen            = "open"
	PermVolume          = "volume"
	PermLike            = "like"
	PermLibrary         = "library"
	PermPrivateRead     = "private.read"
	// PermNetwork is the one permission no call needs: the OS sandbox
	// gives a program that holds it the network and
	// takes it from every other (#304).
	PermNetwork = "network"
)

// Permissions lists every permission, in the order the settings page and
// the documentation show them.
var Permissions = []string{
	PermPlaybackRead, PermPlaybackControl, PermQueueRead, PermQueueControl, PermLyricsRead,
	PermSearch, PermOpen, PermVolume, PermLike, PermLibrary, PermPrivateRead, PermNetwork,
}

// KnownPermission reports whether p is a permission of this protocol.
func KnownPermission(p string) bool { return slices.Contains(Permissions, p) }

// commandPermission is the permission each v1 command needs when an
// add-on sends it. Every command that runs in the running Sona has an
// entry (a test walks the command table); RequestPermission adds the
// verbs that need another.
var commandPermission = map[string]string{
	"play": PermPlaybackControl, "pause": PermPlaybackControl, "toggle": PermPlaybackControl,
	"next": PermPlaybackControl, "prev": PermPlaybackControl, "seek": PermPlaybackControl,
	"shuffle": PermPlaybackControl, "repeat": PermPlaybackControl, "crossfade": PermPlaybackControl,
	"status":  PermPlaybackRead,
	"queue":   PermQueueRead,
	"lyrics":  PermLyricsRead,
	"search":  PermSearch,
	"open":    PermOpen,
	"volume":  PermVolume,
	"like":    PermLike,
	"dislike": PermLike,
	"library": PermLibrary,
	"private": PermPrivateRead,
}

// RequestPermission is the one permission a v1 request needs when an
// add-on sends it: its command's, but queue.control for `queue jump` and
// `queue clear`. False when an add-on may not send the command at all.
func RequestPermission(req Request) (string, bool) {
	p, ok := commandPermission[req.Command]
	if ok && req.Command == "queue" && len(req.Args) > 0 {
		return PermQueueControl, true
	}
	return p, ok
}

// Topics an add-on may subscribe to.
const (
	// TopicStatus carries a Status on every change but a position that just
	// moves on (the position jumping, a seek, does count).
	TopicStatus = "status"
	// TopicNowPlaying carries a NowPlaying each time the song that plays
	// changes, and when nothing does.
	TopicNowPlaying = "now-playing"
	// TopicQueue carries the Queue each time it changes.
	TopicQueue = "queue"
	// TopicSettings carries the add-on's own settings rows' values
	// (SettingsValues) each time one changes. It needs no permission: the
	// values are the add-on's own.
	TopicSettings = "settings"
)

// Topics lists every topic, in documentation order.
var Topics = []string{TopicStatus, TopicNowPlaying, TopicQueue, TopicSettings}

var topicPermission = map[string]string{
	TopicStatus: PermPlaybackRead, TopicNowPlaying: PermPlaybackRead, TopicQueue: PermQueueRead, TopicSettings: "",
}

// TopicPermission is the permission a subscription to topic needs ("" for
// none); false for a topic that doesn't exist.
func TopicPermission(topic string) (string, bool) {
	p, ok := topicPermission[topic]
	return p, ok
}

// SettingsValues is the payload of TopicSettings: the value of every
// non-secret settings row the manifest declares, by row ID (a toggle is
// "true" or "false"; a row the user never set has its default). Secrets are
// not here: the add-on asks for one with the secret method.
type SettingsValues struct {
	Values map[string]string `json:"values"`
}

// SecretParams is the params of secret, and SecretResult its result.
type SecretParams struct {
	// Key is the ID of one of the add-on's own secret rows.
	Key string `json:"key"`
}

// SecretResult is what the user saved in the secret row; the method fails
// with not_found while there is none.
type SecretResult struct {
	Value string `json:"value"`
}
type NowPlaying struct {
	// Track is the song that plays; null when none does (nothing is
	// selected, or the listen is not one an add-on may know of).
	Track *Track `json:"track"`
	// StartedAt is when Sona began this listen, in Unix milliseconds; 0
	// when Track is null.
	StartedAt int64 `json:"startedAt"`
}

// Frame is one line of the channel. A request has ID and Method; its
// response has the same ID and a Result or an Error; an event has Event
// and Data and no ID. IDs are chosen by whoever sends the request; the two
// directions are separate.
type Frame struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
	Event  string          `json:"event,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// MarshalJSON writes an error frame with its id even when it is 0 (an
// error that answers no request), as the schema's ErrorResponse requires.
func (f Frame) MarshalJSON() ([]byte, error) {
	type plain Frame
	if f.Error == nil {
		return json.Marshal(plain(f))
	}
	return json.Marshal(struct {
		ID int64 `json:"id"`
		plain
	}{f.ID, plain(f)})
}

// HelloParams is the add-on's first frame.
type HelloParams struct {
	// ID is the add-on's ID: the one the supervisor started it as.
	ID       string       `json:"id"`
	Protocol VersionRange `json:"protocol"`
	// Permissions are the ones the add-on asks for now.
	Permissions []string `json:"permissions"`
}

// VersionRange is the protocol versions an add-on speaks.
type VersionRange struct {
	Min string `json:"min"`
	Max string `json:"max"`
}

// HelloResult answers a hello that was accepted.
type HelloResult struct {
	ID string `json:"id"`
	// Protocol is the version both sides speak from now on.
	Protocol string `json:"protocol"`
	// Permissions are the asked-for permissions that are granted now. The
	// grant is checked again at every call, so this is only what held at
	// hello.
	Permissions []string `json:"permissions"`
	// Limits are the session's limits.
	Limits AddonLimits `json:"limits"`
	// Credentials are the build credentials of one of Sona's own add-ons
	// (CredAPIKey and the other Cred* keys), sent in this one answer and
	// nowhere else: not in the environment, a request, an event or a log.
	// An add-on of the user's never gets the field.
	Credentials map[string]string `json:"credentials,omitempty"`
}

// Keys of HelloResult.Credentials: the Last.fm API account of the build, and
// the Discord application ID.
const (
	CredAPIKey        = "apiKey"
	CredAPISecret     = "apiSecret"
	CredApplicationID = "applicationId"
)

// TopicsParams is the params of subscribe and unsubscribe, and their
// result: the topics subscribed to after the call.
type TopicsParams struct {
	Topics []string `json:"topics"`
}

// InvokeParams is what Sona sends an add-on to run one of its
// contributions.
type InvokeParams struct {
	// Action is the contribution's ID (addon.<id>.<name>).
	Action string `json:"action"`
	// Target is what the action was chosen on (an item's kind, ID and
	// name); null for a palette command.
	Target json.RawMessage `json:"target"`
	// DeadlineMs is how long the add-on has to answer, in milliseconds. Sona
	// sets it; a late answer is dropped.
	DeadlineMs int64 `json:"deadlineMs"`
}

// InvokeResult is an add-on's answer to an invoke. Both fields are
// optional plain text.
type InvokeResult struct {
	// Toast is a short message Sona shows.
	Toast string `json:"toast,omitempty"`
	// URL is a link Sona offers to open, after the user clicks it: http or
	// https only.
	URL string `json:"url,omitempty"`
}

// Limits of a session, reported in HelloResult. A zero field of the
// session's configuration takes its default here.
type AddonLimits struct {
	// MaxFrame is the longest frame, in bytes, either way; a longer one ends
	// the session.
	MaxFrame int `json:"maxFrame"`
	// Calls is how many calls may run at once; more are refused with busy.
	Calls int `json:"calls"`
	// Invokes is how many invokes may await an answer at once.
	Invokes int `json:"invokes"`
	// Subscriptions is how many topics may be subscribed to at once, and
	// how many one subscribe may name.
	Subscriptions int `json:"subscriptions"`
	// Args and ArgBytes bound a call's arguments: their number and each one's
	// length in bytes.
	Args     int `json:"args"`
	ArgBytes int `json:"argBytes"`
	// Queue is how many frames may wait to be written to an add-on that
	// doesn't read before Sona ends the session.
	Queue int `json:"queue"`
}

// Defaults of AddonLimits.
const (
	AddonMaxFrame      = 1 << 20
	addonCalls         = 8
	addonInvokes       = 4
	addonSubscriptions = 8
	addonArgs          = 32
	addonArgBytes      = 4096
	addonQueue         = 256
	// addonMaxQueueTracks bounds the queue an add-on sees: a window around
	// the playing song.
	addonMaxQueueTracks = 1000
)

func (l AddonLimits) withDefaults() AddonLimits {
	fill := func(v *int, d int) {
		if *v <= 0 {
			*v = d
		}
	}
	fill(&l.MaxFrame, AddonMaxFrame)
	fill(&l.Calls, addonCalls)
	fill(&l.Invokes, addonInvokes)
	fill(&l.Subscriptions, addonSubscriptions)
	fill(&l.Args, addonArgs)
	fill(&l.ArgBytes, addonArgBytes)
	fill(&l.Queue, addonQueue)
	return l
}

// Rate bounds how often a call may be made: PerSecond on average, Burst at
// once.
type Rate struct {
	PerSecond float64
	Burst     int
}

// defaultRates are the rates by permission; "" is every other one. Search,
// like, library and open reach Apple or move Sona's window, so each has its
// own.
var defaultRates = map[string]Rate{
	PermSearch:  {PerSecond: 2, Burst: 5},
	PermLike:    {PerSecond: 1, Burst: 3},
	PermLibrary: {PerSecond: 1, Burst: 3},
	PermOpen:    {PerSecond: 1, Burst: 3},
	"":          {PerSecond: 20, Burst: 40},
}

// bucket is a token bucket.
type bucket struct {
	rate   Rate
	tokens float64
	last   time.Time
}

func newBucket(r Rate, now time.Time) *bucket {
	return &bucket{rate: r, tokens: float64(r.Burst), last: now}
}

func (b *bucket) take(now time.Time) bool {
	if dt := now.Sub(b.last).Seconds(); dt > 0 {
		b.tokens = min(float64(b.rate.Burst), b.tokens+dt*b.rate.PerSecond)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Error codes of protocol v2, beside the v1 and service codes that pass
// through (addonMessages lists them all). An error frame carries the code
// and its fixed message, never the text of the error that caused it.
const (
	AddonErrBadFrame       = "bad_frame"
	AddonErrFrameTooLarge  = "frame_too_large"
	AddonErrHelloRequired  = "hello_required"
	AddonErrAlreadyHello   = "already_hello"
	AddonErrBadHello       = "bad_hello"
	AddonErrVersion        = "version_unsupported"
	AddonErrUnknownMethod  = "unknown_method"
	AddonErrUnknownTopic   = "unknown_topic"
	AddonErrPermission     = "permission_denied"
	AddonErrRateLimited    = "rate_limited"
	AddonErrBusy           = "busy"
	AddonErrTooMany        = "too_many_subscriptions"
	AddonErrBadArguments   = "bad_arguments"
	AddonErrResultTooLarge = "result_too_large"
	AddonErrTimeout        = "timeout"
	AddonErrNotRunning     = "not_running"
	AddonErrFailed         = CodeFailed
)

// addonMessages are the codes an add-on sees, with their fixed English
// messages. No message is built from an error's text: that could carry a
// URL, a path or a token of what failed. The codes after the first block
// are the v1 and service codes that pass through unchanged.
var addonMessages = map[string]string{
	AddonErrBadFrame:       "the frame is not a JSON object of the protocol",
	AddonErrFrameTooLarge:  "the frame is longer than the limit",
	AddonErrHelloRequired:  "the first frame must be hello",
	AddonErrAlreadyHello:   "hello was already sent",
	AddonErrBadHello:       "hello is not valid, or names another add-on",
	AddonErrVersion:        "no protocol version both sides speak",
	AddonErrUnknownMethod:  "no such method",
	AddonErrUnknownTopic:   "no such topic",
	AddonErrPermission:     "the add-on doesn't have the permission for this",
	AddonErrRateLimited:    "too many calls; slow down",
	AddonErrBusy:           "too many calls at once",
	AddonErrTooMany:        "too many subscriptions",
	AddonErrBadArguments:   "the arguments are not valid for this command",
	AddonErrResultTooLarge: "the answer is longer than the frame limit",
	AddonErrTimeout:        "the call took too long",
	AddonErrNotRunning:     "the add-on is not running",
	AddonErrFailed:         "Sona couldn't do that",

	// The answers of Sona's own scrobbling add-ons (addon_scrobble.go).
	AddonErrCredentialsRejected:  "the service refused the credentials",
	AddonErrSubmissionRejected:   "the service refused the submission",
	AddonErrAuthorisationPending: "the user hasn't approved the authorisation yet",
	AddonErrAuthorisationExpired: "the authorisation expired before it was approved",
	AddonErrNotConfigured:        "the service isn't set up in this build",

	CodeUnknownCommand: "no such command",
	CodeNothingPlaying: "nothing is playing",
	CodeNoLyrics:       "this song has no lyrics",
	CodeBadLink:        "that isn't an Apple Music link Sona can open",
	CodeAccountLoading: "Sona is still reading the account",
	CodeStarting:       "Sona is starting",
	CodeShuttingDown:   "Sona is shutting down",
	"not_signed_in":    "this needs an Apple Music account signed in to Sona",
	"network":          "Sona couldn't reach the network",
	"unsupported":      "this isn't available on this system",
	"private_forced":   "private listening is forced on",
	"not_found":        "it couldn't be found",
	"not_playable":     "this song isn't available to stream",
	"no_subscription":  "this account has no Apple Music subscription",
	"live_station":     "not available while a live station plays",
	"station":          "not available while a station plays",
	"not_released":     "this song isn't out yet",
	"unauthorized":     "Apple Music refused the request",
	"cancelled":        "the call was cancelled",
}

// AddonErrorCodes lists every code of an error frame, sorted.
func AddonErrorCodes() []string {
	out := make([]string, 0, len(addonMessages))
	for c := range addonMessages {
		out = append(out, c)
	}
	slices.Sort(out)
	return out
}

// addonError is the error frame body for code: its fixed message.
func addonError(code string) *Error {
	return &Error{Code: code, Message: addonMessages[code]}
}

// addonCode is err as the code an add-on sees: the code of an *Error when
// the protocol words it, else "failed". Its message is dropped.
func addonCode(err error) *Error {
	e := AsError(err)
	if e.Code == CodeUsage {
		return addonError(AddonErrBadArguments)
	}
	if _, ok := addonMessages[e.Code]; ok {
		return addonError(e.Code)
	}
	return addonError(AddonErrFailed)
}

// versionExpr matches major.minor and major.minor.patch.
var versionExpr = regexp.MustCompile(`^(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})(?:\.(?:0|[1-9][0-9]{0,5}))?$`)

type version struct{ major, minor int }

func parseVersion(s string) (version, bool) {
	m := versionExpr.FindStringSubmatch(s)
	if m == nil {
		return version{}, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return version{major, minor}, true
}

func (v version) less(o version) bool {
	return v.major < o.major || (v.major == o.major && v.minor < o.minor)
}

func (v version) String() string { return strconv.Itoa(v.major) + "." + strconv.Itoa(v.minor) + ".0" }

// NegotiateVersion picks the highest protocol version both sides speak:
// the add-on's range against Sona's. It fails (false) when a version is
// not major.minor[.patch], the range is empty, or the ranges don't meet:
// an add-on whose minimum is above Sona's highest, or whose highest is
// below Sona's lowest, is refused rather than guessed at.
func NegotiateVersion(addon VersionRange) (string, bool) {
	amin, ok1 := parseVersion(addon.Min)
	amax, ok2 := parseVersion(addon.Max)
	smin, _ := parseVersion(AddonProtocolMin)
	smax, _ := parseVersion(AddonProtocolMax)
	if !ok1 || !ok2 || amax.less(amin) {
		return "", false
	}
	pick := amax
	if smax.less(pick) {
		pick = smax
	}
	if pick.less(amin) || pick.less(smin) {
		return "", false
	}
	return pick.String(), true
}

// artworkHostSuffix is Apple's public artwork CDN.
const artworkHostSuffix = ".mzstatic.com"

// PublicArtwork is u when it is an https URL on Apple's artwork CDN with
// no credentials, query or odd port; else "". Events and answers carry
// nothing else for an image: never an Apple API host.
func PublicArtwork(u string) string {
	p, err := url.Parse(strings.TrimSpace(u))
	if err != nil || p.Scheme != "https" || p.User != nil || p.RawQuery != "" || p.Fragment != "" || len(u) > 2048 {
		return ""
	}
	h := strings.ToLower(p.Hostname())
	if !strings.HasSuffix(h, artworkHostSuffix) || h == artworkHostSuffix[1:] || (p.Port() != "" && p.Port() != "443") {
		return ""
	}
	return u
}

// PublicShareURL is u when it is an https link on music.apple.com with no
// credentials or odd port, the form Apple Music shares; else "".
func PublicShareURL(u string) string {
	p, err := url.Parse(strings.TrimSpace(u))
	if err != nil || p.Scheme != "https" || p.User != nil || p.Fragment != "" || len(u) > 2048 {
		return ""
	}
	if strings.ToLower(p.Hostname()) != "music.apple.com" || (p.Port() != "" && p.Port() != "443") {
		return ""
	}
	return u
}

// MaxToast and maxResultURL bound an InvokeResult's fields.
const (
	MaxToast     = 500
	maxResultURL = 2048
)

// ParseInvokeResult reads and checks an add-on's answer to an invoke: plain
// text no longer than the caps, and a URL that is http or https without
// credentials (the user still has to click it).
func ParseInvokeResult(raw json.RawMessage) (InvokeResult, bool) {
	var r InvokeResult
	if len(raw) > 0 && string(raw) != "null" {
		if json.Unmarshal(raw, &r) != nil {
			return InvokeResult{}, false
		}
	}
	if len(r.Toast) > MaxToast || len(r.URL) > maxResultURL {
		return InvokeResult{}, false
	}
	if r.URL != "" {
		if _, ok := SafeLink(r.URL); !ok {
			return InvokeResult{}, false
		}
	}
	return r, true
}

// SafeLink reports whether raw is a link an add-on may ask Sona to offer:
// http or https with a host, no user name or password, plain ASCII with no
// control or space characters, at most 2048 bytes. It returns the link's host
// (the user is shown where the link goes). It never means the link may be
// opened unasked: it opens only when the user clicks it.
func SafeLink(raw string) (host string, ok bool) {
	if raw == "" || len(raw) > maxResultURL {
		return "", false
	}
	for _, r := range raw {
		if r <= ' ' || r >= 0x7f {
			return "", false
		}
	}
	p, err := url.Parse(raw)
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.User != nil || p.Hostname() == "" {
		return "", false
	}
	return p.Hostname(), true
}

// forAddon is v, a result or an event payload, as an add-on may see it:
// artwork only from the public CDN, links only on music.apple.com, a queue
// no longer than a frame can hold. Values of other types pass unchanged.
func forAddon(v any) any {
	switch t := v.(type) {
	case Status:
		if t.Track != nil {
			tr := publicTrack(*t.Track)
			t.Track = &tr
		}
		return t
	case Track:
		return publicTrack(t)
	case NowPlaying:
		if t.Track != nil {
			tr := publicTrack(*t.Track)
			t.Track = &tr
		}
		return t
	case Like:
		t.Track = publicTrack(t.Track)
		return t
	case Lyrics:
		t.Track = publicTrack(t.Track)
		return t
	case Queue:
		return windowQueue(t)
	case SearchResults:
		out := t
		out.Results = make([]SearchResult, len(t.Results))
		for i, r := range t.Results {
			r.URL = PublicShareURL(r.URL)
			out.Results[i] = r
		}
		return out
	case LibraryItems:
		out := t
		out.Items = make([]LibraryItem, len(t.Items))
		for i, it := range t.Items {
			it.Artwork, it.URL = PublicArtwork(it.Artwork), PublicShareURL(it.URL)
			out.Items[i] = it
		}
		return out
	}
	return v
}

func publicTrack(t Track) Track {
	t.Artwork = PublicArtwork(t.Artwork)
	return t
}

// windowQueue keeps at most addonMaxQueueTracks songs around the playing
// one; Index is its place in what is kept.
func windowQueue(q Queue) Queue {
	lo, hi := AddonQueueWindow(len(q.Tracks), q.Index)
	out := Queue{Index: q.Index, Tracks: make([]QueueTrack, 0, hi-lo)}
	if q.Index >= 0 {
		out.Index = q.Index - lo
	}
	for _, t := range q.Tracks[lo:hi] {
		t.Track = publicTrack(t.Track)
		out.Tracks = append(out.Tracks, t)
	}
	return out
}

// AddonQueueWindow is the part [lo, hi) of a queue of n songs, the one at
// index playing (-1: none), that an add-on sees: all of it, or
// addonMaxQueueTracks songs around the playing one. A place in what the
// add-on sees is lo after the same place in the queue.
func AddonQueueWindow(n, index int) (lo, hi int) {
	if n <= addonMaxQueueTracks {
		return 0, n
	}
	lo = min(max(index-addonMaxQueueTracks/4, 0), n-addonMaxQueueTracks)
	return lo, lo + addonMaxQueueTracks
}
