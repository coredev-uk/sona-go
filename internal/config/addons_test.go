package config

import (
	"reflect"
	"strings"
	"testing"
)

// A settings file someone hands you must not be able to approve code to run:
// an import never sets, enables or grants an add-on, in any spelling.
func TestImportCannotEnableOrGrantAnAddon(t *testing.T) {
	for _, doc := range []string{
		`{"addons":{"evil":{"enabled":true,"granted":["playback.control","like"]}}}`,
		`{"Addons":{"EVIL":{"Enabled":true,"Granted":["open"]}}}`,
		`{"ADDONS":{"demo":{"enabled":true,"granted":["open","like"]}}}`,
	} {
		im, err := ParseExport([]byte(`{"format":"sona-settings","version":1,"settings":` + doc + `}`))
		if err != nil {
			t.Fatal(err)
		}
		here := Defaults()
		here.Addons = Addons{"demo": {Enabled: false, Granted: []string{"playback.read"}}}
		got := here
		got.Addons = here.Addons.clone()
		im.Apply(&got)
		if !reflect.DeepEqual(got.Normalized().Addons, here.Addons) {
			t.Errorf("%s: addons became %+v", doc, got.Addons)
		}
	}
}

func TestExportLeavesOutAddons(t *testing.T) {
	s := Defaults()
	s.Addons = Addons{"demo": {Enabled: true, Granted: []string{"playback.read"}}}
	b, err := Export(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "addons") || strings.Contains(string(b), "playback.read") {
		t.Errorf("export has add-ons:\n%s", b)
	}
}

func TestAddonsNormalise(t *testing.T) {
	if (Addons{}).normalized() != nil || Addons(nil).normalized() != nil {
		t.Error("an empty set is nil")
	}
	long := strings.Repeat("x", maxAddonIDLength+1)
	in := Addons{
		"b":  {Enabled: true, Granted: []string{"queue.read", "", "playback.read", "queue.read"}},
		"":   {Enabled: true},
		long: {Enabled: true},
		"c":  {Granted: []string{}},
	}
	got := in.normalized()
	want := Addons{"b": {Enabled: true, Granted: []string{"playback.read", "queue.read"}}, "c": {}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalized = %+v, want %+v", got, want)
	}
	big := Addons{}
	for i := range MaxAddons + 20 {
		big[strings.Repeat("a", 3)+string(rune('a'+i/26))+string(rune('a'+i%26))] = AddonSetting{}
	}
	if n := len(big.normalized()); n != MaxAddons {
		t.Errorf("kept %d add-ons, want %d", n, MaxAddons)
	}
}

func TestStoreHandsOutCopiesOfAddons(t *testing.T) {
	st, err := OpenStore(t.TempDir() + "/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(s *Settings) { s.Addons = Addons{"demo": {Enabled: true, Granted: []string{"open"}}} }); err != nil {
		t.Fatal(err)
	}
	a := st.Get()
	a.Addons["demo"] = AddonSetting{}
	a.Addons["x"] = AddonSetting{}
	if b := st.Get(); !b.Addons["demo"].Enabled || len(b.Addons) != 1 {
		t.Errorf("a copy's edit reached the store: %+v", b.Addons)
	}
}
