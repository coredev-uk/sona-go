package addons

import (
	"strings"
	"testing"
)

const goodManifest = `{"id":"demo","name":"Demo","version":"1.2.3","command":"bin/run","permissions":["playback.read"]}`

func TestParseManifest(t *testing.T) {
	m, err := ParseManifest([]byte(goodManifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "demo" || m.Name != "Demo" || m.Version != "1.2.3" || m.Command != "bin/run" || len(m.Permissions) != 1 {
		t.Errorf("manifest = %+v", m)
	}
}

func TestParseManifestRefuses(t *testing.T) {
	with := func(field, value string) string {
		return strings.Replace(goodManifest, field, value, 1)
	}
	for name, tc := range map[string]struct{ text, code string }{
		"unknown field":      {strings.Replace(goodManifest, "}", `,"sandbox":"wasm"}`, 1), ProblemManifest},
		"not json":           {`id: demo`, ProblemManifest},
		"two documents":      {goodManifest + goodManifest, ProblemManifest},
		"wrong type":         {with(`"permissions":["playback.read"]`, `"permissions":"all"`), ProblemManifest},
		"too large":          {`{"id":"demo","description":"` + strings.Repeat("x", MaxManifestBytes) + `"}`, ProblemTooLarge},
		"upper-case id":      {with(`"id":"demo"`, `"id":"Demo"`), ProblemID},
		"id with a dot":      {with(`"id":"demo"`, `"id":"de.mo"`), ProblemID},
		"id too short":       {with(`"id":"demo"`, `"id":"d"`), ProblemID},
		"id too long":        {with(`"id":"demo"`, `"id":"a`+strings.Repeat("b", 40)+`"`), ProblemID},
		"id starts digit":    {with(`"id":"demo"`, `"id":"1demo"`), ProblemID},
		"no name":            {with(`"name":"Demo"`, `"name":""`), ProblemName},
		"long name":          {with(`"name":"Demo"`, `"name":"`+strings.Repeat("n", MaxNameRunes+1)+`"`), ProblemName},
		"bidi in name":       {with(`"name":"Demo"`, `"name":"Demo\u202egpj.exe"`), ProblemName},
		"newline in name":    {with(`"name":"Demo"`, `"name":"De\nmo"`), ProblemName},
		"bad version":        {with(`"1.2.3"`, `"latest"`), ProblemVersion},
		"two-part version":   {with(`"1.2.3"`, `"1.2"`), ProblemVersion},
		"absolute command":   {with(`bin/run`, `/bin/sh`), ProblemCommand},
		"parent command":     {with(`bin/run`, `../run`), ProblemCommand},
		"inner parent":       {with(`bin/run`, `bin/../../run`), ProblemCommand},
		"backslash command":  {with(`bin/run`, `bin\\run`), ProblemCommand},
		"drive command":      {with(`bin/run`, `C:run`), ProblemCommand},
		"empty command":      {with(`bin/run`, ``), ProblemCommand},
		"dot element":        {with(`bin/run`, `bin/./run`), ProblemCommand},
		"unknown permission": {with(`"playback.read"`, `"root"`), ProblemPermission},
		"repeated":           {with(`["playback.read"]`, `["playback.read","playback.read"]`), ProblemPermission},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseManifest([]byte(tc.text))
			if got := ProblemOf(err); err == nil || got != tc.code {
				t.Errorf("err = %v (%q), want problem %q", err, got, tc.code)
			}
		})
	}
}

func TestReservedIDs(t *testing.T) {
	for _, id := range []string{"lastfm", "listenbrainz", "discord", "sona", "sona-extras", "sonar"} {
		if !Reserved(id) {
			t.Errorf("%q is not reserved", id)
		}
	}
	for _, id := range []string{"demo", "my-sona", "lastfm-plus"} {
		if Reserved(id) {
			t.Errorf("%q is reserved", id)
		}
	}
}

const wasmManifest = `{"id":"demo","name":"Demo","version":"1.2.3","command":"bin/run.wasm","runtime":"wasm","permissions":["playback.read"]}`

func TestManifestRuntime(t *testing.T) {
	m, err := ParseManifest([]byte(wasmManifest))
	if err != nil || !m.Wasm() {
		t.Fatalf("wasm manifest = %+v, %v", m, err)
	}
	if m, err := ParseManifest([]byte(strings.Replace(goodManifest, "}", `,"runtime":"native"}`, 1))); err != nil || m.Wasm() {
		t.Errorf("native manifest = %+v, %v", m, err)
	}
	if m, err := ParseManifest([]byte(goodManifest)); err != nil || m.Wasm() || m.Runtime != "" {
		t.Errorf("a manifest with no runtime = %+v, %v: want the native default", m, err)
	}
	for name, tc := range map[string]struct{ text, code string }{
		"unknown runtime":      {strings.Replace(wasmManifest, `"wasm","`, `"docker","`, 1), ProblemRuntime},
		"upper-case runtime":   {strings.Replace(wasmManifest, `"wasm","`, `"WASM","`, 1), ProblemRuntime},
		"module not .wasm":     {strings.Replace(wasmManifest, `bin/run.wasm`, `bin/run`, 1), ProblemCommand},
		"module in upper case": {strings.Replace(wasmManifest, `bin/run.wasm`, `bin/run.WASM`, 1), ProblemCommand},
		"module that escapes":  {strings.Replace(wasmManifest, `bin/run.wasm`, `../run.wasm`, 1), ProblemCommand},
		// WASI has no sockets: the permission would promise what a module can't use.
		"network for a module": {strings.Replace(wasmManifest, `["playback.read"]`, `["playback.read","network"]`, 1), ProblemPermission},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseManifest([]byte(tc.text))
			if got := ProblemOf(err); err == nil || got != tc.code {
				t.Errorf("err = %v (%q), want problem %q", err, got, tc.code)
			}
		})
	}
}
