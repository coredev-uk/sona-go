// Package applestrings brings Apple Music's own translations into Sona
// (docs/ui-architecture.md, "Strings and locales"): it reads Apple's
// web string bundles, converts the strings mapped to Sona's messages
// (apple-keys.json) into Lingui (ICU) messages, and, at run time, keeps
// the converted messages for the user's locales in Sona's cache directory
// (Store). Apple's text is never committed or bundled: the app fetches it
// from music.apple.com, like the Widevine CDM from Google.
//
// Apple Music on the web (music.apple.com) is a Vite app: its index script
// maps each locale's `locales/<tag>/translations.json` to a hashed chunk
// (`./translations~<hash>.js`), a JS module whose default export is the
// locale's strings. The chunk names change with every Apple release, so
// they are found from the current index script.
package applestrings

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Origin is Apple Music on the web.
const Origin = "https://music.apple.com"

// indexPath is the page whose HTML names the index script: any page does;
// this one needs no account and is small (about 10 kB compressed).
const indexPath = "/us/home"

// UserAgent is a browser's: Apple serves an empty page to other clients.
const UserAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0"

// SourceLocale is the bundle the mapping is made and checked against.
const SourceLocale = "en-gb"

// Size caps on Apple's responses (decompressed). Today the page is about
// 75 kB, the index script 0.9 MB and a locale's bundle 0.1 MB.
const (
	maxPageBytes   = 4 << 20
	maxScriptBytes = 16 << 20
	maxBundleBytes = 4 << 20
)

// ErrTooLarge is a response over its size cap.
var ErrTooLarge = errors.New("response too large")

// Bundle is one locale's strings: key → text.
type Bundle map[string]string

// Fetcher downloads Apple's web assets: plain GETs with a browser user
// agent and no credentials, like the web player's own page loads.
type Fetcher struct {
	HTTP *http.Client
	// Origin replaces music.apple.com (tests).
	Origin string
}

func (f *Fetcher) origin() string {
	if f.Origin != "" {
		return f.Origin
	}
	return Origin
}

func (f *Fetcher) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("GET %s: %w (over %d bytes)", url, ErrTooLarge, limit)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("GET %s: empty body", url)
	}
	return b, nil
}

var (
	indexScriptRe = regexp.MustCompile(`src="(/assets/index~[0-9a-f]+\.js)"`)
	localeChunkRe = regexp.MustCompile(`"[^"]*/locales/([A-Za-z0-9-]+)/translations\.json"\s*:\s*\(\)\s*=>[^"]*"\./(translations~[0-9a-f]+\.js)"`)
)

// IndexScript returns the URL of the web app's current index script.
func (f *Fetcher) IndexScript(ctx context.Context) (string, error) {
	page := f.origin() + indexPath
	html, err := f.get(ctx, page, maxPageBytes)
	if err != nil {
		return "", err
	}
	m := indexScriptRe.FindSubmatch(html)
	if m == nil {
		return "", fmt.Errorf("no index script in %s", page)
	}
	return f.origin() + string(m[1]), nil
}

// Locales returns each locale's bundle URL (Apple's lower-case tag → URL)
// from an index script.
func (f *Fetcher) Locales(ctx context.Context, script string) (map[string]string, error) {
	js, err := f.get(ctx, script, maxScriptBytes)
	if err != nil {
		return nil, err
	}
	urls := localeBundles(js, f.origin())
	if len(urls) == 0 {
		return nil, fmt.Errorf("no locale bundles in %s", script)
	}
	return urls, nil
}

// Discover returns each locale's bundle URL from the current index
// script, which must have the source locale's.
func (f *Fetcher) Discover(ctx context.Context) (map[string]string, error) {
	script, err := f.IndexScript(ctx)
	if err != nil {
		return nil, err
	}
	urls, err := f.Locales(ctx, script)
	if err != nil {
		return nil, err
	}
	if _, ok := urls[SourceLocale]; !ok {
		return nil, fmt.Errorf("no %s bundle in %s", SourceLocale, script)
	}
	return urls, nil
}

// localeBundles finds the locale → chunk URL table in an index script.
func localeBundles(js []byte, origin string) map[string]string {
	urls := map[string]string{}
	for _, m := range localeChunkRe.FindAllSubmatch(js, -1) {
		urls[strings.ToLower(string(m[1]))] = origin + "/assets/" + string(m[2])
	}
	return urls
}

