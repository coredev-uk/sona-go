// Package po reads and writes the gettext PO catalogues Lingui keeps
// Sona's messages in (frontend/src/shared/i18n): the subset Lingui and
// translation tools write.
package po

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
)

// Entry is one gettext message. Lingui writes the English source text as
// msgid (its runtime IDs are hashes of msgid and msgctxt), so an entry's
// identity is its msgid and context.
type Entry struct {
	Translator []string // "# " lines
	Extracted  []string // "#." lines
	References []string // "#:" lines
	Flags      []string // "#," flags
	Context    string
	HasContext bool
	ID         string
	Str        string
	Obsolete   bool
}

// Key identifies a message across catalogues.
type Key struct {
	ID, Context string
}

// Key returns the entry's identity.
func (e *Entry) Key() Key { return Key{e.ID, e.Context} }

// Fuzzy reports the "fuzzy" flag.
func (e *Entry) Fuzzy() bool {
	for _, f := range e.Flags {
		if f == "fuzzy" {
			return true
		}
	}
	return false
}

// File is a parsed catalogue: the header (the msgid "" entry's msgstr)
// and the messages in file order.
type File struct {
	Header  string
	Entries []*Entry
}

// Read parses the catalogue at path.
func Read(path string) (*File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Parse reads the subset of gettext PO that Lingui and translation tools
// write: comments, flags, msgctxt, msgid, msgstr (no msgid_plural: Sona's
// plurals are ICU inside one message), continuation lines and obsolete
// "#~" entries.
func Parse(b []byte) (*File, error) {
	f := &File{}
	var cur *Entry
	var field *string
	headerDone := false
	flush := func() {
		if cur == nil {
			return
		}
		if cur.ID == "" && !cur.HasContext && !headerDone {
			f.Header = cur.Str
			headerDone = true
		} else if cur.ID != "" {
			f.Entries = append(f.Entries, cur)
		}
		cur, field = nil, nil
	}
	ensure := func() {
		if cur == nil {
			cur = &Entry{}
		}
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		s := strings.TrimSpace(sc.Text())
		obsolete := false
		if strings.HasPrefix(s, "#~") {
			obsolete = true
			s = strings.TrimSpace(s[2:])
		}
		switch {
		case s == "":
			flush()
		case strings.HasPrefix(s, "#"):
			if cur != nil && field == &cur.Str { // a comment after msgstr starts a new entry
				flush()
			}
			ensure()
			switch {
			case strings.HasPrefix(s, "#."):
				cur.Extracted = append(cur.Extracted, strings.TrimSpace(s[2:]))
			case strings.HasPrefix(s, "#:"):
				cur.References = append(cur.References, strings.TrimSpace(s[2:]))
			case strings.HasPrefix(s, "#,"):
				for _, fl := range strings.Split(s[2:], ",") {
					if fl = strings.TrimSpace(fl); fl != "" {
						cur.Flags = append(cur.Flags, fl)
					}
				}
			case strings.HasPrefix(s, "#|"):
				// previous msgid: dropped
			default:
				cur.Translator = append(cur.Translator, strings.TrimSpace(strings.TrimPrefix(s, "#")))
			}
		case strings.HasPrefix(s, "msgctxt "), strings.HasPrefix(s, "msgid "), strings.HasPrefix(s, "msgstr "):
			kw, rest, _ := strings.Cut(s, " ")
			if kw == "msgctxt" || kw == "msgid" {
				if cur != nil && field == &cur.Str {
					flush()
				}
			}
			ensure()
			cur.Obsolete = cur.Obsolete || obsolete
			v, err := unquote(rest)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			switch kw {
			case "msgctxt":
				cur.Context, cur.HasContext, field = v, true, &cur.Context
			case "msgid":
				cur.ID, field = v, &cur.ID
			default:
				cur.Str, field = v, &cur.Str
			}
		case strings.HasPrefix(s, `"`):
			if field == nil {
				return nil, fmt.Errorf("line %d: string outside a field", line)
			}
			v, err := unquote(s)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			*field += v
		default:
			return nil, fmt.Errorf("line %d: unexpected %q", line, s)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	flush()
	return f, nil
}

func unquote(s string) (string, error) {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return "", fmt.Errorf("not a quoted string: %s", s)
	}
	var b strings.Builder
	in := s[1 : len(s)-1]
	for i := 0; i < len(in); i++ {
		c := in[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i == len(in) {
			return "", fmt.Errorf("dangling escape: %s", s)
		}
		switch in[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '"', '\\':
			b.WriteByte(in[i])
		default:
			return "", fmt.Errorf("unknown escape \\%c: %s", in[i], s)
		}
	}
	return b.String(), nil
}

func quote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`, "\r", `\r`)
	return `"` + r.Replace(s) + `"`
}

// Format writes the catalogue without line wrapping, as Lingui does.
func Format(f *File) []byte {
	var b bytes.Buffer
	b.WriteString("msgid \"\"\nmsgstr \"\"\n")
	for _, l := range strings.SplitAfter(f.Header, "\n") {
		if l != "" {
			b.WriteString(quote(l) + "\n")
		}
	}
	for _, e := range f.Entries {
		if e.Obsolete {
			continue
		}
		b.WriteByte('\n')
		for _, c := range e.Translator {
			b.WriteString(strings.TrimRight("# "+c, " ") + "\n")
		}
		for _, c := range e.Extracted {
			b.WriteString("#. " + c + "\n")
		}
		for _, c := range e.References {
			b.WriteString("#: " + c + "\n")
		}
		if len(e.Flags) > 0 {
			b.WriteString("#, " + strings.Join(e.Flags, ", ") + "\n")
		}
		if e.HasContext {
			b.WriteString("msgctxt " + quote(e.Context) + "\n")
		}
		b.WriteString("msgid " + quote(e.ID) + "\n")
		b.WriteString("msgstr " + quote(e.Str) + "\n")
	}
	return b.Bytes()
}

// Header builds a header from ordered fields.
func Header(fields ...[2]string) string {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(f[0] + ": " + f[1] + "\n")
	}
	return b.String()
}

// HeaderField returns a header field's value ("" when absent).
func HeaderField(header, name string) string {
	for _, l := range strings.Split(header, "\n") {
		if k, v, ok := strings.Cut(l, ":"); ok && strings.EqualFold(strings.TrimSpace(k), name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
