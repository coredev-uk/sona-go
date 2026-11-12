package main

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestRelevant(t *testing.T) {
	root := filepath.FromSlash("/src/sona")
	p := func(s string) string { return filepath.Join(root, filepath.FromSlash(s)) }
	l := &loop{cfg: config{root: root}}

	all := []string{p("internal/ui/uitest/x.go"), p("internal/ui/app/app.go")}
	if got := l.relevant(all); !slices.Equal(got, all) {
		t.Fatalf("before the first build every change counts: %v", got)
	}

	l.deps = map[string]bool{p("internal/ui/app"): true, p("internal/ui/theme"): true, p("cmd/sona"): true}
	got := l.relevant([]string{
		p("internal/ui/uitest/x.go"),       // not imported by the target
		p("internal/ui/app/app.go"),        // a package of the target
		p("internal/ui/theme/icons/a.svg"), // embedded below one
		p("go.mod"),                        // the module's
		p("cmd/uigallery/main.go"),         // another command
	})
	want := []string{p("internal/ui/app/app.go"), p("internal/ui/theme/icons/a.svg"), p("go.mod")}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRebuilds(t *testing.T) {
	if rebuilds([]string{"a/tokens.json", "l/fr.po"}) {
		t.Error("tokens and catalogues reload live: no build")
	}
	if !rebuilds([]string{"a/tokens.json", "a/x.go"}) {
		t.Error("a Go file builds")
	}
}

func TestAppEnv(t *testing.T) {
	env := []string{"A=1", "SONA_SOURCE="}
	has := func(env []string, kv string) bool { return slices.Contains(env, kv) }

	got := appEnv(config{root: "/src"}, env)
	if !has(got, "SONA_SOURCE=/src") || has(got, "MYGO_DEV_BACKGROUND=1") {
		t.Errorf("plain: %v, want SONA_SOURCE and no background", got)
	}
	if len(env) != 2 {
		t.Errorf("the loop's own environment was changed: %v", env)
	}

	got = appEnv(config{root: "/src", background: true}, []string{"SONA_SOURCE=/mine"})
	if !has(got, "MYGO_DEV_BACKGROUND=1") || has(got, "SONA_SOURCE=/src") || !has(got, "SONA_SOURCE=/mine") {
		t.Errorf("background: %v, want MYGO_DEV_BACKGROUND=1 and the loop's own SONA_SOURCE kept", got)
	}
}

func TestExtldProbeCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	probe := func(tmp string) []string {
		return []string{"-m64", "-o", filepath.Join(tmp, "a.out"), "-Wl,--build-id=0x1234567890abcdef", filepath.Join(tmp, "trivial.c")}
	}
	k1, k2 := probeKey("sh", probe("/tmp/go-link-1")), probeKey("sh", probe("/tmp/go-link-2"))
	if k1 == "" || k1 != k2 {
		t.Fatalf("one probe in two temporary directories must share a key: %q %q", k1, k2)
	}
	if k := probeKey("sh", []string{"-m64", "-o", "/tmp/x/a.out", "obj.o"}); k != "" {
		t.Fatalf("a real link is not a probe: %q", k)
	}
	if k := probeKey("sh", append(probe("/tmp/go-link-1"), "-Wl,-z,relro")); k == k1 {
		t.Fatal("another flag, another key")
	}

	if _, ok := loadProbe(k1); ok {
		t.Fatal("nothing stored yet")
	}
	storeProbe(k1, probeResult{Code: 1, Output: "unrecognized command line option"})
	r, ok := loadProbe(k1)
	if !ok || r.Code != 1 || r.Output != "unrecognized command line option" {
		t.Fatalf("got %+v %v", r, ok)
	}
}
