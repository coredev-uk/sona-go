package applemusic

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// SocialProfileAttributes are an Apple Music user profile's attributes
// (social-profiles).
type SocialProfileAttributes struct {
	Name       string   `json:"name"`
	Handle     string   `json:"handle"`
	URL        string   `json:"url"`
	Artwork    *Artwork `json:"artwork"`
	IsVerified bool     `json:"isVerified"`
	IsPrivate  bool     `json:"isPrivate"`
}

// Profile is an Apple Music user profile ("sp.…"): a social-profiles
// resource.
type Profile = Resource[SocialProfileAttributes]

// TrackContribution is one track of a collaborative playlist and who
// added it. AddedBy is nil when Apple does not say (songs added before the
// playlist became collaborative).
type TrackContribution struct {
	ID      string // the track's catalog ID
	Type    string // TypeSongs or "music-videos"
	AddedBy *Profile
}

// PlaylistCollaboration is who works on a user playlist (pl.u-…): its
// owner, whether it is collaborative, its collaborators in Apple's order,
// and who added each of its first 300 tracks (Apple attributes no later
// page: its tracks endpoint ignores the association).
type PlaylistCollaboration struct {
	Collaborative bool
	Owner         Profile // ID "" when Apple names none
	Collaborators []Profile
	Tracks        []TrackContribution // playlist order, at most 300
}

type collaborationBody struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			HasCollaboration bool `json:"hasCollaboration"`
		} `json:"attributes"`
		Relationships struct {
			Curator Relationship[Profile] `json:"curator"`
			Tracks  Relationship[struct {
				ID   string `json:"id"`
				Type string `json:"type"`
				Meta struct {
					Associations struct {
						Collaborators struct {
							Data []Profile `json:"data"`
						} `json:"collaborators"`
					} `json:"associations"`
				} `json:"meta"`
			}] `json:"tracks"`
		} `json:"relationships"`
		Views struct {
			Collaborators View[Profile] `json:"collaborators"`
		} `json:"views"`
	} `json:"data"`
}

// profileFields are the social profile attributes a collaboration needs.
const profileFields = "name,handle,url,artwork"

// PlaylistCollaboration fetches the collaboration of catalog playlist id,
// the web player's social call for a playlist page. It needs no Music
// User Token.
func (c *Client) PlaylistCollaboration(ctx context.Context, sf, id string) (PlaylistCollaboration, error) {
	q := AMPQuery{
		Include: []string{"curator"},
		Views:   []string{"collaborators"},
		Extend:  []string{"hasCollaboration"},
		With:    []string{"lightProfiles"},
		Fields: map[string][]string{
			"playlists":       {"tracks", "curator", "hasCollaboration"},
			"social-profiles": {profileFields},
			// Only the IDs: the page has the tracks already.
			"songs":        {"playParams"},
			"music-videos": {"playParams"},
		},
		Extra: url.Values{
			"relate[playlists]":           {"tracks"},
			"associate[playlists:tracks]": {"collaborators"},
			"limit[tracks]":               {strconv.Itoa(catalogTrackPage)},
			"limit[collaborators]":        {"100"},
		},
	}
	body, err := AMP[collaborationBody](ctx, c, CatalogPath(sf, TypePlaylists, id), q)
	if err != nil {
		return PlaylistCollaboration{}, err
	}
	d, err := first(body.Data, "playlist "+id)
	if err != nil {
		return PlaylistCollaboration{}, err
	}
	out := PlaylistCollaboration{
		Collaborative: d.Attributes.HasCollaboration,
		Collaborators: d.Views.Collaborators.Data,
		Tracks:        make([]TrackContribution, 0, len(d.Relationships.Tracks.Data)),
	}
	if cur := d.Relationships.Curator.Data; len(cur) > 0 && cur[0].Type == TypeSocialProfiles {
		out.Owner = cur[0]
	}
	if out.Collaborators == nil {
		out.Collaborators = []Profile{}
	}
	for _, t := range d.Relationships.Tracks.Data {
		tc := TrackContribution{ID: t.ID, Type: t.Type}
		if by := t.Meta.Associations.Collaborators.Data; len(by) > 0 {
			tc.AddedBy = &by[0]
		}
		out.Tracks = append(out.Tracks, tc)
	}
	return out, nil
}

// OwnSocialProfileID returns the ID ("sp.…") of the signed-in user's
// Apple Music profile, ErrNotFound when they have none.
func (c *Client) OwnSocialProfileID(ctx context.Context) (string, error) {
	var body struct {
		Data []struct {
			Relationships struct {
				Profile Relationship[Resource[struct{}]] `json:"social-profile"`
			} `json:"relationships"`
		} `json:"data"`
	}
	q := url.Values{"include": {"social-profile"}, "fields[social-profiles]": {"handle"}}
	if err := c.amp(ctx, http.MethodGet, "/v1/me/social/profile", q, nil, &body); err != nil {
		return "", err
	}
	me, err := first(body.Data, "own social profile")
	if err != nil {
		return "", err
	}
	p, err := first(me.Relationships.Profile.Data, "own social profile")
	return p.ID, err
}
