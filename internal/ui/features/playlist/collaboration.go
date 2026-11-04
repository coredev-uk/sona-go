package playlist

import "github.com/coredev-uk/sona-go/internal/services"

// addedByRows says who added each of tracks, by row: Apple names the adder
// of a collaborative playlist's songs by catalogue ID in playlist order, so
// each row takes the next unclaimed attribution of its song (a song added
// twice keeps both adders). Rows Apple says nothing about are nil.
func addedByRows(tracks []services.Song, c services.PlaylistCollaboration) []*services.SocialProfile {
	people := map[string]*services.SocialProfile{}
	for _, list := range [][]services.SocialProfile{c.Collaborators, c.Former} {
		for i := range list {
			people[list[i].ID] = &list[i]
		}
	}
	queue := map[string][]string{}
	for _, a := range c.AddedBy {
		queue[a.SongID] = append(queue[a.SongID], a.ProfileID)
	}
	out := make([]*services.SocialProfile, len(tracks))
	for i, t := range tracks {
		id := t.CatalogID
		if id == "" {
			id = t.ID
		}
		if q := queue[id]; len(q) > 0 {
			out[i] = people[q[0]]
			queue[id] = q[1:]
		}
	}
	return out
}
