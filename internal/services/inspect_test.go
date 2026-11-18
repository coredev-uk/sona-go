//go:build unix

package services

import (
	"encoding/json"
	"testing"
)

// The add-on section of Inspect is what an add-on is sent: the target is the
// one a real invoke carries to a running add-on (it echoes what it was sent),
// and the track is the command line's mapping of the song as the session
// writes the now-playing event.
func TestInspectShowsTheTargetAnAddonReceivesAndTheNowPlayingTrack(t *testing.T) {
	a := newAddonEnv(t)
	a.install(t, "echo", manifestWith("echo", `{"actions":[{"id":"send","label":"Send","kinds":["song","album","playlist"]}]}`), hello("echo")+`
while read -r line; do
  case $line in
    *'"method":"invoke"'*)
      id=${line#'{"id":'}; id=${id%%,*}
      t=${line#*'"target":'}; t=${t%%,'"deadlineMs"'*}
      t=${t//\"/\\\"}
      printf '{"id":%s,"result":{"toast":"%s"}}\n' "$id" "$t" ;;
  esac
done
`)
	if err := a.svc.Enable("echo", nil); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	a.waitState(t, "echo", AddonRunning)
	insp := newInspectService(a.e)

	song := Song{ID: "100", Source: SourceCatalog, CatalogID: "100", Name: "Come \u202etogether", Artist: "The Beatles", Album: "Abbey Road", DurationMs: 259000, Explicit: true}
	for name, item := range map[string]any{
		"song":     song,
		"album":    Album{ID: "i.5", Source: SourceLibrary, LibraryID: "i.5", Name: "Abbey Road"},
		"playlist": Playlist{ID: "pl.x", CatalogID: "pl.x", Name: "Mix"},
	} {
		target, ok := AddonTargetOf(item)
		if !ok {
			t.Fatalf("%s: no target", name)
		}
		got, err := a.svc.Invoke(t.Context(), "echo", "addon.echo.send", &target)
		if err != nil {
			t.Fatal(err)
		}
		rep, err := insp.Inspect(t.Context(), InspectQuery{Kind: name, Item: item})
		if err != nil {
			t.Fatal(err)
		}
		if !sameJSON(t, rep.Addon.Target, got.Toast) {
			t.Errorf("%s: Inspect shows target %s, the add-on was sent %s", name, rep.Addon.Target, got.Toast)
		}
	}

	rep, err := insp.Inspect(t.Context(), InspectQuery{Kind: "song", Item: song})
	if err != nil {
		t.Fatal(err)
	}
	// The name is cleaned as the invoke cleans it, and the track is the
	// mapping `sona status` gives, with the explicit flag and seconds.
	if want := `{"id":"100","title":"Come \u202etogether","artist":"The Beatles","album":"Abbey Road","duration":259,"explicit":true,"artwork":""}`; !sameJSON(t, rep.Addon.Track, want) {
		t.Errorf("track = %s, want %s", rep.Addon.Track, want)
	}

	for _, kind := range []struct {
		name string
		item any
	}{{"station", Station{ID: "ra.1", Name: "Radio"}}, {"music-video", MusicVideo{ID: "1", Name: "Clip"}}} {
		rep, err := insp.Inspect(t.Context(), InspectQuery{Kind: kind.name, Item: kind.item})
		if err != nil {
			t.Fatal(err)
		}
		if rep.Addon.Target != "" || rep.Addon.Track != "" {
			t.Errorf("%s: add-on section = %+v, want none", kind.name, rep.Addon)
		}
	}
}

func sameJSON(t *testing.T, a, b string) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal([]byte(a), &x); err != nil {
		t.Fatalf("%q: %v", a, err)
	}
	if err := json.Unmarshal([]byte(b), &y); err != nil {
		t.Fatalf("%q: %v", b, err)
	}
	p, _ := json.Marshal(x)
	q, _ := json.Marshal(y)
	return string(p) == string(q)
}
