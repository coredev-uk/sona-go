package services

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
)

// Collaboration returns who works on playlist id (library or catalog) when
// it is a collaborative playlist: its owner, its collaborators, who added
// each song and what the signed-in user is to it. Only user playlists
// shared to Apple Music (pl.u-…) can be collaborative; for any other
// playlist it answers at once, without the network, with Collaborative
// false. A shared playlist Apple no longer has is not collaborative.
func (s *PlaylistsService) Collaboration(ctx context.Context, id string) (PlaylistCollaboration, error) {
	catalogID, own := id, false
	if applemusic.IsLibraryPlaylistID(id) {
		row, err := s.e.DB.RQ.GetPlaylistByLibraryID(ctx, db.NullString(id))
		if errors.Is(err, sql.ErrNoRows) {
			return noCollaboration(), nil
		}
		if err != nil {
			return PlaylistCollaboration{}, err
		}
		catalogID, own = row.CatalogID.String, row.CanEdit != 0
	}
	if !strings.HasPrefix(catalogID, "pl.u-") {
		return noCollaboration(), nil
	}
	var (
		me string
		g  errgroup.Group
	)
	g.Go(func() error {
		me = s.ownProfile(ctx)
		return nil
	})
	c, err := s.e.API.PlaylistCollaboration(ctx, s.e.Storefront(), catalogID)
	_ = g.Wait()
	if errors.Is(err, applemusic.ErrNotFound) {
		return noCollaboration(), nil
	}
	if err != nil {
		return PlaylistCollaboration{}, s.e.Fail(err)
	}
	return s.m.collaboration(c, me, own), nil
}

// ownProfile is the signed-in user's profile ID, asked once per account
// ("" when signed out or Apple did not say).
func (s *PlaylistsService) ownProfile(ctx context.Context) string {
	if s.e.API.UserToken == nil || s.e.API.UserToken() == "" {
		return ""
	}
	s.mu.Lock()
	me := s.me
	s.mu.Unlock()
	if me != "" {
		return me
	}
	me, err := s.e.API.OwnSocialProfileID(ctx)
	if err != nil {
		// Without it the page shows the user as a listener.
		s.e.Report(err)
		return ""
	}
	s.mu.Lock()
	s.me = me
	s.mu.Unlock()
	return me
}

func noCollaboration() PlaylistCollaboration {
	return PlaylistCollaboration{Collaborators: []SocialProfile{}, Former: []SocialProfile{}, AddedBy: []TrackAddedBy{}}
}

// collaboration maps Apple's collaboration for a viewer with profile ID me
// ("" unknown); own: the user's library copy is editable, which tells the
// owner apart when me is unknown.
func (m mapper) collaboration(c applemusic.PlaylistCollaboration, me string, own bool) PlaylistCollaboration {
	out := noCollaboration()
	if !c.Collaborative {
		return out
	}
	out.Collaborative = true
	out.OwnerID = c.Owner.ID
	in := map[string]bool{}
	if c.Owner.ID != "" {
		out.Collaborators = append(out.Collaborators, m.amProfile(c.Owner))
		in[c.Owner.ID] = true
	}
	for _, p := range c.Collaborators {
		if !in[p.ID] {
			in[p.ID] = true
			out.Collaborators = append(out.Collaborators, m.amProfile(p))
		}
	}
	former := map[string]bool{}
	for _, t := range c.Tracks {
		a := TrackAddedBy{SongID: t.ID}
		if p := t.AddedBy; p != nil {
			a.ProfileID = p.ID
			if !in[p.ID] && !former[p.ID] {
				former[p.ID] = true
				out.Former = append(out.Former, m.amProfile(*p))
			}
		}
		out.AddedBy = append(out.AddedBy, a)
	}
	switch {
	case me != "" && me == c.Owner.ID, me == "" && own:
		out.Viewer = RoleOwner
	case me != "" && in[me]:
		out.Viewer = RoleCollaborator
	default:
		out.Viewer = RoleListener
	}
	return out
}
