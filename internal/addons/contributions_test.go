package addons

import (
	"strings"
	"testing"
)

const contribManifest = `{
 "id":"demo","name":"Demo","version":"1.0.0","command":"run","permissions":["playback.read"],
 "contributions":{
  "actions":[{"id":"send","label":"Send to webhook","kinds":["song","album"],"icon":"send"}],
  "commands":[{"id":"refresh","label":"Refresh the webhook","keywords":"post url"}],
  "settings":[
   {"id":"url","kind":"text","label":"Web address","description":"Where the song is posted."},
   {"id":"enabled","kind":"toggle","label":"Post songs","default":"true"},
   {"id":"mode","kind":"choice","label":"Format","options":[{"value":"json","label":"JSON"},{"value":"text","label":"Plain text"}],"default":"text"},
   {"id":"token","kind":"secret","label":"Access token"}
  ]},
 "strings":{"de":{"action.send":"An Webhook senden","setting.url":"Webadresse","option.mode.json":"JSON"},"pt-BR":{"command.refresh":"Atualizar"}}
}`

func TestManifestWithContributions(t *testing.T) {
	m, err := ParseManifest([]byte(contribManifest))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Contributions.Actions) != 1 || len(m.Contributions.Commands) != 1 || len(m.Contributions.Settings) != 4 {
		t.Fatalf("contributions = %+v", m.Contributions)
	}
	if !m.Contributions.Has("send") || !m.Contributions.Has("refresh") || m.Contributions.Has("url") || m.Contributions.Has("evil") {
		t.Error("Has answers wrongly: only actions and commands are invocable")
	}
	for _, tc := range []struct{ locale, key, want string }{
		{"de", "action.send", "An Webhook senden"},
		{"de-AT", "action.send", "An Webhook senden"}, // the language's
		{"pt-BR", "command.refresh", "Atualizar"},
		{"pt", "command.refresh", "English"}, // not the other way round
		{"fr", "action.send", "English"},
		{"", "action.send", "English"},
	} {
		if got := m.Text(tc.locale, tc.key, "English"); got != tc.want {
			t.Errorf("Text(%q, %q) = %q, want %q", tc.locale, tc.key, got, tc.want)
		}
	}
	for id, want := range map[string]string{"url": "", "enabled": "true", "mode": "text", "token": ""} {
		for _, s := range m.Contributions.Settings {
			if s.ID == id && s.SettingDefault() != want {
				t.Errorf("default of %s = %q, want %q", id, s.SettingDefault(), want)
			}
		}
	}
}

