package applemusic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
)

// Normalised ("format[resources]=map") responses. With that parameter amp-api
// returns references ({id, type, href, meta}) in data and relationships and
// the full objects once each in resources[type][id]; Resources resolves them.

// Ref is a reference to a resource. Meta carries per-reference data such as
// a recommendation's reason ({"reason":{"stringForDisplay":"New Release"}}).
type Ref struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Href string          `json:"href,omitempty"`
	Meta json.RawMessage `json:"meta,omitempty"`
}

// Reason returns meta.reason.stringForDisplay ("" when absent).
func (r Ref) Reason() string {
	if len(r.Meta) == 0 {
		return ""
	}
	var m struct {
		Reason struct {
			Text string `json:"stringForDisplay"`
		} `json:"reason"`
	}
	if json.Unmarshal(r.Meta, &m) != nil {
		return ""
	}
	return m.Reason.Text
}

// NodeRel is one relationship of a Node: references plus the next page.
type NodeRel struct {
	Href string `json:"href,omitempty"`
	Next string `json:"next,omitempty"`
	Data []Ref  `json:"data"`
}

// Node is one resource of any type with its attributes left undecoded.
// Raw is the whole object, so As can decode it into a typed resource
// (Album, Playlist, MusicVideo, …) whose relationships then hold the
// references.
type Node struct {
	ID            string             `json:"id"`
	Type          string             `json:"type"`
	Href          string             `json:"href,omitempty"`
	Attributes    json.RawMessage    `json:"attributes,omitempty"`
	Relationships map[string]NodeRel `json:"relationships,omitempty"`
	Views         map[string]NodeRel `json:"views,omitempty"`
	Meta          json.RawMessage    `json:"meta,omitempty"`
	Raw           json.RawMessage    `json:"-"`
}

// UnmarshalJSON keeps the raw object next to the decoded envelope.
func (n *Node) UnmarshalJSON(b []byte) error {
	type node Node
	var v node
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*n = Node(v)
	n.Raw = bytes.Clone(b)
	return nil
}

// MarshalJSON writes the raw object back (a Node round-trips unchanged).
func (n Node) MarshalJSON() ([]byte, error) {
	if len(n.Raw) > 0 {
		return n.Raw, nil
	}
	type node Node
	return json.Marshal(node(n))
}

// Rel returns relationship name's references (nil when absent).
func (n Node) Rel(name string) []Ref { return n.Relationships[name].Data }

// Attrs decodes the node's attributes into a.
func (n Node) Attrs(a any) error {
	if len(n.Attributes) == 0 {
		return nil
	}
	return json.Unmarshal(n.Attributes, a)
}

// As decodes n into a typed resource such as Album or Resource[A].
func As[T any](n Node) (T, error) {
	var v T
	if len(n.Raw) == 0 {
		return v, fmt.Errorf("applemusic: empty %s %s", n.Type, n.ID)
	}
	err := json.Unmarshal(n.Raw, &v)
	return v, err
}

// Resources is a normalised response's resources map: type → id → object.
type Resources map[string]map[string]Node

// Get returns the object ref points to.
func (r Resources) Get(ref Ref) (Node, bool) {
	n, ok := r[ref.Type][ref.ID]
	return n, ok
}

// Resolve returns the objects refs point to, in order, skipping references
// the response did not include.
func (r Resources) Resolve(refs []Ref) []Node {
	out := make([]Node, 0, len(refs))
	for _, ref := range refs {
		if n, ok := r.Get(ref); ok {
			out = append(out, n)
		}
	}
	return out
}

// Merge adds other's objects to r, replacing objects with the same type
// and ID.
func (r Resources) Merge(other Resources) {
	for typ, byID := range other {
		if r[typ] == nil {
			r[typ] = make(map[string]Node, len(byID))
		}
		for id, n := range byID {
			r[typ][id] = n
		}
	}
}

