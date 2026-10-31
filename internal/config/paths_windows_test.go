package config

import "testing"

func TestResolvePathsWindows(t *testing.T) {
	p, err := ResolvePaths(envMap(map[string]string{
		"APPDATA":      `C:\Users\u\AppData\Roaming`,
		"LOCALAPPDATA": `C:\Users\u\AppData\Local`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := Paths{
		Config: `C:\Users\u\AppData\Roaming\Sona`,
		Data:   `C:\Users\u\AppData\Local\Sona`,
		Cache:  `C:\Users\u\AppData\Local\Sona\Cache`,
		Art:    `C:\Users\u\AppData\Local\Sona\Cache\art`,
		State:  `C:\Users\u\AppData\Local\Sona\State`,
	}
	if p != want {
		t.Fatalf("got %+v, want %+v", p, want)
	}
	if _, err := ResolvePaths(envMap(map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`})); err == nil {
		t.Fatal("want error without LOCALAPPDATA")
	}
}