// Bundle downloads and parses a locale's bundle.
func (f *Fetcher) Bundle(ctx context.Context, url string) (Bundle, error) {
	js, err := f.get(ctx, url, maxBundleBytes)
	if err != nil {
		return nil, err
	}
	b, err := ParseBundleModule(string(js))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	return b, nil
}

// ParseBundleModule reads a minified Vite JSON chunk without running it:
//
//	const e="Back",i="Done",o={"ASE.Web.Music.X":"Text",Back:e};export{e as Back,o as default};
//
// Constants hold strings; the default export is an object of string keys
// (quoted or bare) whose values are strings or constants. Anything else
// (calls, template substitutions, numbers) is refused.
func ParseBundleModule(src string) (Bundle, error) {
	p := &jsParser{src: src}
	consts := map[string]string{}
	var def string
	objects := map[string]Bundle{}
	for {
		p.space()
		if p.eof() {
			break
		}
		switch {
		case p.word("const"), p.word("var"), p.word("let"):
			for {
				name, err := p.ident()
				if err != nil {
					return nil, err
				}
				if err := p.expect('='); err != nil {
					return nil, err
				}
				p.space()
				if p.peek() == '{' {
					obj, err := p.object(consts)
					if err != nil {
						return nil, err
					}
					objects[name] = obj
				} else {
					v, err := p.value(consts)
					if err != nil {
						return nil, err
					}
					consts[name] = v
				}
				p.space()
				if p.peek() == ',' {
					p.pos++
					continue
				}
				break
			}
			p.space()
			if p.peek() == ';' {
				p.pos++
			}
		case p.word("export"):
			if err := p.expect('{'); err != nil {
				return nil, err
			}
			for {
				p.space()
				if p.peek() == '}' {
					p.pos++
					break
				}
				local, err := p.ident()
				if err != nil {
					return nil, err
				}
				exported := local
				p.space()
				if p.word("as") {
					if exported, err = p.ident(); err != nil {
						return nil, err
					}
				}
				if exported == "default" {
					def = local
				}
				p.space()
				if p.peek() == ',' {
					p.pos++
				}
			}
			p.space()
			if p.peek() == ';' {
				p.pos++
			}
		default:
			return nil, p.errorf("unexpected input")
		}
	}
	obj, ok := objects[def]
	if !ok {
		return nil, fmt.Errorf("no default-exported object")
	}
	return obj, nil
}

type jsParser struct {
	src string
	pos int
}

func (p *jsParser) eof() bool { return p.pos >= len(p.src) }

func (p *jsParser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}

func (p *jsParser) errorf(format string, args ...any) error {
	end := min(p.pos+40, len(p.src))
	return fmt.Errorf("offset %d near %q: %s", p.pos, p.src[p.pos:end], fmt.Sprintf(format, args...))
}

func (p *jsParser) space() {
	for !p.eof() && strings.IndexByte(" \t\r\n", p.peek()) >= 0 {
		p.pos++
	}
}

func isIdentByte(c byte, first bool) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || !first && c >= '0' && c <= '9'
}

// word consumes the keyword w when it stands alone.
func (p *jsParser) word(w string) bool {
	p.space()
	if !strings.HasPrefix(p.src[p.pos:], w) {
		return false
	}
	end := p.pos + len(w)
	if end < len(p.src) && isIdentByte(p.src[end], false) {
		return false
	}
	p.pos = end
	return true
}

func (p *jsParser) ident() (string, error) {
	p.space()
	start := p.pos
	for !p.eof() && isIdentByte(p.peek(), p.pos == start) {
		p.pos++
	}
	if p.pos == start {
		return "", p.errorf("identifier expected")
	}
	return p.src[start:p.pos], nil
}

func (p *jsParser) expect(c byte) error {
	p.space()
	if p.peek() != c {
		return p.errorf("%q expected", c)
	}
	p.pos++
	return nil
}

