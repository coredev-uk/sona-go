package diag

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/coredev-uk/sona-go/internal/logging"
)

// The bundle is pasted into public issues, and a log tail is a listening
// history: which songs played, which station, what was searched for, which
// shelf of recommendations was refreshed. Anonymising works on what a value
// is, not on the messages that happen to carry it today:
//
//   - A log attribute whose key can identify content or a person (see
//     [classify]) has its value replaced by a pseudonym, "id~3fa2ab": the
//     first bytes of an HMAC of the value under a random key made for this
//     bundle and thrown away with it. The same value gives the same
//     pseudonym everywhere in the bundle, so "the same song failed twice"
//     can still be read, and a different one in the next bundle, so two
//     bundles cannot be matched. Numbers and booleans are sizes, counts,
//     durations and flags: they stay.
//   - Every string, whatever its key, goes through [anonymiser.text]: Apple
//     IDs, URLs, user paths, e-mail addresses and secrets.

// pseudonymBytes is how many bytes of the HMAC a pseudonym shows (six hex
// digits): few enough to tell nothing, enough that two songs of one bundle
// rarely share one.
const pseudonymBytes = 3

// Placeholders for what a reduced URL or path no longer says.
const (
	idMask   = "{id}"
	pathMask = "{path}"
)

// anonymiser pseudonymises with a key of its own, so the pseudonyms of one
// bundle mean nothing in another.
type anonymiser struct {
	key []byte
	// homes are the spellings of the home directory a string can hold:
	// as given, with doubled backslashes (a Windows path inside JSON text)
	// and with forward slashes.
	homes []string
}