func TestContributionsRefused(t *testing.T) {
	with := func(old, new string) string { return strings.Replace(contribManifest, old, new, 1) }
	many := func(n int, item string) string { return strings.TrimSuffix(strings.Repeat(item+",", n), ",") }
	for name, tc := range map[string]struct{ text, code string }{
		"an action id with a dot":       {with(`"id":"send"`, `"id":"send.x"`), ProblemContribution},
		"an action id up the tree":      {with(`"id":"send"`, `"id":"../x"`), ProblemContribution},
		"an action id that spoofs":      {with(`"id":"send"`, `"id":"addon.other.send"`), ProblemContribution},
		"an action id of Sona's":        {with(`"id":"send"`, `"id":"player.play-next"`), ProblemContribution},
		"upper-case id":                 {with(`"id":"send"`, `"id":"Send"`), ProblemContribution},
		"two actions of one id":         {with(`"kinds":["song","album"],"icon":"send"}`, `"kinds":["song"]},{"id":"send","label":"b","kinds":["song"]}`), ProblemContribution},
		"an action and a command alike": {with(`"id":"refresh"`, `"id":"send"`), ProblemContribution},
		"no kinds":                      {with(`"kinds":["song","album"]`, `"kinds":[]`), ProblemContribution},
		"a station action":              {with(`"kinds":["song","album"]`, `"kinds":["station"]`), ProblemContribution},
		"a kind twice":                  {with(`"kinds":["song","album"]`, `"kinds":["song","song"]`), ProblemContribution},
		"bidi in a label":               {with(`Send to webhook`, `Send\u202e to webhook`), ProblemContribution},
		"a newline in a label":          {with(`Send to webhook`, `Send\nto webhook`), ProblemContribution},
		"a long label":                  {with(`Send to webhook`, strings.Repeat("x", MaxLabelRunes+1)), ProblemContribution},
		"an icon with a path":           {with(`"icon":"send"`, `"icon":"../../etc/passwd"`), ProblemContribution},
		"an image as an icon":           {with(`"icon":"send"`, `"icon":"data:image/png;base64,AAAA"`), ProblemContribution},
		"a toggle with options":         {with(`"id":"enabled","kind":"toggle"`, `"id":"enabled","kind":"toggle","options":[{"value":"a","label":"A"}]`), ProblemContribution},
		"a bad toggle default":          {with(`"default":"true"`, `"default":"yes"`), ProblemContribution},
		"a choice default not offered":  {with(`"default":"text"`, `"default":"xml"`), ProblemContribution},
		"a choice of one":               {with(`,{"value":"text","label":"Plain text"}`, ``), ProblemContribution},
		"an option value with space":    {with(`"value":"json"`, `"value":"j son"`), ProblemContribution},
		"a secret with a default":       {with(`"id":"token","kind":"secret"`, `"id":"token","kind":"secret","default":"x"`), ProblemContribution},
		"an unknown kind":               {with(`"kind":"secret"`, `"kind":"script"`), ProblemContribution},
		"an unknown field":              {with(`"keywords":"post url"`, `"keywords":"post url","run":"rm -rf /"`), ProblemManifest},
		"too many actions":              {with(`"actions":[{"id":"send","label":"Send to webhook","kinds":["song","album"],"icon":"send"}]`, `"actions":[`+many(MaxActions+1, `{"id":"a","label":"x","kinds":["song"]}`)+`]`), ProblemContribution},
		"strings for nothing":           {with(`"action.send":"An Webhook senden"`, `"action.nope":"x"`), ProblemStrings},
		"a bad locale":                  {with(`"de":{`, `"../de":{`), ProblemStrings},
		"bidi in strings":               {with(`An Webhook senden`, `An\u202e Webhook`), ProblemStrings},
		"a long string":                 {with(`An Webhook senden`, strings.Repeat("x", MaxDetailRunes+1)), ProblemStrings},
	} {
		t.Run(name, func(t *testing.T) {
			// Several of the cases above rely on a bad shape the generic JSON
			// check would also catch; the code asserted is the reason Sona gives.
			_, err := ParseManifest([]byte(tc.text))
			if got := ProblemOf(err); err == nil || got != tc.code {
				t.Errorf("err = %v (%q), want %q", err, got, tc.code)
			}
		})
	}
}

func TestActionIDs(t *testing.T) {
	if ActionID("demo", "send") != "addon.demo.send" {
		t.Error("ActionID")
	}
	for action, want := range map[string]bool{
		"addon.demo.send": true, "addon.demo.": false, "addon.other.send": false, "player.play-next": false,
		"addon.demo.send.x": false, "addon.demo.Send": false, "addon.demo-x.send": false, "demo.send": false,
	} {
		if _, ok := LocalID("demo", action); ok != want {
			t.Errorf("LocalID(demo, %q) ok = %v, want %v", action, ok, want)
		}
	}
}

func TestValidValue(t *testing.T) {
	m, err := ParseManifest([]byte(contribManifest))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Setting{}
	for _, s := range m.Contributions.Settings {
		byID[s.ID] = s
	}
	for _, tc := range []struct {
		row, v string
		ok     bool
	}{
		{"enabled", "true", true},
		{"enabled", "maybe", false},
		{"mode", "json", true},
		{"mode", "xml", false},
		{"url", "https://example.com", true},
		{"url", strings.Repeat("x", MaxValueBytes+1), false},
		{"token", "s3cret", true},
		{"token", strings.Repeat("x", MaxValueBytes+1), false},
	} {
		if err := byID[tc.row].ValidValue(tc.v); (err == nil) != tc.ok {
			t.Errorf("%s=%.12q: %v", tc.row, tc.v, err)
		}
	}
}
