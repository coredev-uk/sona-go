package applestrings

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// The fuzz targets below feed the string-bundle code what Apple's web player
// serves: a minified JavaScript module of translations (which is never
// run, only read), the index script that names the bundles, and Apple's
// message text. Their seeds are in testdata/fuzz/<target> (made-up strings);
// go test runs them as ordinary tests.

// FuzzParseBundleModule reads a bundle chunk.
//
// Invariants: no panic; a bundle comes back with no error and an error with
// no bundle; the bundle has no more entries than the chunk has bytes
// for them (a key, a colon and quotes take four), and every key and value is
// valid UTF-8 (a lone surrogate escape must not make the UI show invalid
// text); the result is the same when read twice.
func FuzzParseBundleModule(f *testing.F) {
	f.Fuzz(func(t *testing.T, src string) {
		b, err := ParseBundleModule(src)
		if err != nil {
			if b != nil {
				t.Fatalf("a bundle of %d entries with the error %v", len(b), err)
			}
			return
		}
		if len(b) > len(src)/4+1 {
			t.Fatalf("%d entries from %d bytes", len(b), len(src))
		}
		for k, v := range b {
			if !utf8.ValidString(k) || !utf8.ValidString(v) {
				t.Fatalf("invalid UTF-8 in %q: %q", k, v)
			}
		}
		again, err := ParseBundleModule(src)
		if err != nil || len(again) != len(b) {
			t.Fatalf("a second read gave %d entries and %v", len(again), err)
		}
		for k, v := range b {
			if again[k] != v {
				t.Fatalf("a second read gave %q for %q, not %q", again[k], k, v)
			}
		}
	})
}

// FuzzLocaleBundles reads the index script's table of locale bundles.
//
// Invariants: no panic; every key is a lower-case locale tag and every URL
// is the given origin followed by /assets/ and a file name with no path
// separator or query, so a hostile index script cannot point a download at
// another host or path.
func FuzzLocaleBundles(f *testing.F) {
	const origin = "https://example.invalid"
	f.Fuzz(func(t *testing.T, js []byte) {
		for loc, u := range localeBundles(js, origin) {
			name, ok := strings.CutPrefix(u, origin+"/assets/")
			if !ok || name == "" || strings.ContainsAny(name, "/\\?#:@ ") {
				t.Fatalf("locale %q has the URL %q", loc, u)
			}
			if loc != strings.ToLower(loc) || loc == "" {
				t.Fatalf("locale %q", loc)
			}
		}
	})
}

// FuzzMessages reads Apple's message text and a Sona message, and converts
// one to the other's ICU form.
//
// Invariants: no panic; a Sona message that parses lists each placeholder
// once; when ToICU succeeds for a plain message, the text it wrote has
// balanced braces outside ICU quotes, and every argument outside quotes is
// one the Sona message has, so text from Apple can never add an argument or
// break the message's structure.
func FuzzMessages(f *testing.F) {
	f.Fuzz(func(t *testing.T, appleText, sonaID string) {
		sona, ok := ParseSona(sonaID)
		if !ok {
			return
		}
		names := sona.Placeholders()
		for i, n := range names {
			if slices.Contains(names[i+1:], n) {
				t.Fatalf("placeholder %q twice", n)
			}
		}
		apple := Message{Forms: map[string][]Token{"": parseAppleText(appleText)}}
		if sona.IsPlural() {
			apple = Message{Plural: "@", Forms: map[string][]Token{"other": parseAppleText(appleText)}}
		}
		renames := map[string]string{}
		for _, p := range apple.Placeholders() {
			if len(names) > 0 {
				renames[p] = names[0]
			}
		}
		out, err := ToICU(apple, sona, renames)
		if err != nil || sona.IsPlural() {
			return
		}
		args, ok := icuArgs(out)
		if !ok {
			t.Fatalf("ToICU wrote %q, whose braces are not balanced", out)
		}
		for _, a := range args {
			if !slices.Contains(names, a) {
				t.Fatalf("ToICU wrote the argument %q of %q, which %q lacks", a, out, sonaID)
			}
		}
	})
}

// icuArgs reads the arguments of a plain ICU message outside its quotes:
// ok is false when braces are unbalanced or an argument is not a simple name.
func icuArgs(s string) (args []string, ok bool) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\'':
			switch {
			case i+1 < len(s) && s[i+1] == '\'':
				i++
			case i+1 < len(s) && (s[i+1] == '{' || s[i+1] == '}' || s[i+1] == '#'):
				// A quote runs to the next single apostrophe.
				j := i + 1
				for j < len(s) && (s[j] != '\'' || (j+1 < len(s) && s[j+1] == '\'')) {
					if s[j] == '\'' {
						j++
					}
					j++
				}
				i = j
			}
		case '{':
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				return nil, false
			}
			name := s[i+1 : i+end]
			if !simpleArgRe.MatchString(name) {
				return nil, false
			}
			args = append(args, name)
			i += end
		case '}':
			return nil, false
		}
	}
	return args, true
}
