package applestrings

import (
	"maps"
	"testing"
)

func TestParseBundleModule(t *testing.T) {
	src := `const e="Back",i='Done',o={"ASE.Web.Music.A":"Caf\u00e9 \"quoted\"","ASE.Web.Music.B":"Line\nnext \x41 \ud83c\udfb5 \u{1F3B5}",Back:e,"ASE.Done":i,Next:"Weiter"};export{e as Back,i as Done,o as default};`
	got, err := ParseBundleModule(src)
	if err != nil {
		t.Fatal(err)
	}
	want := Bundle{
		"ASE.Web.Music.A": `Café "quoted"`,
		"ASE.Web.Music.B": "Line\nnext A 🎵 🎵",
		"Back":            "Back",
		"ASE.Done":        "Done",
		"Next":            "Weiter",
	}
	if !maps.Equal(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestParseBundleModuleRejectsCode(t *testing.T) {
	for _, src := range []string{
		`const o={"A":fetch("x")};export{o as default};`,
		`const o={"A":"x"};`,
		"const o={\"A\":`${x}`};export{o as default};",
		`const o={"A":unknown};export{o as default};`,
		`const o={"A":1};export{o as default};`,
		`const o=["A"];export{o as default};`,
	} {
		if _, err := ParseBundleModule(src); err == nil {
			t.Errorf("ParseBundleModule(%q) accepted", src)
		}
	}
}

func TestLocaleBundles(t *testing.T) {
	js := []byte(`Object.assign({"../../tmp/locales/ar/translations.json":()=>Ke(()=>import("./translations~c0cdb3abc3.js"),[]),"../../tmp/locales/zh-hans-cn/translations.json":()=>Ke(()=>import("./translations~9239ef5edb.js"),[])})`)
	got := localeBundles(js, Origin)
	want := map[string]string{
		"ar":         "https://music.apple.com/assets/translations~c0cdb3abc3.js",
		"zh-hans-cn": "https://music.apple.com/assets/translations~9239ef5edb.js",
	}
	if !maps.Equal(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestBCP47(t *testing.T) {
	for in, want := range map[string]string{"zh-hans-cn": "zh-Hans-CN", "pt-br": "pt-BR", "es-419": "es-419", "ar": "ar"} {
		if got := BCP47(in); got != want {
			t.Errorf("BCP47(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchLocale(t *testing.T) {
	apple := []string{"ar", "de-ch", "de-de", "fr-ca", "fr-fr", "ko", "ko-kr", "pt-br", "pt-pt", "zh-hans-cn", "zh-hant-hk", "zh-hant-tw"}
	for want, got := range map[string]string{
		"fr-CA":      "fr-ca",
		"fr":         "fr-fr",
		"fr-BE":      "fr-fr",
		"de-AT":      "de-de",
		"pt-BR":      "pt-br",
		"ko-KR":      "ko-kr",
		"ko":         "ko",
		"zh-TW":      "zh-hant-tw",
		"zh-Hant":    "zh-hant-hk",
		"zh-CN":      "zh-hans-cn",
		"zh-Hans-SG": "zh-hans-cn",
		"ja":         "",
		"":           "",
	} {
		if m := MatchLocale(want, apple); m != got {
			t.Errorf("MatchLocale(%q) = %q, want %q", want, m, got)
		}
	}
}

func TestSkip(t *testing.T) {
	for tag, skip := range map[string]bool{"dev": true, "en": true, "en-GB": true, "fr-fr": false, "es-419": false} {
		if got := Skip(tag) != ""; got != skip {
			t.Errorf("Skip(%q) = %v, want %v", tag, got, skip)
		}
	}
}
