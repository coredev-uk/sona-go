package diag

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The fuzz targets below feed the anonymiser what a log line can hold: any
// string a person typed, a server sent or a stack trace printed. The
// anonymiser is what keeps the bundle, which is pasted into public issues,
// free of identifying values, so what it promises is checked on arbitrary
// text. Their seeds are in testdata/fuzz/<target> (made-up values only);
// go test runs them as ordinary tests.

// fuzzHome is the home directory the anonymiser of the targets hides.
const fuzzHome = "/home/tester"

// pseudonymOnly is a value that was replaced whole by a pseudonym.
var pseudonymOnly = regexp.MustCompile(`^[a-z0-9]+~[0-9a-f]{6}$`)

// leaks lists what the output of the string pass must not hold, whatever
// the input had: an e-mail address, an Apple ID, a URL with a query, a
// fragment or user info, an API path with a query, or a home directory.
func leaks(out string) []string {
	var found []string
	for _, m := range emailRe.FindAllString(out, -1) {
		found = append(found, "e-mail "+m)
	}
	for _, m := range appleIDRe.FindAllStringSubmatchIndex(out, -1) {
		if id := out[m[4]:m[5]]; isAppleID(id, out[m[5]:]) {
			found = append(found, "Apple ID "+id)
		}
	}
	for _, m := range urlRe.FindAllString(out, -1) {
		trimmed := strings.TrimRight(m, ".,;:!?)]")
		u, err := url.Parse(trimmed)
		if err != nil || u.Host == "" {
			if strings.ContainsAny(trimmed, "?#") {
				found = append(found, "URL query "+m)
			}
			continue
		}
		if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.User != nil || strings.ContainsAny(trimmed, "?#@") {
			found = append(found, "URL query, fragment or user info "+m)
		}
	}
	for _, m := range apiPathRe.FindAllStringSubmatch(out, -1) {
		if m[3] != "" {
			found = append(found, "API path query "+m[3])
		}
	}
	return found
}

// FuzzAnonymiseText runs the string pass, which every string of a bundle goes
// through whatever its key.
//
// Invariants: no panic; the output has no e-mail address, no Apple ID, no
// URL query, fragment or user info and no API path query (so none the input
// had); the home directory is gone; the pass is idempotent (anonymising
// its output changes nothing, so a line handled twice reads the same);
// the output is never more than a constant factor longer than the input.
func FuzzAnonymiseText(f *testing.F) {
	a := newAnonymiser(fuzzHome)
	f.Fuzz(func(t *testing.T, s string) {
		out := a.text(s)
		if l := leaks(out); len(l) > 0 {
			t.Fatalf("%q leaks %q", out, l)
		}
		if strings.Contains(out, fuzzHome) {
			t.Fatalf("%q holds the home directory", out)
		}
		if again := a.text(out); again != out {
			t.Fatalf("not idempotent:\n%q\n%q", out, again)
		}
		if len(out) > 4*len(s)+64 {
			t.Fatalf("%d bytes became %d", len(s), len(out))
		}
	})
}

// FuzzAnonymiseValue anonymises one attribute value under a key, which picks
// how (a pseudonym, a route, a URL, a path, plain text).
//
// Invariants: no panic; a value under a key of the pseudonym kind is
// replaced whole by "label~hex" and the same value gives the same pseudonym;
// any other value leaks nothing the string pass removes, and anonymising
// it again changes nothing.
func FuzzAnonymiseValue(f *testing.F) {
	a := newAnonymiser(fuzzHome)
	f.Fuzz(func(t *testing.T, key, s string) {
		out := a.value(key, s)
		if s == "" {
			if out != "" {
				t.Fatalf("empty value became %q", out)
			}
			return
		}
		if classify(key) == pseudo {
			if !pseudonymOnly.MatchString(out) || out != a.value(key, s) {
				t.Fatalf("key %q: %q is not a stable pseudonym", key, out)
			}
			return
		}
		if l := leaks(out); len(l) > 0 {
			t.Fatalf("key %q: %q leaks %q", key, out, l)
		}
		if again := a.value(key, out); again != out {
			t.Fatalf("key %q is not idempotent:\n%q\n%q", key, out, again)
		}
	})
}

// FuzzAnonymiseLogLine anonymises one line of the log tail, which is a JSON
// record, a crash dump or text.
//
// Invariants: no panic; a record comes out as a JSON object, the same as
// jsonLine gives; no string value of it leaks (see FuzzAnonymiseText); any
// other line is the string pass of it. (A record is not idempotent: a value
// under a pseudonym key is hashed again, which is the same as a different
// value.)
func FuzzAnonymiseLogLine(f *testing.F) {
	a := newAnonymiser(fuzzHome)
	f.Fuzz(func(t *testing.T, line string) {
		out := a.logLine(line)
		trimmed := strings.TrimSpace(line)
		jsonOut, ok := a.jsonLine(trimmed)
		if !strings.HasPrefix(trimmed, "{") || !ok {
			if out != a.text(line) {
				t.Fatalf("a text line must be the string pass: %q", out)
			}
			return
		}
		if out != jsonOut {
			t.Fatalf("logLine and jsonLine differ: %q %q", out, jsonOut)
		}
		if !json.Valid([]byte(out)) {
			t.Fatalf("output is not JSON: %q", out)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(out), &obj); err != nil {
			t.Fatalf("output is not an object: %q", out)
		}
		var walk func(v any)
		walk = func(v any) {
			switch v := v.(type) {
			case string:
				if l := leaks(v); len(l) > 0 {
					t.Fatalf("%q leaks %q", v, l)
				}
			case []any:
				for _, e := range v {
					walk(e)
				}
			case map[string]any:
				for _, e := range v {
					walk(e)
				}
			}
		}
		walk(obj)
	})
}
