package playlist

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

func TestAddedByRows(t *testing.T) {
	person := func(id string) services.SocialProfile { return services.SocialProfile{ID: id, Name: id} }
	track := func(id string, catalogID ...string) services.Song {
		s := services.Song{ID: id, CatalogID: id}
		if len(catalogID) > 0 {
			s.CatalogID = catalogID[0]
		}
		return s
	}
	collab := services.PlaylistCollaboration{
		Collaborative: true,
		OwnerID:       "robin",
		Collaborators: []services.SocialProfile{person("robin"), person("ada")},
		Former:        []services.SocialProfile{person("jules")},
		AddedBy: []services.TrackAddedBy{
			{SongID: "1", ProfileID: "ada"},
			{SongID: "2", ProfileID: ""},
			{SongID: "3", ProfileID: "jules"},
			{SongID: "1", ProfileID: "robin"},
		},
	}
	names := func(rows []*services.SocialProfile) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = "-"
			if r != nil {
				out[i] = r.ID
			}
		}
		return out
	}
	for _, c := range []struct {
		name   string
		tracks []services.Song
		want   []string
	}{
		{
			"playlist order, a repeated song to each adder",
			[]services.Song{track("1"), track("2"), track("3"), track("1")},
			[]string{"ada", "-", "jules", "robin"},
		},
		{
			"library songs match by catalogue ID, whatever their order",
			[]services.Song{track("i.c", "3"), track("i.a", "1"), track("i.b", "1")},
			[]string{"jules", "ada", "robin"},
		},
		{
			"rows Apple did not attribute stay empty",
			[]services.Song{track("1"), track("1"), track("1"), track("9"), track("i.up", "")},
			[]string{"ada", "robin", "-", "-", "-"},
		},
		{"no tracks", nil, []string{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := names(addedByRows(c.tracks, collab)); !slices.Equal(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
