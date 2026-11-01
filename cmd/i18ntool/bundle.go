package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Apple Music on the web (music.apple.com) is a Vite app: its index script
// maps each locale's `locales/<tag>/translations.json` to a hashed chunk
// (`./translations~<hash>.js`), a JS module whose default export is the
// locale's strings. The chunk names change with every Apple release, so
// they are found from the current index script on every run.
const appleOrigin = "https://music.apple.com"

// The page whose HTML names the index script. Any page does; a catalog
// page needs no account.
const appleIndexPage = appleOrigin + "/gb/new"

// Apple serves an empty page to clients without a browser user agent.
const userAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0"

// appleSourceLocale is the bundle the mapping is made and checked against.
const appleSourceLocale = "en-gb"

// appleBundle is one locale's strings: key → text.
type appleBundle map[string]string

type fetcher struct {
	client *http.Client
}

func newFetcher() *fetcher {
	return &fetcher{client: &http.Client{Timeout: 60 * time.Second}}
}

func (f *fetcher) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
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

// discoverBundles returns each locale's bundle URL (Apple's lower-case tag →
// URL) from the current index script.
func (f *fetcher) discoverBundles(ctx context.Context) (map[string]string, error) {
	html, err := f.get(ctx, appleIndexPage)
	if err != nil {
		return nil, err
	}
	m := indexScriptRe.FindSubmatch(html)
	if m == nil {
		return nil, fmt.Errorf("no index script in %s", appleIndexPage)
	}
	js, err := f.get(ctx, appleOrigin+string(m[1]))
	if err != nil {
		return nil, err
	}
	urls := localeBundles(js)
	if len(urls) == 0 {
		return nil, fmt.Errorf("no locale bundles in %s%s", appleOrigin, m[1])
	}
	if _, ok := urls[appleSourceLocale]; !ok {
		return nil, fmt.Errorf("no %s bundle in %s%s", appleSourceLocale, appleOrigin, m[1])
	}
	return urls, nil
}

// localeBundles finds the locale → chunk URL table in an index script.
func localeBundles(js []byte) map[string]string {
	urls := map[string]string{}
	for _, m := range localeChunkRe.FindAllSubmatch(js, -1) {
		urls[strings.ToLower(string(m[1]))] = appleOrigin + "/assets/" + string(m[2])
	}
	return urls
}

func (f *fetcher) bundle(ctx context.Context, url string) (appleBundle, error) {
	js, err := f.get(ctx, url)
	if err != nil {
		return nil, err
	}
	b, err := parseBundleModule(string(js))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	return b, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// parseBundleModule reads a minified Vite JSON chunk without running it:
//
//	const e="Back",i="Done",o={"ASE.Web.Music.X":"Text",Back:e};export{e as Back,o as default};
//
// Constants hold strings; the default export is an object of string keys
// (quoted or bare) whose values are strings or constants.
func parseBundleModule(src string) (appleBundle, error) {
	p := &jsParser{src: src}
	consts := map[string]string{}
	var def string
	objects := map[string]appleBundle{}
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

func (p *jsParser) object(consts map[string]string) (appleBundle, error) {
	if err := p.expect('{'); err != nil {
		return nil, err
	}
	obj := appleBundle{}
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