func newAnonymiser(home string) *anonymiser {
	key := make([]byte, 32)
	// crypto/rand does not fail on the platforms Sona runs on (Go 1.24).
	_, _ = rand.Read(key)
	a := &anonymiser{key: key}
	if home = strings.TrimRight(home, `/\`); home != "" {
		a.homes = append(a.homes, home)
		if strings.Contains(home, `\`) {
			a.homes = append(a.homes, strings.ReplaceAll(home, `\`, `\\`), strings.ReplaceAll(home, `\`, "/"))
		}
	}
	return a
}

// pseudonym is label~hex of an HMAC of value. The label says what kind of
// value it was and never depends on the value.
func (a *anonymiser) pseudonym(label, value string) string {
	m := hmac.New(sha256.New, a.key)
	m.Write([]byte(value))
	return label + "~" + hex.EncodeToString(m.Sum(nil)[:pseudonymBytes])
}

// labelOf is the pseudonym label for an attribute key: its letters and
// digits, lower case, at most 16.
func labelOf(key string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(key) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			if b.Len() == 16 {
				break
			}
		}
	}
	if b.Len() == 0 {
		return "value"
	}
	return b.String()
}

// kind says how a string value is anonymised, by the key it has.
type kind uint8

const (
	plain  kind = iota // free text: the string pass only
	pseudo             // replaced whole by a pseudonym
	route              // an in-app route: the page type stays, IDs are masked
	link               // a URL (or an API path): see reduceURL
	file               // a file system path
)

// classify is the rule for attribute keys, in logs and in the sections of
// the bundle. Keys match without case, '-' and '_'. A key that can carry
// the identity of content or of a person, or what the person typed, gets a
// kind other than plain. docs/architecture.md ("Diagnostics and crashes")
// tells whoever adds a log attribute to follow this list, and
// TestEveryLogAttributeIsAudited fails when a key is in neither this rule
// nor keptLogKeys.
func classify(key string) kind {
	k := logging.NormalizeKey(key)
	switch k {
	case "route", "startpage", "lastroute":
		return route
	case "url", "href", "uri", "link", "endpoint":
		return link
	case "path", "paths", "file", "files", "dir", "directory", "cwd":
		return file
	case
		// IDs of content.
		"id", "ids", "song", "songs", "album", "albums", "artist", "artists", "composer",
		"playlist", "playlists", "station", "stations", "shelf", "shelves", "key", "keys",
		"item", "items", "folder", "folders", "curator", "track", "tracks", "video", "videos",
		// What the person typed or sees.
		"title", "name", "term", "query", "q", "search", "text", "lyric", "lyrics", "events", "body",
		// Who the person is, and the hardware.
		"user", "owner", "account", "accountname", "appleid", "dsid", "email", "username", "nickname",
		"handle", "profile", "listener", "device", "sink":
		return pseudo
	}
	switch {
	case strings.HasPrefix(k, "catalog"), strings.HasPrefix(k, "library"):
		return pseudo
	case strings.HasSuffix(k, "route"):
		return route
	case strings.HasSuffix(k, "url"), strings.HasSuffix(k, "href"), strings.HasSuffix(k, "uri"):
		return link
	case strings.HasSuffix(k, "path"), strings.HasSuffix(k, "file"), strings.HasSuffix(k, "dir"):
		return file
	case strings.HasSuffix(k, "id"), strings.HasSuffix(k, "ids"), strings.HasSuffix(k, "title"), strings.HasSuffix(k, "name"):
		return pseudo
	}
	return plain
}

// value anonymises one string under key.
func (a *anonymiser) value(key, s string) string {
	if s == "" {
		return s
	}
	switch classify(key) {
	case pseudo:
		return a.pseudonym(labelOf(key), s)
	case route:
		if !strings.HasPrefix(s, "/") {
			return a.text(s)
		}
		return a.text(maskRoute(s))
	case link:
		if strings.Contains(s, "://") || strings.HasPrefix(s, "/") {
			return a.text(s)
		}
		return a.pseudonym(labelOf(key), s)
	case file:
		return maskPathValue(a.text(s))
	}
	return a.text(s)
}

var (
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	// An http(s) URL in text; it ends at a quote, space or backslash.
	urlRe = regexp.MustCompile("https?://[^\\s\"'<>`\\\\]+")
	// A bare API path ("/v1/me/recommendations/…") with its query.
	apiPathRe = regexp.MustCompile("(^|[^A-Za-z0-9_./~-])(/v1/[^\\s\"'<>`\\\\?#]*)([?#][^\\s\"'<>`\\\\]*)?")
	// Another user's home directory (the one running Sona is replaced
	// exactly).
	otherHomeRe = regexp.MustCompile(`(?:/home|/Users)/[^/\s"'<>:\\]+|[A-Za-z]:\\{1,2}Users\\{1,2}[^\\/\s"'<>:]+`)
	// A path to a media file or a lyrics file, whatever its folders:
	// the file name is a song title.
	mediaPathRe = regexp.MustCompile(`(?i)(?:^|[\s"'=(\[,:])((?:~|[A-Za-z]:)?(?:[/\\][^/\\"'<>|*?\n:]+)+?\.(?:mp3|m4a|m4b|m4p|m4v|aac|flac|alac|wav|aiff?|ogg|oga|opus|wma|mp4|mov|mkv|webm|lrc|ttml|cue))\b`)
	// What the person keeps in their home folder: the folder named by the
	// platform for it, and what is below it.
	userRootRe = regexp.MustCompile(`(?i)~[/\\](?:Music|Movies|Videos|Pictures|Documents|Downloads|Desktop|Public|iTunes|Dropbox|OneDrive|iCloud Drive)[/\\][^"'<>|*?\n:]+`)
	// Apple catalogue and library IDs: i.…, l.…, p.…, pl.…, pl.u-…, mv.…,
	// ra.…, sp.…, r.…. The token must look like an ID (see isAppleID), so
	// "r.go" and "p.Name()" in a stack trace stay.
	appleIDRe = regexp.MustCompile(`(^|[^A-Za-z0-9_.])((?:pl|mv|ra|sp|i|l|p|r)\.[A-Za-z0-9][A-Za-z0-9_-]{3,})`)
	// A bare catalogue ID: six or more digits after a word that says what
	// they are (song 1234567, id=1234567, /albums/1234567, adamId: 1234567).
	numericIDRe = regexp.MustCompile(`(?i)\b(\w*id|ids|songs?|albums?|artists?|playlists?|tracks?|stations?|videos?|curators?|catalog|library|webPlayback|playback|items?|resources?|lyrics|rooms?|groupings?|charts?|ratings?|contents?|entry|adam)(["']?(?:\s*[:=]\s*|\s+|/)["']?)(\d{6,}(?:[,\s]+\d{6,})*)\b`)
	// A path segment of six or more digits is an ID in Apple's paths
	// ("/rooms/12345678").
	pathNumRe = regexp.MustCompile(`(/)(\d{6,})\b`)
	digitsRe  = regexp.MustCompile(`\d{6,}`)
)

