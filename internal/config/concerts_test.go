package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConcertsCityIsKeptOrFallsBackToTheDefault(t *testing.T) {
	if got := Defaults().Concerts.City; got != "" {
		t.Errorf("default city = %q, want the storefront's (empty)", got)
	}
	for city, want := range map[string]string{
		"de-hamburg":    "de-hamburg",
		"":              "",
		"zz-atlantis":   "", // a city the list does not have (any more)
		"DE-HAMBURG":    "", // IDs are exact
		"gb-london ":    "",
		"../etc/passwd": "",
	} {
		s := Settings{Concerts: Concerts{City: city}}.Normalized()
		if s.Concerts.City != want {
			t.Errorf("city %q normalised to %q, want %q", city, s.Concerts.City, want)
		}
	}
}

func TestConcertsCityPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(s *Settings) { s.Concerts.City = "jp-osaka" }); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"city": "jp-osaka"`) {
		t.Errorf("settings file lacks the city:\n%s", b)
	}
	again, err := OpenStore(path)
	if err != nil || again.Get().Concerts.City != "jp-osaka" {
		t.Errorf("reopened: %+v, %v", again.Get().Concerts, err)
	}
}
