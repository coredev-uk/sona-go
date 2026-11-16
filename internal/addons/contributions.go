package addons

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// What an add-on may contribute to Sona's own surfaces: actions on items,
// commands in the palette and rows in its own section of Settings. Sona draws
// all of it; choosing one sends the add-on an invoke (docs/addons.md). Every
// label is plain text, shown with the add-on's name.

// Limits of the contributions.
const (
	MaxActions     = 8
	MaxCommands    = 8
	MaxSettings    = 16
	MaxOptions     = 16
	MaxLabelRunes  = 60
	MaxDetailRunes = 200
	MaxLocales     = 40
	MaxStrings     = 100
	maxTotal       = 32
)

// ActionKinds are the kinds of item an action may be for.
var ActionKinds = []string{"song", "album", "artist", "playlist"}

// Setting kinds.
const (
	SettingToggle = "toggle" // "true" or "false"
	SettingText   = "text"
	SettingChoice = "choice"
	// SettingSecret is a text Sona keeps in the keyring under `addon.<id>`
	// and never in settings.json; the row never shows it again.
	SettingSecret = "secret"
)

// Contributions is the manifest's `contributions`.
type Contributions struct {
	Actions  []Action  `json:"actions,omitempty"`
	Commands []Command `json:"commands,omitempty"`
	Settings []Setting `json:"settings,omitempty"`
}

// Action is an item action: an entry in the menu of a song, album, artist or
// playlist. Its full ID is `addon.<add-on id>.<id>`.
type Action struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Kinds []string `json:"kinds"`
	// Icon is the name of one of Sona's own icons; an unknown name gets the
	// plain one. An add-on cannot bring a picture.
	Icon string `json:"icon,omitempty"`
}

// Command is a command in the palette. Its full ID is
// `addon.<add-on id>.<id>`. It has no keys of its own: you may bind it in
// Settings → Shortcuts, and an add-on can't take a key from Sona.
type Command struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Keywords string `json:"keywords,omitempty"`
	Icon     string `json:"icon,omitempty"`
}

// Setting is a row in the add-on's section of Settings → Add-ons.
type Setting struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	// Default is the value before the user sets one (a toggle: "true" or
	// "false"; a choice: one of Options). Secrets have none.
	Default string   `json:"default,omitempty"`
	Options []Option `json:"options,omitempty"`
}

// Option is one choice of a choice row.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

var (
	localIDExpr = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	iconExpr    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	localeExpr  = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8}){0,2}$`)
	valueExpr   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,40}$`)
)

// ActionID is the full ID of a contribution of add-on id.
func ActionID(id, local string) string { return "addon." + id + "." + local }

// LocalID is the contribution's own ID in a full action ID of add-on id; ok
// is false for an ID that isn't of that add-on.
func LocalID(id, action string) (local string, ok bool) {
	local, ok = strings.CutPrefix(action, "addon."+id+".")
	return local, ok && localIDExpr.MatchString(local)
}

// Has reports whether the manifest declares an action or command with this
// local ID: the only things Sona will invoke.
func (c Contributions) Has(local string) bool {
	return slices.ContainsFunc(c.Actions, func(a Action) bool { return a.ID == local }) ||
		slices.ContainsFunc(c.Commands, func(a Command) bool { return a.ID == local })
}

func (c Contributions) validate() error {
	if len(c.Actions) > MaxActions || len(c.Commands) > MaxCommands || len(c.Settings) > MaxSettings ||
		len(c.Actions)+len(c.Commands)+len(c.Settings) > maxTotal {
		return problem(ProblemContribution, "too many contributions")
	}
	ids := map[string]bool{}
	bad := func(format string, args ...any) error { return problem(ProblemContribution, format, args...) }
	for _, a := range c.Actions {
		if !localIDExpr.MatchString(a.ID) || ids[a.ID] {
			return bad("action id %q", a.ID)
		}
		ids[a.ID] = true
		if err := plainText(a.Label, 1, MaxLabelRunes); err != nil {
			return bad("action %s label: %v", a.ID, err)
		}
		if len(a.Kinds) == 0 || len(a.Kinds) > len(ActionKinds) {
			return bad("action %s kinds", a.ID)
		}
		for i, k := range a.Kinds {
			if !slices.Contains(ActionKinds, k) || slices.Contains(a.Kinds[:i], k) {
				return bad("action %s kind %q", a.ID, k)
			}
		}
		if a.Icon != "" && !iconExpr.MatchString(a.Icon) {
			return bad("action %s icon", a.ID)
		}
	}
	for _, a := range c.Commands {
		if !localIDExpr.MatchString(a.ID) || ids[a.ID] {
			return bad("command id %q", a.ID)
		}
		ids[a.ID] = true
		if err := plainText(a.Label, 1, MaxLabelRunes); err != nil {
			return bad("command %s label: %v", a.ID, err)
		}
		if a.Keywords != "" {
			if err := plainText(a.Keywords, 1, MaxLabelRunes); err != nil {
				return bad("command %s keywords: %v", a.ID, err)
			}
		}
		if a.Icon != "" && !iconExpr.MatchString(a.Icon) {
			return bad("command %s icon", a.ID)
		}
	}
	rows := map[string]bool{}
	for _, s := range c.Settings {
		if !localIDExpr.MatchString(s.ID) || rows[s.ID] {
			return bad("setting id %q", s.ID)
		}
		rows[s.ID] = true
		if err := plainText(s.Label, 1, MaxLabelRunes); err != nil {
			return bad("setting %s label: %v", s.ID, err)
		}
		if s.Description != "" {
			if err := plainText(s.Description, 1, MaxDetailRunes); err != nil {
				return bad("setting %s description: %v", s.ID, err)
			}
		}
		if err := s.validateKind(); err != nil {
			return err
		}
	}
	return nil
}

