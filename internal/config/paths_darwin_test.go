package config

import "testing"

func TestResolvePathsMacOS(t *testing.T) {
	p, err := ResolvePaths(envMap(map[string]string{"HOME": "/Users/u", "XDG_CONFIG_HOME": "/x/config"}))
	if err != nil {
		t.Fatal(err)
	}
	want := Paths{
		Config: "/Users/u/Library/Application Support/Sona",
		Data:   "/Users/u/Library/Application Support/Sona",
		Cache:  "/Users/u/Library/Caches/Sona",
		Art:    "/Users/u/Library/Caches/Sona/art",
		State:  "/Users/u/Library/Application Support/Sona/State",
	}
	if p != want {
		t.Fatalf("got %+v, want %+v", p, want)
	}
	if _, err := ResolvePaths(envMap(map[string]string{"HOME": "relative"})); err == nil {
		t.Fatal("want error for a relative HOME")
	}
}
