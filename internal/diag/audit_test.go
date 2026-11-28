package diag

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/logging"
)

// keptLogKeys are the log attribute keys of Sona's own code that the
// bundle leaves as they are, because their values are counts, sizes,
// durations, flags, versions, codes, the names of Sona's own parts or
// technical text that holds no title, name, term, route, URL, Apple ID or
// device name (strings in them still pass the string pass: a URL, an Apple
// ID, a user path or an e-mail address in an "err" goes whatever the key).
//
// A key that can identify content or a person is not listed here: classify
// pseudonymises it. TestEveryLogAttributeIsAudited fails for a key that is
// in neither place, so whoever adds a log attribute decides which.
var keptLogKeys = map[string]string{
	// Counts, sizes, durations and positions.
	"after": "duration", "args": "count", "artwork_bytes": "bytes", "at": "time or position", "attempt": "count",
	"available": "flag", "bytes": "size", "challenge": "duration", "changed": "count",
	"checked": "count", "counts": "library counts", "covers": "count", "crossfade": "duration", "current": "version",
	"duration": "duration", "entries": "count", "entry": "index", "expiry": "time", "failed": "count",
	"failures": "count", "first_audio": "duration", "freed_bytes": "bytes", "full": "flag", "gain": "number",
	"held_until_paint": "duration", "hidden": "flag", "i": "index", "ignored": "flag", "in": "duration", "init": "duration",
	"kbps": "number", "last": "time", "last_success": "time", "latest": "version", "length": "bytes", "licence+update": "duration",
	"listens": "count", "live": "flag", "locales": "count", "messages": "count", "missing": "count", "new": "flag", "of": "count",
	"offline": "flag", "offset": "bytes", "ok": "flag", "on": "flag", "on_battery": "flag", "phones": "count",
	"prepare": "duration", "preview": "flag", "quality": "number", "rated": "count", "removed": "count", "resolution": "WxH",
	"rows": "count", "segments": "count", "seq": "number", "session": "CDM session number", "sessions": "count",
	"since_start_ms": "duration", "size": "bytes", "soundCheckGain": "number", "spatial": "duration or flag", "start": "position",
	"took": "duration", "took_ms": "duration", "tried": "count", "trustedDevices": "flag",
	"until": "time", "upower_on_battery": "flag", "webPlayback": "duration", "refused": "count", "budget": "duration",
	// Error codes, statuses, versions and the names of Sona's own parts.
	"addon": "add-on ID: catalogue add-ons are public", "add-on": "add-on ID: catalogue add-ons are public",
	"audio": "HLS group", "backend": "keyring backend", "cdm": "CDM version", "checks": "fixed text", "cmd": "command name",
	"code": "error code", "codec": "codec", "codecs": "codecs", "component": "Sona's part", "decoder": "decoder", "detail": "refusal text",
	"domain": "library domain", "domains": "library domains", "env": "environment variable name", "event": "event name",
	"exit": "error", "flavor": "build flavour", "from": "state", "gstreamer": "version", "host": "Apple's or GitHub's host",
	"hostIf": "interface number", "interface": "interface number", "kind": "kind of item", "line": "child process stderr line",
	"locale": "Apple's language code", "message": "text that did not parse: Sona's own UI strings", "method": "protocol method",
	"mode": "playback mode", "network": "flag", "platform": "platform", "previous": "version", "provider": "scrobbling or lyrics service",
	"reason": "reason code or text", "release": "version", "rendition": "kind of rendition", "script": "Apple strings chunk",
	"bundle": "Apple strings chunk", "signal": "D-Bus signal name", "source": "power source", "stage": "start-up stage", "stack": "Go stack",
	"panic": "panic value", "status": "HTTP status", "storefront": "country code: kept, a bug report needs it", "tag": "release tag",
	"task": "Sona's goroutine", "to": "state", "topic": "add-on event topic", "tree": "flag", "type": "message type", "var": "environment variable name",
	"version": "version", "want": "Go type", "got": "Go type", "why": "reason code", "err": "error text", "cached_account": "flag",
}

