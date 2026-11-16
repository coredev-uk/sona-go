//go:build unix

package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/coredev-uk/sona-go/internal/addons"
)

// memKeyring is an in-memory Secret Service.
type memKeyring struct {
	mu    sync.Mutex
	items map[string]string
}

func (k *memKeyring) Get(service, user string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.items[service+"/"+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (k *memKeyring) Set(service, user, password string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.items == nil {
		k.items = map[string]string{}
	}
	k.items[service+"/"+user] = password
	return nil
}

func (k *memKeyring) Delete(service, user string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.items[service+"/"+user]; !ok {
		return keyring.ErrNotFound
	}
	delete(k.items, service+"/"+user)
	return nil
}

func (k *memKeyring) all() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	var b strings.Builder
	for key, v := range k.items {
		b.WriteString(key + "=" + v + "\n")
	}
	return b.String()
}

const rowsManifest = `{
 "actions":[{"id":"send","label":"Send to webhook","kinds":["song","album"],"icon":"send"}],
 "commands":[{"id":"refresh","label":"Refresh","keywords":"post"}],
 "settings":[
  {"id":"url","kind":"text","label":"Web address","description":"Where it goes."},
  {"id":"post","kind":"toggle","label":"Post songs","default":"true"},
  {"id":"format","kind":"choice","label":"Format","options":[{"value":"json","label":"JSON"},{"value":"text","label":"Text"}]},
  {"id":"token","kind":"secret","label":"Token"}
 ]}`

func rowsEnv(t *testing.T) (*addonEnv, *memKeyring) {
	t.Helper()
	a := newAddonEnv(t)
	kr := &memKeyring{}
	a.e.Keyring = kr
	return a, kr
}

func TestAddonsContributionsInTheUsersLanguage(t *testing.T) {
	a, _ := rowsEnv(t)
	m := strings.Replace(manifestWith("demo", rowsManifest), `"contributions"`, `"strings":{"de":{"action.send":"An Webhook senden","setting.url":"Webadresse","option.format.text":"Text (de)"}},"contributions"`, 1)
	a.install(t, "demo", m, hello("demo")+"sleep 300\n")
	c := a.svc.Contributions("de")
	if len(c) != 1 {
		t.Fatalf("contributions = %+v", c)
	}
	// Not running: its rows are there to set up, its actions and commands are not.
	if c[0].Running || len(c[0].Actions) != 0 || len(c[0].Commands) != 0 || len(c[0].Rows) != 4 {
		t.Errorf("stopped add-on = %+v", c[0])
	}
	rows := map[string]AddonRow{}
	for _, r := range c[0].Rows {
		rows[r.ID] = r
	}
	if rows["url"].Label != "Webadresse" || rows["post"].Label != "Post songs" || rows["post"].Value != "true" || rows["format"].Value != "json" {
		t.Errorf("rows = %+v", rows)
	}
	if rows["format"].Options[1].Label != "Text (de)" {
		t.Errorf("options = %+v", rows["format"].Options)
	}

	if err := a.svc.Enable("demo", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	a.waitState(t, "demo", AddonRunning)
	c = a.svc.Contributions("de")
	if !c[0].Running || len(c[0].Actions) != 1 || len(c[0].Commands) != 1 {
		t.Fatalf("running add-on = %+v", c[0])
	}
	if act := c[0].Actions[0]; act.ID != "addon.demo.send" || act.Label != "An Webhook senden" || act.Icon != "send" || len(act.Kinds) != 2 {
		t.Errorf("action = %+v", act)
	}
	if cmd := c[0].Commands[0]; cmd.ID != "addon.demo.refresh" || cmd.Label != "Refresh" || cmd.Keywords != "post" {
		t.Errorf("command = %+v", cmd)
	}
	if c := a.svc.Contributions("fr")[0]; c.Actions[0].Label != "Send to webhook" {
		t.Errorf("fallback = %q", c.Actions[0].Label)
	}
	// A refused add-on contributes nothing.
	a.install(t, "bad", `{"id":"bad","name":"x","version":"1.0.0","command":"run","permissions":[],"contributions":{"actions":[{"id":"x.y","label":"x","kinds":["song"]}]}}`, "exit 0\n")
	for _, c := range a.svc.Contributions("en") {
		if c.ID == "bad" {
			t.Error("a refused add-on contributes")
		}
	}
}

func TestAddonsSettingsValuesAndSecrets(t *testing.T) {
	a, kr := rowsEnv(t)
	a.install(t, "demo", manifestWith("demo", rowsManifest), "exit 0\n")
	row := func(id string) AddonRow {
		for _, r := range a.svc.Contributions("en")[0].Rows {
			if r.ID == id {
				return r
			}
		}
		t.Fatalf("no row %s", id)
		return AddonRow{}
	}
	if err := a.svc.SetValue("demo", "url", "https://example.com/hook"); err != nil {
		t.Fatal(err)
	}
	if err := a.svc.SetValue("demo", "post", "false"); err != nil {
		t.Fatal(err)
	}
	if v := row("url").Value; v != "https://example.com/hook" || row("post").Value != "false" {
		t.Errorf("values = %q %q", v, row("post").Value)
	}
	if v := a.settings.Get().Addons["demo"].Values; v["url"] != "https://example.com/hook" {
		t.Errorf("saved values = %v", v)
	}
	for name, tc := range map[string][2]string{
		"a bad toggle": {"post", "maybe"}, "a bad choice": {"format", "xml"}, "a secret by the value call": {"token", "s3cret"},
		"a long value": {"url", strings.Repeat("x", addons.MaxValueBytes+1)}, "an unknown row": {"nope", "x"},
	} {
		if err := a.svc.SetValue("demo", tc[0], tc[1]); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := a.svc.SetValue("ghost", "url", "x"); CodeOf(err) != CodeNotFound {
		t.Errorf("unknown add-on: %v", err)
	}

	// A secret goes to the keyring and nowhere else.
	const secret = "sample-secret-value"
	if err := a.svc.SetSecret("demo", "token", secret); err != nil {
		t.Fatal(err)
	}
	if r := row("token"); !r.Saved || r.Value != "" {
		t.Errorf("secret row = %+v: it must say saved and never carry the value", r)
	}
	if !strings.Contains(kr.all(), secret) || !strings.Contains(kr.all(), "addon.demo") {
		t.Errorf("keyring: %q", kr.all())
	}
	settingsJSON, err := json.Marshal(a.settings.Get())
	if err != nil || strings.Contains(string(settingsJSON), secret) {
		t.Errorf("the secret reached the settings: %s (%v)", settingsJSON, err)
	}
	if err := a.svc.SetSecret("demo", "url", "x"); err == nil {
		t.Error("a text row was set as a secret")
	}
	if err := a.svc.SetSecret("demo", "token", ""); err != nil {
		t.Fatal(err)
	}
	if r := row("token"); r.Saved {
		t.Errorf("cleared secret still saved: %+v", r)
	}
}

// A running add-on hears its settings, learns of a change, and reads its own
// secret; the host never gives one add-on another's.
func TestAddonsReceiveTheirSettingsAndSecrets(t *testing.T) {
	a, _ := rowsEnv(t)
	dir := a.install(t, "demo", manifestWith("demo", rowsManifest), hello("demo")+`
exec 3<&0
cat <&3 > in.log &
printf '%s\n' '{"id":2,"method":"subscribe","params":{"topics":["settings"]}}'
printf '%s\n' '{"id":3,"method":"secret","params":{"key":"token"}}'
printf '%s\n' '{"id":4,"method":"secret","params":{"key":"url"}}'
printf '%s\n' '{"id":5,"method":"secret","params":{"key":"other-addons-token"}}'
wait
`)
	if err := a.svc.SetSecret("demo", "token", "tok-xyz"); err != nil {
		t.Fatal(err)
	}
	if err := a.svc.Enable("demo", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	in := filepath.Join(dir, "in.log")
	waitUntil(t, "the first settings", fileHas(in, `"event":"settings","data":{"values":{"format":"json","post":"true","url":""}}`))
	waitUntil(t, "the secret", fileHas(in, `"result":{"value":"tok-xyz"}`))
	// A text row is not a secret; a row the manifest never declared is not either.
	b, _ := os.ReadFile(in)
	if n := strings.Count(string(b), `"code":"not_found"`); n != 2 {
		t.Errorf("not_found answers = %d, want 2:\n%s", n, b)
	}
	if strings.Contains(string(b), "tok-xyz") && strings.Count(string(b), "tok-xyz") != 1 {
		t.Errorf("the secret appears more than once:\n%s", b)
	}
	if err := a.svc.SetValue("demo", "url", "https://example.com/new"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the changed settings", fileHas(in, `"url":"https://example.com/new"`))
	if b, _ := os.ReadFile(in); strings.Contains(string(b), "token\":") && strings.Contains(string(b), `"values":{"token`) {
		t.Errorf("a secret row is in the settings event:\n%s", b)
	}
}

// What an add-on answers to an invoke is text: control characters and
// bidirectional marks never reach the toast, and it is cut.
func TestAddonsInvokeAnswerIsCleaned(t *testing.T) {
	a, _ := rowsEnv(t)
	a.install(t, "loud", manifestWith("loud", sendAction), hello("loud")+`
while read -r line; do
  case $line in
    *'"method":"invoke"'*)
      id=${line#'{"id":'}; id=${id%%,*}
      printf '{"id":%s,"result":{"toast":"Done\\u001b[31m red \\u202egnp.exe"}}\n' "$id" ;;
  esac
done
`)
	if err := a.svc.Enable("loud", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	a.waitState(t, "loud", AddonRunning)
	res, err := a.svc.Invoke(t.Context(), "loud", "addon.loud.ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(res.Toast, "\x1b\u202e") || !strings.HasPrefix(res.Toast, "Done") {
		t.Errorf("toast = %q", res.Toast)
	}
}