func (s Setting) validateKind() error {
	bad := func(format string, args ...any) error {
		return problem(ProblemContribution, "setting %s: "+format, append([]any{s.ID}, args...)...)
	}
	if s.Kind != SettingChoice && len(s.Options) != 0 {
		return bad("options on a %s row", s.Kind)
	}
	switch s.Kind {
	case SettingToggle:
		if s.Default != "" && s.Default != "true" && s.Default != "false" {
			return bad("default %q", s.Default)
		}
	case SettingText:
		if len(s.Default) > MaxDetailRunes {
			return bad("default too long")
		}
		if err := plainOrEmpty(s.Default); err != nil {
			return bad("default: %v", err)
		}
	case SettingSecret:
		if s.Default != "" {
			return bad("a secret has no default")
		}
	case SettingChoice:
		if len(s.Options) < 2 || len(s.Options) > MaxOptions {
			return bad("%d options", len(s.Options))
		}
		seen := map[string]bool{}
		for _, o := range s.Options {
			if !valueExpr.MatchString(o.Value) || seen[o.Value] {
				return bad("option value %q", o.Value)
			}
			seen[o.Value] = true
			if err := plainText(o.Label, 1, MaxLabelRunes); err != nil {
				return bad("option %s label: %v", o.Value, err)
			}
		}
		if s.Default != "" && !seen[s.Default] {
			return bad("default %q is not an option", s.Default)
		}
	default:
		return bad("kind %q", s.Kind)
	}
	return nil
}

func plainOrEmpty(s string) error {
	if s == "" {
		return nil
	}
	return plainText(s, 1, MaxDetailRunes)
}

// Strings are the manifest's `strings`: the add-on's labels by locale, as
// {"de": {"action.send": "Senden"}}. Keys name a label: `action.<id>`,
// `command.<id>`, `command.<id>.keywords`, `setting.<id>`,
// `setting.<id>.description` and `option.<setting id>.<value>`. The manifest's
// own text is the English fallback. They sit outside Sona's generated
// catalogues.
type Strings map[string]map[string]string

func (s Strings) validate(c Contributions) error {
	if len(s) > MaxLocales {
		return problem(ProblemStrings, "%d locales", len(s))
	}
	known := map[string]bool{}
	for _, a := range c.Actions {
		known["action."+a.ID] = true
	}
	for _, a := range c.Commands {
		known["command."+a.ID] = true
		if a.Keywords != "" {
			known["command."+a.ID+".keywords"] = true
		}
	}
	for _, r := range c.Settings {
		known["setting."+r.ID] = true
		known["setting."+r.ID+".description"] = r.Description != ""
		for _, o := range r.Options {
			known["option."+r.ID+"."+o.Value] = true
		}
	}
	for locale, m := range s {
		if !localeExpr.MatchString(locale) || len(m) > MaxStrings {
			return problem(ProblemStrings, "locale %q", locale)
		}
		for k, v := range m {
			if !known[k] {
				return problem(ProblemStrings, "%s: no such label %q", locale, k)
			}
			if err := plainText(v, 1, MaxDetailRunes); err != nil {
				return problem(ProblemStrings, "%s %s: %v", locale, k, err)
			}
		}
	}
	return nil
}

// Text is the label with this key (`action.send`…) for the locale ("de",
// "pt-BR"): the exact locale's, else its language's, else fallback.
func (m Manifest) Text(locale, key, fallback string) string {
	for _, l := range localeChain(locale) {
		if v, ok := m.Strings[l][key]; ok {
			return v
		}
	}
	return fallback
}

func localeChain(locale string) []string {
	locale = strings.ReplaceAll(locale, "_", "-")
	out := []string{locale}
	if lang, _, ok := strings.Cut(locale, "-"); ok {
		out = append(out, lang)
	}
	return out
}

// SettingDefault is a row's value before the user sets one.
func (s Setting) SettingDefault() string {
	switch {
	case s.Kind == SettingSecret:
		return ""
	case s.Default != "":
		return s.Default
	case s.Kind == SettingToggle:
		return "false"
	case s.Kind == SettingChoice:
		return s.Options[0].Value
	}
	return ""
}

// ValidValue reports whether v is a value the row may hold (the page and
// the service check what a user or a stale page sends).
func (s Setting) ValidValue(v string) error {
	switch s.Kind {
	case SettingToggle:
		if v != "true" && v != "false" {
			return fmt.Errorf("a toggle is true or false")
		}
	case SettingChoice:
		if !slices.ContainsFunc(s.Options, func(o Option) bool { return o.Value == v }) {
			return fmt.Errorf("%q is not one of the options", v)
		}
	case SettingText, SettingSecret:
		if len(v) > MaxValueBytes {
			return fmt.Errorf("longer than %d bytes", MaxValueBytes)
		}
	}
	return nil
}

// MaxValueBytes bounds what a text or secret row holds.
const MaxValueBytes = 1024
