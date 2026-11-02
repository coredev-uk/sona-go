package applestrings

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// A Token is literal text or a placeholder. In a plural branch the plural
// value (ICU's `#`) is the placeholder named PluralValue.
type Token struct {
	Text        string
	Placeholder string
	IsArg       bool
}

// PluralValue names the plural value (`#`) as a placeholder.
const PluralValue = "#"

func lit(s string) Token { return Token{Text: s} }
func arg(n string) Token { return Token{Placeholder: n, IsArg: true} }

func (t Token) String() string {
	if t.IsArg {
		return "{" + t.Placeholder + "}"
	}
	return t.Text
}

// Message is a Sona or Apple message reduced to what matching and
// conversion compare: a plain text, or a plural over one argument with a
// text per CLDR category.
type Message struct {
	Plural string             // the plural argument ("" for plain text)
	Forms  map[string][]Token // category ("" for plain text) → tokens
}

// IsPlural reports a plural message.
func (m Message) IsPlural() bool { return m.Plural != "" }

// PluralCategories are CLDR's, the suffixes Apple's keys use.
var PluralCategories = []string{"zero", "one", "two", "few", "many", "other"}

// Placeholders lists a message's argument names (the plural argument
// included), each once.
func (m Message) Placeholders() []string {
	var names []string
	if m.IsPlural() {
		names = append(names, m.Plural)
	}
	for _, c := range m.Categories() {
		for _, t := range m.Forms[c] {
			if t.IsArg && t.Placeholder != PluralValue && !slices.Contains(names, t.Placeholder) {
				names = append(names, t.Placeholder)
			}
		}
	}
	return names
}

// Categories lists the message's forms in CLDR order ("" for plain text).
func (m Message) Categories() []string {
	if !m.IsPlural() {
		return []string{""}
	}
	var cs []string
	for _, c := range PluralCategories {
		if _, ok := m.Forms[c]; ok {
			cs = append(cs, c)
		}
	}
	return cs
}

var simpleArgRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// ParseSona reads a Lingui message (ICU MessageFormat) when it is plain text
// with simple `{name}` arguments, or one plural argument spanning the whole
// message whose branches are such text. Anything else (selects, several
// plurals, text around a plural, `=n` branches) cannot map to one Apple
// key, so ok is false.
func ParseSona(id string) (m Message, ok bool) {
	if name, body, isPlural := cutPlural(id); isPlural {
		forms, ok := parseBranches(body)
		if !ok {
			return Message{}, false
		}
		if _, ok := forms["other"]; !ok {
			return Message{}, false
		}
		return Message{Plural: name, Forms: forms}, true
	}
	toks, ok := parseText(id, false)
	if !ok {
		return Message{}, false
	}
	return Message{Forms: map[string][]Token{"": toks}}, true
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

func parseBranches(body string) (map[string][]Token, bool) {
	forms := map[string][]Token{}
	s := strings.TrimSpace(body)
	for s != "" {
		open := strings.IndexByte(s, '{')
		if open < 0 {
			return nil, false
		}
		sel := strings.TrimSpace(s[:open])
		if !slices.Contains(PluralCategories, sel) {
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
func parseText(s string, inPlural bool) ([]Token, bool) {
	var toks []Token
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
			toks = append(toks, arg(PluralValue))
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
func parseAppleText(s string) []Token {
	var toks []Token
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

// SplitPluralKey cuts a CLDR category suffix off an Apple key
// ("ASE.Web.Music.NoOfSongs.one" → "ASE.Web.Music.NoOfSongs", "one").
func SplitPluralKey(key string) (base, category string, ok bool) {
	i := strings.LastIndexByte(key, '.')
	if i < 0 || !slices.Contains(PluralCategories, key[i+1:]) {
		return key, "", false
	}
	return key[:i], key[i+1:], true
}

// AppleMessage reads key from a bundle: the plain string, or, for a plural
// mapping, every category form of the key (`.other` required).
func AppleMessage(b Bundle, key string, plural bool) (Message, bool) {
	if !plural {
		s, ok := b[key]
		if !ok {
			return Message{}, false
		}
		return Message{Forms: map[string][]Token{"": parseAppleText(s)}}, true
	}
	forms := map[string][]Token{}
	for _, c := range PluralCategories {
		if s, ok := b[key+"."+c]; ok {
			forms[c] = parseAppleText(s)
		}
	}
	if _, ok := forms["other"]; !ok {
		return Message{}, false
	}
	// Apple's plural argument is filled in by Apple's own code; Sona names
	// it through the mapping, so any name serves here.
	return Message{Plural: "@", Forms: forms}, true
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

// ToICU renders Apple's message as a Lingui (ICU) message for the Sona
// message it maps to. renames maps Apple's placeholder names to Sona's; in
// a plural, Apple's placeholder renamed to Sona's plural argument becomes
// `#`. The placeholders must agree: every Apple placeholder renames to one
// of Sona's arguments, and every argument Sona's message shows appears in
// Apple's text (in a plural, the count in the `other` form when Sona's
// `other` shows it; a `one` form may spell the number out).
func ToICU(apple, sona Message, renames map[string]string) (string, error) {
	known := sona.Placeholders()
	render := func(toks []Token, inPlural bool, used map[string]bool) (string, error) {
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
				used[PluralValue] = true
			} else {
				b.WriteString("{" + name + "}")
				used[name] = true
			}
		}
		return b.String(), nil
	}
	if !sona.IsPlural() {
		if apple.IsPlural() {
			return "", fmt.Errorf("a plural Apple key for a plain message")
		}
		used := map[string]bool{}
		s, err := render(apple.Forms[""], false, used)
		if err != nil {
			return "", err
		}
		return s, missingArgs(sona.Forms[""], used)
	}
	if !apple.IsPlural() {
		return "", fmt.Errorf("a plain Apple key for a plural message")
	}
	var b strings.Builder
	b.WriteString("{" + sona.Plural + ", plural,")
	used := map[string]bool{}
	for _, c := range apple.Categories() {
		form := map[string]bool{}
		s, err := render(apple.Forms[c], true, form)
		if err != nil {
			return "", fmt.Errorf("%s: %w", c, err)
		}
		if c == "other" {
			if err := missingArgs(onlyCount(sona.Forms["other"]), form); err != nil {
				return "", fmt.Errorf("other: %w", err)
			}
		}
		for n := range form {
			used[n] = true
		}
		b.WriteString(" " + c + " {" + s + "}")
	}
	b.WriteString("}")
	var named []Token
	for _, c := range sona.Categories() {
		for _, t := range sona.Forms[c] {
			if t.IsArg && t.Placeholder != PluralValue {
				named = append(named, t)
			}
		}
	}
	if err := missingArgs(named, used); err != nil {
		return "", err
	}
	return b.String(), nil
}

// onlyCount keeps a plural form's `#`.
func onlyCount(toks []Token) []Token {
	var out []Token
	for _, t := range toks {
		if t.IsArg && t.Placeholder == PluralValue {
			out = append(out, t)
		}
	}
	return out
}

// missingArgs names the first argument of want that used lacks.
func missingArgs(want []Token, used map[string]bool) error {
	for _, t := range want {
		if t.IsArg && !used[t.Placeholder] {
			return fmt.Errorf("the argument %s is missing from Apple's text", t)
		}
	}
	return nil
}