// text is the string pass: what is removed from every string of the bundle,
// whatever its key and whichever line it comes from. It is idempotent.
func (a *anonymiser) text(s string) string {
	if s == "" {
		return s
	}
	s = urlRe.ReplaceAllStringFunc(s, reduceURLText)
	s = maskAPIPaths(s)
	s = logging.Redact(s)
	s = emailRe.ReplaceAllString(s, logging.Redacted)
	for _, h := range a.homes {
		s = strings.ReplaceAll(s, h, "~")
	}
	s = otherHomeRe.ReplaceAllString(s, "~")
	s = userRootRe.ReplaceAllStringFunc(s, func(m string) string {
		return m[:strings.IndexAny(m[2:], `/\`)+2] + "/" + pathMask
	})
	s = mediaPathRe.ReplaceAllStringFunc(s, func(m string) string {
		path := mediaPathRe.FindStringSubmatch(m)[1]
		return m[:len(m)-len(path)] + pathMask + path[strings.LastIndexByte(path, '.'):]
	})
	s = a.appleIDs(s)
	s = a.numericIDs(s)
	s = pathNumRe.ReplaceAllStringFunc(s, func(m string) string { return "/" + a.pseudonym("id", m[1:]) })
	return s
}

// appleIDs pseudonymises the Apple ID shapes in s.
func (a *anonymiser) appleIDs(s string) string {
	idx := appleIDRe.FindAllStringSubmatchIndex(s, -1)
	if len(idx) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range idx {
		id := s[m[4]:m[5]]
		if !isAppleID(id, s[m[5]:]) {
			continue
		}
		b.WriteString(s[last:m[4]])
		b.WriteString(a.pseudonym("id", id))
		last = m[5]
	}
	b.WriteString(s[last:])
	return b.String()
}

// isAppleID tells an ID from code that looks like one ("r.Body.Close()",
// "p.cmd"): the part after the prefix has a digit, or a lower case letter
// with an upper case one after its first character ("p.vQDQYvBsRK"). rest
// is what follows the token.
func isAppleID(id, rest string) bool {
	if rest != "" {
		if rest[0] == '(' {
			return false
		}
		if rest[0] == '.' && len(rest) > 1 && unicode.IsLetter(rune(rest[1])) {
			return false
		}
	}
	token := id[strings.IndexByte(id, '.')+1:]
	var digit, lower, innerUpper bool
	for i, r := range token {
		switch {
		case r >= '0' && r <= '9':
			digit = true
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			innerUpper = innerUpper || i > 0
		}
	}
	return digit || (lower && innerUpper)
}

// numericIDs pseudonymises bare catalogue IDs after a word naming them.
func (a *anonymiser) numericIDs(s string) string {
	return numericIDRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := numericIDRe.FindStringSubmatch(m)
		switch strings.ToLower(sub[1]) {
		case "pid", "ppid", "uid", "gid", "tid", "euid", "egid":
			return m
		}
		return sub[1] + sub[2] + digitsRe.ReplaceAllStringFunc(sub[3], func(d string) string { return a.pseudonym("id", d) })
	})
}

// Hosts of Apple: the catalogue and library API, artwork, streams.
var (
	appleDomains = []string{"apple.com", "mzstatic.com", "cdn-apple.com", "aaplimg.com"}
	// appleWebHosts serve pages, whose paths hold slugs made of titles. The
	// API ("amp-api.music.apple.com") is not one of them.
	appleWebHosts = []string{"music.apple.com", "beta.music.apple.com", "classical.music.apple.com", "embed.music.apple.com", "itunes.apple.com"}
	wordRe        = regexp.MustCompile(`^[A-Za-z]+(?:[-_][A-Za-z]+)*$`)
	versionRe     = regexp.MustCompile(`^v\d{1,2}$`)
	sizeRe        = regexp.MustCompile(`^(?:\d+|\{w\})x(?:\d+|\{h\})[A-Za-z-]*(?:\.[A-Za-z0-9]+)?$`)
)

func hostIn(host string, domains ...string) bool {
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// keepSegment reports whether a path segment is a word of an endpoint or a
// page ("recommendations", "top-songs", "gb", "v1"), not an ID, a slug with a
// title in it or a hash.
func keepSegment(seg string) bool {
	switch {
	case seg == "" || seg == idMask || seg == pathMask:
		return true
	case versionRe.MatchString(seg):
		return true
	case !wordRe.MatchString(seg):
		return false
	}
	return len(seg) <= 24 && (len(seg) <= 20 || strings.ContainsAny(seg, "-_"))
}

// maskSegments keeps the segments of a path that keepSegment accepts
// and replaces the rest by {id}. from is the first segment (counted from 0)
// that is replaced whatever it is; sizes keeps artwork size segments.
func maskSegments(path string, from int, sizes bool) string {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	out := segs[:0]
	for i, s := range segs {
		if (i >= from || !keepSegment(s)) && (!sizes || !sizeRe.MatchString(s)) {
			s = idMask
		}
		if s == idMask && len(out) > 0 && out[len(out)-1] == idMask {
			continue
		}
		out = append(out, s)
	}
	return "/" + strings.Join(out, "/")
}

// reduceURL cuts an http(s) URL down to what a bug report needs: scheme,
// host and the shape of the path. The query, the fragment and any user info
// go (a query holds search terms, IDs and credentials). On Apple's hosts
// IDs and slugs in the path become {id}, so "/v1/me/recommendations/{id}"
// still says which endpoint was called. On music.apple.com a page's slug is
// a title: only the storefront and the type stay. Elsewhere only the first
// segment of the path stays.
func reduceURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		if i := strings.IndexAny(raw, "?#"); i >= 0 {
			raw = raw[:i]
		}
		return raw
	}
	host := strings.ToLower(u.Hostname())
	path := u.Path
	switch {
	case slices.Contains(appleWebHosts, host):
		path = maskSegments(path, 2, false)
	case hostIn(host, "mzstatic.com"):
		path = maskSegments(path, 2, true)
	case hostIn(host, appleDomains...):
		path = maskSegments(path, 1<<30, false)
	default:
		segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
		switch {
		case len(segs) > 1 && keepSegment(segs[0]):
			path = "/" + segs[0] + "/…"
		case len(segs) > 1:
			path = "/…"
		case segs[0] != "" && !keepSegment(segs[0]):
			path = "/…"
		}
	}
	if path == "/" {
		path = ""
	}
	return u.Scheme + "://" + strings.ToLower(u.Host) + path
}

// reduceURLText is reduceURL for a URL found in text: trailing punctuation
// is not part of the URL.
func reduceURLText(m string) string {
	rest := strings.TrimRight(m, ".,;:!?)]")
	return reduceURL(rest) + m[len(rest):]
}

// maskAPIPaths masks the IDs of bare API paths ("/v1/…") in s and drops
// their queries.
func maskAPIPaths(s string) string {
	return apiPathRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := apiPathRe.FindStringSubmatch(m)
		return sub[1] + maskSegments(sub[2], 1<<30, false)
	})
}

// maskRoute keeps the page type of an in-app route and masks the rest:
// "/playlist/p.abc123?song=4" is "/playlist/{id}". The query goes.
func maskRoute(r string) string {
	if i := strings.IndexAny(r, "?#"); i >= 0 {
		r = r[:i]
	}
	if r == "" || r == "/" {
		return r
	}
	return maskSegments(r, 1<<30, false)
}

// systemPaths are places a path value may stay under: the operating
// system's own, which no person's files are in.
var systemPaths = []string{
	"/usr/", "/opt/", "/etc/", "/var/", "/tmp/", "/proc/", "/sys/", "/dev/", "/nix/", "/snap/", "/app/", "/run/user/",
	"/Applications/", "/System/", "/Library/", `c:\windows`, `c:\program files`, `c:\programdata`,
}

// maskPathValue reduces a value under a path key. What starts with "~" is
// the home directory's, the text pass handles it. Another place is kept
// when it is the operating system's, and otherwise its first folder stays
// and what is below it goes: "/mnt/media/…" says it is a mounted disk.
func maskPathValue(p string) string {
	if p == "" || strings.HasPrefix(p, "~") || strings.Contains(p, pathMask) {
		return p
	}
	lower := strings.ToLower(p)
	for _, sys := range systemPaths {
		if strings.HasPrefix(lower, strings.ToLower(sys)) {
			return p
		}
	}
	prefix := ""
	switch {
	case len(p) > 2 && p[1] == ':' && (p[2] == '\\' || p[2] == '/'):
		prefix, p = p[:3], p[3:]
	case p[0] == '/' || p[0] == '\\':
		prefix, p = p[:1], p[1:]
	default:
		return pathMask
	}
	i := strings.IndexAny(p, `/\`)
	if i < 0 {
		return prefix + pathMask
	}
	return prefix + p[:i] + "/" + pathMask
}

// jsonLine anonymises a log line that is one JSON object. It keeps the
// order and spelling of the keys, so the line reads as it was written. ok
// is false for text that is not a JSON object.
func (a *anonymiser) jsonLine(line string) (string, bool) {
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	tok, err := dec.Token()
	if d, isDelim := tok.(json.Delim); err != nil || !isDelim || d != '{' {
		return "", false
	}
	var b bytes.Buffer
	if err := a.object(dec, &b, ""); err != nil {
		return "", false
	}
	if _, err := dec.Token(); err != io.EOF {
		return "", false
	}
	return b.String(), true
}

// object copies the members of the object whose '{' dec has read to b.
func (a *anonymiser) object(dec *json.Decoder, b *bytes.Buffer, parent string) error {
	b.WriteByte('{')
	for first := true; dec.More(); first = false {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		if !first {
			b.WriteByte(',')
		}
		writeString(b, key)
		b.WriteByte(':')
		if logging.SensitiveKey(key) {
			var skipped json.RawMessage
			if err := dec.Decode(&skipped); err != nil {
				return err
			}
			writeString(b, logging.Redacted)
			continue
		}
		// What an object under an identifying key holds is as identifying
		// as the key: a member with no kind of its own takes the parent's.
		eff := key
		if parent != "" && classify(key) == plain {
			eff = parent
		}
		if err := a.member(dec, b, eff); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	b.WriteByte('}')
	return nil
}

// member copies one value of an object member, or array element, under key.
func (a *anonymiser) member(dec *json.Decoder, b *bytes.Buffer, key string) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch t := tok.(type) {
	case json.Delim:
		if t == '{' {
			return a.object(dec, b, key)
		}
		b.WriteByte('[')
		for first := true; dec.More(); first = false {
			if !first {
				b.WriteByte(',')
			}
			if err := a.member(dec, b, key); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil {
			return err
		}
		b.WriteByte(']')
	case string:
		writeString(b, a.value(key, t))
	case json.Number:
		b.WriteString(t.String())
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case nil:
		b.WriteString("null")
	}
	return nil
}

// writeString writes s as a JSON string without HTML escaping, as slog's
// JSON handler does.
func writeString(b *bytes.Buffer, s string) {
	enc := json.NewEncoder(b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	b.Truncate(b.Len() - 1) // Encode's newline
}

// logLine anonymises one line of the log tail: a JSON record by its keys, and
// then every string of it; anything else (a crash dump, a line an older
// build wrote) by the string pass.
func (a *anonymiser) logLine(line string) string {
	if t := strings.TrimSpace(line); strings.HasPrefix(t, "{") {
		if out, ok := a.jsonLine(t); ok {
			return out
		}
	}
	return a.text(line)
}
