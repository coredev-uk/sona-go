// Package logging builds Sona's slog logger: text to stderr in development,
// JSON to a size-rotated file in the state directory, and a redacting
// handler in front of both so tokens never reach a log line.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// Redacted replaces every secret the handler removes.
const Redacted = "[REDACTED]"

var (
	// JWTs: three base64url segments, the first starting with a JSON header.
	jwtRe = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]*`)
	// key=value / key: value / "key":"value" pairs whose key names a secret,
	// including HTTP headers and query parameters. An optional "Bearer " is
	// swallowed with the value; an already redacted value is matched whole,
	// so redacting twice changes nothing.
	kvRe = regexp.MustCompile(`(?i)\b(media-user-token|music-user-token|musicusertoken|mediausertoken|x-apple-music-user-token|mut|token|access_token|refresh_token|dev_token|authorization)\b(\s*["']?\s*[:=]\s*["']?)(?:\[REDACTED\]|(?:bearer\s+)?[^\s"',;&}\])]+)`)
	// A bare "Bearer <credential>".
	bearerRe = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`)
	// Music User Tokens are long base64 strings with no fixed prefix; any
	// unbroken base64 run of 100+ characters is treated as one.
	longB64Re = regexp.MustCompile(`[A-Za-z0-9+/_-]{100,}={0,2}`)
	// The -ldflags build setting, as a Go program's "Build Info:"
	// record logs it (JSON `"-ldflags":"…"`, text `-ldflags="…"`): the whole
	// value goes, quoted or bare.
	ldflagsRe = regexp.MustCompile(`(-ldflags["']?\s*[:=]\s*)(?:"(?:[^"\\]|\\.)*"|'[^']*'|[^\s"',;&}\])]+)`)
	// A linker -X assignment anywhere else (-X pkg.name=value): the build
	// injects API credentials this way, so every value goes, whatever the
	// symbol is called.
	linkerXRe = regexp.MustCompile(`(^|[\s"'=])(-X(?:\s+|=)["']?[\w./~-]+=)[^\s"'\\]+`)
)

// Redact scrubs secrets from free text: JWTs, values of token-named keys
// and headers (Media-User-Token, Authorization, token=…), bearer
// credentials, long base64 runs that look like Music User Tokens, and
// build flags that carry compiled-in credentials (-ldflags values, linker
// -X assignments).
func Redact(s string) string {
	if !mayHoldSecret(s) {
		return s
	}
	return scrub(s)
}

// scrub is Redact's regexps.
func scrub(s string) string {
	s = ldflagsRe.ReplaceAllString(s, `${1}"`+Redacted+`"`)
	s = linkerXRe.ReplaceAllString(s, "${1}${2}"+Redacted)
	s = kvRe.ReplaceAllString(s, "${1}${2}"+Redacted)
	s = jwtRe.ReplaceAllString(s, Redacted)
	s = bearerRe.ReplaceAllString(s, "Bearer "+Redacted)
	s = longB64Re.ReplaceAllStringFunc(s, func(m string) string {
		if strings.Contains(m, Redacted) {
			return m
		}
		return Redacted
	})
	return s
}

// mayHoldSecret is a cheap test before the regexps, which run on every log
// message and string attribute: false means none of them can match. A match
// needs one of: 100 characters (longB64Re); a ':' or '=' (kvRe); a '-'
// (ldflagsRe, linkerXRe); "eyJ" (jwtRe); "bearer" in any case (bearerRe).
func mayHoldSecret(s string) bool {
	if len(s) >= 100 || strings.ContainsAny(s, ":=-") || strings.Contains(s, "eyJ") {
		return true
	}
	for i := 0; i+len("bearer") <= len(s); i++ {
		if s[i]|0x20 == 'b' && strings.EqualFold(s[i:i+len("bearer")], "bearer") {
			return true
		}
	}
	return false
}

// keyNormalizer drops the separators NormalizeKey ignores.
var keyNormalizer = strings.NewReplacer("-", "", "_", "")

// NormalizeKey lower-cases key and removes '-' and '_', the form the
// secret-key checks compare ("Set-Cookie" and "set_cookie" are "setcookie").
func NormalizeKey(key string) string {
	return strings.ToLower(keyNormalizer.Replace(key))
}

// sensitiveKey reports whether an attribute key names a secret. Matching
// ignores case, '-' and '_' and accepts any key ending in "token".
// "ldflags" is a Build Info setting: the linker flags inject Sona's
// Last.fm and Discord credentials (-X), and nothing else in them is worth
// a log line (the version is logged at startup).
func sensitiveKey(key string) bool {
	k := NormalizeKey(key)
	switch k {
	case "mut", "authorization", "auth", "cookie", "setcookie", "password", "secret", "ldflags":
		return true
	}
	return strings.HasSuffix(k, "token")
}

// RedactHandler wraps a slog.Handler and scrubs every record before it
// reaches it: attributes with secret-naming keys are replaced wholesale, and
// the message and all string-like values pass through [Redact].
type RedactHandler struct {
	inner slog.Handler
}

// NewRedactHandler returns h behind redaction.
func NewRedactHandler(h slog.Handler) *RedactHandler { return &RedactHandler{inner: h} }

// Enabled implements slog.Handler.
func (h *RedactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

// Handle implements slog.Handler.
func (h *RedactHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, Redact(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

// WithAttrs implements slog.Handler; the attributes are redacted once here.
func (h *RedactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		clean[i] = redactAttr(a)
	}
	return &RedactHandler{inner: h.inner.WithAttrs(clean)}
}

// WithGroup implements slog.Handler.
func (h *RedactHandler) WithGroup(name string) slog.Handler {
	return &RedactHandler{inner: h.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	if sensitiveKey(a.Key) {
		return slog.String(a.Key, Redacted)
	}
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, Redact(v.String()))
	case slog.KindGroup:
		group := v.Group()
		clean := make([]any, len(group))
		for i, ga := range group {
			clean[i] = redactAttr(ga)
		}
		return slog.Group(a.Key, clean...)
	case slog.KindAny:
		// Errors, Stringers, structs, maps: render to text and scrub, so a
		// token inside a struct field or error message cannot slip through.
		var s string
		switch x := v.Any().(type) {
		case error:
			s = x.Error()
		case fmt.Stringer:
			s = x.String()
		case []byte:
			s = string(x)
		default:
			s = fmt.Sprintf("%+v", x)
		}
		return slog.String(a.Key, Redact(s))
	default:
		return slog.Attr{Key: a.Key, Value: v}
	}
}
