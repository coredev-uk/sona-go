package main

import (
	"maps"
	"testing"
)

func TestParseBundleModule(t *testing.T) {
	src := `const e="Back",i='Done',o={"ASE.Web.Music.A":"Caf\u00e9 \"quoted\"","ASE.Web.Music.B":"Line\nnext \x41 \ud83c\udfb5 \u{1F3B5}",Back:e,"ASE.Done":i,Next:"Weiter"};export{e as Back,i as Done,o as default};`
	got, err := parseBundleModule(src)
	if err != nil {
		t.Fatal(err)
	}
	want := appleBundle{
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
	} {
		if _, err := parseBundleModule(src); err == nil {
			t.Errorf("parseBundleModule(%q) accepted", src)
		}
	}
}

func TestLocaleBundles(t *testing.T) {
	js := []byte(`Object.assign({"../../tmp/locales/ar/translations.json":()=>Ke(()=>import("./translations~c0cdb3abc3.js"),[]),"../../tmp/locales/zh-hans-cn/translations.json":()=>Ke(()=>import("./translations~9239ef5edb.js"),[])})`)
	got := localeBundles(js)
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
		if got := bcp47(in); got != want {
			t.Errorf("bcp47(%q) = %q, want %q", in, got, want)
		}
	}
}