// value is a string literal or a constant's name.
func (p *jsParser) value(consts map[string]string) (string, error) {
	p.space()
	if c := p.peek(); c == '"' || c == '\'' || c == '`' {
		return p.str()
	}
	name, err := p.ident()
	if err != nil {
		return "", err
	}
	v, ok := consts[name]
	if !ok {
		return "", p.errorf("unknown constant %s", name)
	}
	return v, nil
}

func (p *jsParser) object(consts map[string]string) (Bundle, error) {
	if err := p.expect('{'); err != nil {
		return nil, err
	}
	obj := Bundle{}
	for {
		p.space()
		if p.peek() == '}' {
			p.pos++
			return obj, nil
		}
		var key string
		var err error
		if c := p.peek(); c == '"' || c == '\'' {
			key, err = p.str()
		} else {
			key, err = p.ident()
		}
		if err != nil {
			return nil, err
		}
		p.space()
		if p.peek() == ':' {
			p.pos++
			v, err := p.value(consts)
			if err != nil {
				return nil, err
			}
			obj[key] = v
		} else { // shorthand {Back}
			v, ok := consts[key]
			if !ok {
				return nil, p.errorf("unknown constant %s", key)
			}
			obj[key] = v
		}
		p.space()
		switch p.peek() {
		case ',':
			p.pos++
		case '}':
		default:
			return nil, p.errorf("',' or '}' expected")
		}
	}
}

// str reads a JS string literal (no template substitutions).
func (p *jsParser) str() (string, error) {
	q := p.peek()
	p.pos++
	var b strings.Builder
	for {
		if p.eof() {
			return "", p.errorf("unterminated string")
		}
		c := p.src[p.pos]
		switch {
		case c == q:
			p.pos++
			return b.String(), nil
		case q == '`' && c == '$' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '{':
			return "", p.errorf("template substitution")
		case c == '\\':
			p.pos++
			if p.eof() {
				return "", p.errorf("unterminated escape")
			}
			if err := p.escape(&b); err != nil {
				return "", err
			}
		default:
			r, n := utf8.DecodeRuneInString(p.src[p.pos:])
			b.WriteRune(r)
			p.pos += n
		}
	}
}

func (p *jsParser) escape(b *strings.Builder) error {
	c := p.src[p.pos]
	p.pos++
	switch c {
	case 'n':
		b.WriteByte('\n')
	case 't':
		b.WriteByte('\t')
	case 'r':
		b.WriteByte('\r')
	case 'b':
		b.WriteByte('\b')
	case 'f':
		b.WriteByte('\f')
	case 'v':
		b.WriteByte('\v')
	case '0':
		b.WriteByte(0)
	case '\n':
		// line continuation
	case 'x':
		return p.hexRune(b, 2)
	case 'u':
		if p.peek() == '{' {
			end := strings.IndexByte(p.src[p.pos:], '}')
			if end < 0 {
				return p.errorf("bad \\u{} escape")
			}
			n, err := strconv.ParseUint(p.src[p.pos+1:p.pos+end], 16, 32)
			if err != nil {
				return p.errorf("bad \\u{} escape")
			}
			b.WriteRune(rune(n))
			p.pos += end + 1
			return nil
		}
		return p.hexRune(b, 4)
	default:
		r, n := utf8.DecodeRuneInString(p.src[p.pos-1:])
		b.WriteRune(r)
		p.pos += n - 1
	}
	return nil
}

// hexRune reads n hex digits; a UTF-16 surrogate pair (\uD83C\uDFB5) joins.
func (p *jsParser) hexRune(b *strings.Builder, n int) error {
	if p.pos+n > len(p.src) {
		return p.errorf("short escape")
	}
	v, err := strconv.ParseUint(p.src[p.pos:p.pos+n], 16, 32)
	if err != nil {
		return p.errorf("bad escape")
	}
	p.pos += n
	r := rune(v)
	if n == 4 && r >= 0xD800 && r < 0xDC00 && strings.HasPrefix(p.src[p.pos:], `\u`) {
		lo, err := strconv.ParseUint(p.src[p.pos+2:min(p.pos+6, len(p.src))], 16, 32)
		if err == nil && lo >= 0xDC00 && lo < 0xE000 {
			r = (r-0xD800)<<10 + (rune(lo) - 0xDC00) + 0x10000
			p.pos += 6
		}
	}
	b.WriteRune(r)
	return nil
}