// MapResponse is the envelope of a normalised response.
type MapResponse struct {
	Data      []Ref           `json:"data"`
	Next      string          `json:"next,omitempty"`
	Resources Resources       `json:"resources"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

// Primary returns the objects data refers to.
func (m MapResponse) Primary() []Node { return m.Resources.Resolve(m.Data) }

// mapValues adds format[resources]=map to q's extra parameters.
func mapValues(q AMPQuery) AMPQuery { return q.with("format[resources]", "map") }

// AMPMap fetches a normalised (format[resources]=map) response from amp-api.
func AMPMap(ctx context.Context, c *Client, path string, q AMPQuery) (MapResponse, error) {
	return AMP[MapResponse](ctx, c, path, mapValues(q))
}

// Editorial resource types.
const (
	TypeGroupings         = "groupings"
	TypeEditorialElements = "editorial-elements"
	TypeRooms             = "rooms"
	TypeMultirooms        = "multirooms"
	TypeAppleCurators     = "apple-curators"
	TypeCurators          = "curators"
	TypeEditorialItems    = "editorial-items"
	TypeStations          = "stations"
	TypeUploadedVideos    = "uploaded-videos"
	TypeMusicMovies       = "music-movies"
	TypeSocialProfiles    = "social-profiles"
)

// Editorial element kinds (attributes.editorialElementKind) that drive
// presentation. Unlisted kinds are containers or plain shelves.
const (
	ElementPage      = "382" // a tab's root: children are the page's sections
	ElementHero      = "316" // hero carousel: children are 317/320 cards
	ElementHeroCard  = "317" // hero card showing its one content item's art
	ElementHeroArt   = "320" // hero card with its own artwork and tagline
	ElementShelf     = "326" // shelf of contents
	ElementTracks    = "327" // track shelf (songs)
	ElementShelfAlt  = "345" // shelf (multirooms, rooms)
	ElementBrick     = "385" // brick row: children are 386/394 tiles
	ElementBrickItem = "386" // brick tile of its one content item
	ElementBrickArt  = "394" // brick tile with its own artwork and link
	ElementLinks     = "391" // link list
	ElementLinksAlt  = "322" // link list (genres)
	ElementText      = "404" // description paragraph
	ElementLive      = "488" // live radio stations with schedules
)

// Link is an editorial link: a label and an Apple URL (music.apple.com,
// itunes.apple.com legacy store URLs) or an external page.
type Link struct {
	Label  string `json:"label"`
	URL    string `json:"url"`
	Target string `json:"target"` // "internal", "external" or ""
}

// EditorialElementAttributes are an editorial element's attributes.
// Sections in groupings carry Name, elements of multirooms carry Title.
type EditorialElementAttributes struct {
	Kind             string             `json:"editorialElementKind"`
	Name             string             `json:"name"`
	Title            string             `json:"title"`
	Description      string             `json:"description"`
	DisplayStyle     string             `json:"displayStyle"`
	DesignBadge      string             `json:"designBadge"`
	DesignTag        string             `json:"designTag"`
	ResourceTypes    []string           `json:"resourceTypes"`
	Artwork          *Artwork           `json:"artwork"`
	EditorialArtwork map[string]Artwork `json:"editorialArtwork"`
	Link             *Link              `json:"link"`
	Links            []Link             `json:"links"`
}

// Heading returns Name, falling back to Title.
func (a EditorialElementAttributes) Heading() string {
	if a.Name != "" {
		return a.Name
	}
	return a.Title
}

// GroupingAttributes are a grouping's attributes.
type GroupingAttributes struct {
	Name       string   `json:"name"`
	GenreNames []string `json:"genreNames"`
}

// RoomAttributes are a room's ("See All" grid) attributes.
type RoomAttributes struct {
	Title         string   `json:"title"`
	Kind          string   `json:"editorialElementKind"`
	ResourceTypes []string `json:"resourceTypes"`
	Sorts         []string `json:"sorts"`
	DefaultSort   string   `json:"defaultSort"`
}

// Uber is a multiroom's or curator's banner.
type Uber struct {
	MasterArt       *Artwork `json:"masterArt"`
	BackgroundColor string   `json:"backgroundColor"`
	Description     *string  `json:"description"`
	Name            *string  `json:"name"`
}

// MultiroomAttributes are a multiroom's (campaign page) attributes.
type MultiroomAttributes struct {
	Title string `json:"title"`
	Uber  *Uber  `json:"uber"`
}

// CuratorNotes are an apple-curator's editorial notes.
type CuratorNotes struct {
	Name     string `json:"name"`
	Short    string `json:"short"`
	Standard string `json:"standard"`
	Tagline  string `json:"tagline"`
}

// CuratorAttributes are an apple-curator's or curator's attributes. Kind
// is "Genre", "Show" or "Curator" for apple-curators, "" for curators.
type CuratorAttributes struct {
	Name             string             `json:"name"`
	ShortName        string             `json:"shortName"`
	Kind             string             `json:"kind"`
	ShowHostName     string             `json:"showHostName"`
	Artwork          *Artwork           `json:"artwork"`
	EditorialArtwork map[string]Artwork `json:"editorialArtwork"`
	EditorialNotes   *CuratorNotes      `json:"editorialNotes"`
	PlaylistCount    int                `json:"playlistCount"`
	URL              string             `json:"url"`
}

// EditorialItemAttributes are an editorial item's attributes (a card
// pointing at a link, used by Home and search landing).
type EditorialItemAttributes struct {
	Link               *Link              `json:"link"`
	URL                string             `json:"url"`
	EditorialArtwork   map[string]Artwork `json:"editorialArtwork"`
	PlainEditorialNote *struct {
		Name    string `json:"name"`
		Short   string `json:"short"`
		Tagline string `json:"tagline"`
	} `json:"plainEditorialNotes"`
}

// Editorial request vocabulary, as the web player sends it.
var (
	editorialInclude = url.Values{
		"include[albums]":        {"artists"},
		"include[songs]":         {"artists"},
		"include[music-videos]":  {"artists"},
		"include[stations]":      {"radio-show"},
		"relate[songs]":          {"albums"},
		"extend[stations]":       {"airTime,supportsAirTimeUpdates"},
		"omit[resource:artists]": {"autos"},
		"art[url]":               {"c,f"},
		"fields[artists]":        {"name,url,artwork"},
		"extend[station-events]": {"editorialVideo"},
		"meta[stations]":         {"inflectionPoints"},
		"include[playlists]":     {"curator"},
		"fields[curators]":       {"name,url"},
		"fields[apple-curators]": {"name,url,artwork,kind,shortName,editorialArtwork,showHostName"},
	}
	// editorialVideo goes beyond the web player's request: it gives the
	// albums, playlists and stations of a page their motion artwork
	// (played on a tile's hover); services keep only the square flavour.
	editorialExtend = []string{"artistUrl", "editorialArtwork", "editorialVideo", "plainEditorialNotes"}
)

func editorialQuery(extra url.Values) AMPQuery {
	v := maps.Clone(editorialInclude)
	maps.Copy(v, extra)
	return AMPQuery{Extend: editorialExtend, Extra: v}
}

// Grouping fetches the subscriber tab of a named editorial grouping:
// "music" (New) or "radio" (Radio). data[0] is the groupings object; its
// tabs relationship points at the page's root editorial element.
func Grouping(ctx context.Context, c *Client, sf, name string) (MapResponse, error) {
	return AMPMap(ctx, c, scopedPath("editorial", sf, "groupings"), editorialQuery(url.Values{"name": {name}, "tabs": {"subscriber"}}))
}

// GroupingByID fetches grouping id (e.g. the Music Videos hub).
func GroupingByID(ctx context.Context, c *Client, sf, id string) (MapResponse, error) {
	return AMPMap(ctx, c, scopedPath("editorial", sf, "groupings", id), editorialQuery(url.Values{"tabs": {"subscriber"}, "include[groupings]": {"curator"}}))
}

// Room fetches room id (an editorial section's "See all" grid) with its
// first page of contents (100); Next pages come from RoomContents.
func Room(ctx context.Context, c *Client, sf, id, sort string) (MapResponse, error) {
	extra := url.Values{}
	if sort != "" {
		extra.Set("sort", sort)
	}
	return AMPMap(ctx, c, scopedPath("editorial", sf, "rooms", id), editorialQuery(extra))
}

// RoomContents fetches the page of room id's contents at offset; data
// holds the items' references.
func RoomContents(ctx context.Context, c *Client, sf, id, sort string, offset int) (MapResponse, error) {
	q := editorialQuery(nil)
	if sort != "" {
		q.Extra.Set("sort", sort)
	}
	q.Offset = offset
	return AMPMap(ctx, c, scopedPath("editorial", sf, "rooms", id, "contents"), q)
}

// Multiroom fetches multiroom id (a campaign page): its children are rooms
// and editorial elements.
func Multiroom(ctx context.Context, c *Client, sf, id string) (MapResponse, error) {
	q := editorialQuery(nil)
	q.Extend = append(append([]string{}, editorialExtend...), "lockupStyle", "uber")
	return AMPMap(ctx, c, scopedPath("editorial", sf, "multirooms", id), q)
}

// CuratorPage fetches apple-curator or curator id (the type is unknown from a
// link, so both are asked for) with its grouping and playlists.
func CuratorPage(ctx context.Context, c *Client, sf, id string) (MapResponse, error) {
	q := editorialQuery(url.Values{
		"ids[apple-curators]":    {id},
		"ids[curators]":          {id},
		"extend[apple-curators]": {"playlistCount"},
		"extend[curators]":       {"playlistCount"},
	})
	q.Include = []string{"grouping", "playlists"}
	return AMPMap(ctx, c, CatalogPath(sf), q)
}
