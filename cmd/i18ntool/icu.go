package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// A token is literal text or a placeholder. In a plural branch the plural
// value (ICU's `#`) is the placeholder named pluralValue.
type token struct {
	Text        string
	Placeholder string
	IsArg       bool
}

const pluralValue = "#"

func lit(s string) token { return token{Text: s} }
func arg(n string) token { return token{Placeholder: n, IsArg: true} }
func (t token) String() string {
	if t.IsArg {
		return "{" + t.Placeholder + "}"
	}
	return t.Text
}

// message is a Sona or Apple message reduced to what matching compares: a
// plain text, or a plural over one argument with a text per CLDR category.
type message struct {
	Plural string             // the plural argument ("" for plain text)
	Forms  map[string][]token // category ("" for plain text) → tokens
}

func (m message) isPlural() bool { return m.Plural != "" }

// pluralCategories are CLDR's, the suffixes Apple's keys use.
var pluralCategories = []string{"zero", "one", "two", "few", "many", "other"}

// placeholders lists a message's argument names (the plural argument
// included), each once.
func (m message) placeholders() []string {
	var names []string
	if m.isPlural() {
		names = append(names, m.Plural)
	}
	for _, c := range m.categories() {
		for _, t := range m.Forms[c] {
			if t.IsArg && t.Placeholder != pluralValue && !slices.Contains(names, t.Placeholder) {
				names = append(names, t.Placeholder)
			}
		}
	}
	return names
}

func (m message) categories() []string {
	if !m.isPlural() {
		return []string{""}
	}
	var cs []string
	for _, c := range pluralCategories {
		if _, ok := m.Forms[c]; ok {
			cs = append(cs, c)
		}
	}
	return cs
}

var simpleArgRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// parseSona reads a Lingui message (ICU MessageFormat) when it is plain text
// with simple `{name}` arguments, or one plural argument spanning the whole
// message whose branches are such text. Anything else (selects, several
// plurals, text around a plural, `=n` branches) cannot map to one Apple
// key, so ok is false.
func parseSona(id string) (m message, ok bool) {
	if name, body, isPlural := cutPlural(id); isPlural {
		forms, ok := parseBranches(body)
		if !ok {
			return message{}, false
		}
		if _, ok := forms["other"]; !ok {
			return message{}, false
		}
		return message{Plural: name, Forms: forms}, true
	}
	toks, ok := parseText(id, false)
	if !ok {
		return message{}, false
	}
	return message{Forms: map[string][]token{"": toks}}, true
}

// cutPlural splits "{n, plural, one {...} other {...}}" when the plural is
// the whole message.
func cutPlural(s string) (name, body string, ok bool) {
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return "", "", false
	}
	if end := matchBrace(s, 0); end != len(s)-1 {
		return "", "", false
	}
	inner := s[1 : len(s)-1]
	parts := strings.SplitN(inner, ",", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[1]) != "plural" {
		return "", "", false
	}
	name = strings.TrimSpace(parts[0])
	if !simpleArgRe.MatchString(name) {
		return "", "", false
	}
	return name, parts[2], true
}

// matchBrace returns the index of the brace closing s[open].
func matchBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parseBranches(body string) (map[string][]token, bool) {
	forms := map[string][]token{}
	s := strings.TrimSpace(body)
	for s != "" {
		open := strings.IndexByte(s, '{')
		if open < 0 {
			return nil, false
		}
		sel := strings.TrimSpace(s[:open])
		if !slices.Contains(pluralCategories, sel) {
			return nil, false // offset:, =n branches
		}
		end := matchBrace(s, open)
		if end < 0 {
			return nil, false
		}
		toks, ok := parseText(s[open+1:end], true)
		if !ok {
			return nil, false
		}
		forms[sel] = toks
		s = strings.TrimSpace(s[end+1:])
	}
	return forms, len(forms) > 0
}

// parseText splits text with simple `{name}` arguments (and `#` in a plural
// branch). ICU quoting with apostrophes is not used in Sona's messages; a
// message using it does not match.
func parseText(s string, inPlural bool) ([]token, bool) {
	var toks []token
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			toks = append(toks, lit(b.String()))
			b.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '{':
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				return nil, false
			}
			name := s[i+1 : i+end]
			if !simpleArgRe.MatchString(name) {
				return nil, false
			}
			flush()
			toks = append(toks, arg(name))
			i += end
		case c == '}':
			return nil, false
		case c == '#' && inPlural:
			flush()
			toks = append(toks, arg(pluralValue))
		case c == '\'' && i+1 < len(s) && strings.IndexByte("{}#'", s[i+1]) >= 0:
			return nil, false
		default:
			b.WriteByte(c)
		}
	}
	flush()
	return toks, true
}