// logMethods are the names of the log calls whose key-value arguments follow
// the message.
var logMethods = map[string]int{
	"Debug": 1, "Info": 1, "Warn": 1, "Error": 1, // (msg, args…)
	"DebugContext": 2, "InfoContext": 2, "WarnContext": 2, "ErrorContext": 2, // (ctx, msg, args…)
	"With": 0, // (args…)
	"Log":  3, // (ctx, level, msg, args…)
}

type logKey struct {
	key, where string
}

// logKeys parses the non-test Go files of internal/ and cmd/ and returns
// the literal attribute keys of their log calls: "k" in log.Info("m", "k", v)
// and in log.Info("m", slog.String("k", v)).
func logKeys(t *testing.T) []logKey {
	t.Helper()
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	var keys []logKey
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					keys = append(keys, callKeys(fset, call)...)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(keys) < 100 {
		t.Fatalf("found %d log attribute keys: the walk over the sources is broken", len(keys))
	}
	return keys
}

func callKeys(fset *token.FileSet, call *ast.CallExpr) []logKey {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	skip, ok := logMethods[sel.Sel.Name]
	if !ok {
		return nil
	}
	// A logger: a receiver named like one ("log", "r.log", "e.Logger", "l").
	var recv strings.Builder
	_ = printer.Fprint(&recv, fset, sel.X)
	name := strings.ToLower(recv.String())
	if !strings.Contains(name, "log") && name != "l" {
		return nil
	}
	if len(call.Args) < skip {
		return nil
	}
	var keys []logKey
	args := call.Args[skip:]
	for i := 0; i < len(args); i++ {
		pos := fset.Position(args[i].Pos())
		where := fmt.Sprintf("%s:%d", filepath.ToSlash(pos.Filename), pos.Line)
		switch a := args[i].(type) {
		case *ast.BasicLit:
			if a.Kind == token.STRING && i+1 < len(args) {
				keys = append(keys, logKey{strings.Trim(a.Value, "\"`"), where})
				i++
			}
		case *ast.CallExpr:
			// slog.String("k", v), slog.Any(…), slog.Group("k", …).
			if s, ok := a.Fun.(*ast.SelectorExpr); ok && len(a.Args) > 0 {
				if x, ok := s.X.(*ast.Ident); ok && x.Name == "slog" {
					if lit, ok := a.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						keys = append(keys, logKey{strings.Trim(lit.Value, "\"`"), where})
					}
				}
			}
		}
	}
	return keys
}

// Every attribute key of Sona's log calls is either one the bundle
// anonymises (classify) or one decided to be harmless (keptLogKeys). A new
// key fails here until its author says which: the rule for the decision is in
// docs/architecture.md, "Diagnostics and crashes".
func TestEveryLogAttributeIsAudited(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range logKeys(t) {
		if seen[k.key] {
			continue
		}
		seen[k.key] = true
		_, kept := keptLogKeys[k.key]
		anonymised := classify(k.key) != plain
		switch {
		case anonymised && kept:
			t.Errorf("log key %q (%s) is anonymised by classify and also listed in keptLogKeys: remove it from keptLogKeys", k.key, k.where)
		case !anonymised && !kept && !logging.SensitiveKey(k.key):
			t.Errorf("log key %q (%s) is new: if its value can be a title, name, search term, route, URL, Apple ID, device name or any ID of content, add its name to classify in internal/diag/anonymise.go; if it is a count, size, duration, flag, code or version, add it to keptLogKeys", k.key, k.where)
		}
	}
}

// The keys keptLogKeys lists stay what they say: none may be one that
// classify anonymises, so the list cannot hide a key.
func TestKeptLogKeysAreNotAnonymised(t *testing.T) {
	var bad []string
	for k := range keptLogKeys {
		if classify(k) != plain {
			bad = append(bad, k)
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("keptLogKeys holds keys classify anonymises: %v", bad)
	}
}