var applePlaceholderRe = regexp.MustCompile(`@@([A-Za-z0-9_]+)@@`)

// parseAppleText splits Apple's text at its `@@name@@` placeholders.
func parseAppleText(s string) []token {
	var toks []token
	last := 0
	for _, m := range applePlaceholderRe.FindAllStringSubmatchIndex(s, -1) {
		if m[0] > last {
			toks = append(toks, lit(s[last:m[0]]))
		}
		toks = append(toks, arg(s[m[2]:m[3]]))
		last = m[1]
	}
	if last < len(s) {
		toks = append(toks, lit(s[last:]))
	}
	return toks
}

// splitPluralKey cuts a CLDR category suffix off an Apple key
// ("ASE.Web.Music.NoOfSongs.one" → "ASE.Web.Music.NoOfSongs", "one").
func splitPluralKey(key string) (base, category string, ok bool) {
	i := strings.LastIndexByte(key, '.')
	if i < 0 || !slices.Contains(pluralCategories, key[i+1:]) {
		return key, "", false
	}
	return key[:i], key[i+1:], true
}

// appleMessage reads key from a bundle: the plain string, or, for a plural
// mapping, every category form of the key (`.other` required).
func appleMessage(b appleBundle, key string, plural bool) (message, bool) {
	if !plural {
		s, ok := b[key]
		if !ok {
			return message{}, false
		}
		return message{Forms: map[string][]token{"": parseAppleText(s)}}, true
	}
	forms := map[string][]token{}
	for _, c := range pluralCategories {
		if s, ok := b[key+"."+c]; ok {
			forms[c] = parseAppleText(s)
		}
	}
	if _, ok := forms["other"]; !ok {
		return message{}, false
	}
	// Apple's plural argument is filled in by Apple's own code; Sona names
	// it through the mapping, so any name serves here.
	return message{Plural: "@", Forms: forms}, true
}

// icuEscape quotes ICU's syntax characters in literal text: apostrophes
// are doubled, braces (and `#` in a plural branch) quoted.
func icuEscape(s string, inPlural bool) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\'':
			b.WriteString("''")
		case r == '{' || r == '}' || r == '#' && inPlural:
			b.WriteString("'" + string(r) + "'")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// toICU renders Apple's message as a Lingui (ICU) message for the Sona
// message it maps to. renames maps Apple's placeholder names to Sona's; in
// a plural, Apple's placeholder renamed to Sona's plural argument becomes
// `#`. Every Apple placeholder must be renamed to one of Sona's.
func toICU(apple message, sona message, renames map[string]string) (string, error) {
	known := sona.placeholders()
	render := func(toks []token, inPlural bool) (string, error) {
		var b strings.Builder
		for _, t := range toks {
			if !t.IsArg {
				b.WriteString(icuEscape(t.Text, inPlural))
				continue
			}
			name, ok := renames[t.Placeholder]
			if !ok || !slices.Contains(known, name) {
				return "", fmt.Errorf("placeholder @@%s@@ has no Sona argument", t.Placeholder)
			}
			if inPlural && name == sona.Plural {
				b.WriteByte('#')
			} else {
				b.WriteString("{" + name + "}")
			}
		}
		return b.String(), nil
	}
	if !sona.isPlural() {
		if apple.isPlural() {
			return "", fmt.Errorf("a plural Apple key for a plain message")
		}
		return render(apple.Forms[""], false)
	}
	if !apple.isPlural() {
		return "", fmt.Errorf("a plain Apple key for a plural message")
	}
	var b strings.Builder
	b.WriteString("{" + sona.Plural + ", plural,")
	for _, c := range apple.categories() {
		s, err := render(apple.Forms[c], true)
		if err != nil {
			return "", fmt.Errorf("%s: %w", c, err)
		}
		b.WriteString(" " + c + " {" + s + "}")
	}
	b.WriteString("}")
	return b.String(), nil
}
