package services

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/logging"
)

func init() {
	features = append(features, feature{"inspect", svc(newInspectService)})
}

// InspectService is the media item inspector (right-click → Inspect): what Go
// mapped for an item, what Apple sent and what an add-on is told of it, from
// Go's own caches. A development build offers Inspect always, a release build
// while Settings → Diagnostics → Developer tools is on (docs/ui-architecture.md,
// "Developer actions"); either way it only reads the user's own item data.
type InspectService struct{ env *Env }

func newInspectService(e *Env) *InspectService { return &InspectService{env: e} }

// InspectQuery names the item to inspect. Kind is the UI's item kind ("song",
// "album", "playlist", "artist", "station", "music-video"); CatalogID and
// LibraryID are the item's IDs in each source ("" when it has none); Item is
// the mapped DTO as the UI holds it.
type InspectQuery struct {
	Kind      string `json:"kind"`
	CatalogID string `json:"catalogId"`
	LibraryID string `json:"libraryId"`
	Item      any    `json:"item"`
}

// InspectReport is what Inspect found. Every text has passed through the
// logging redaction, and none holds a request header: the cache stores
// response bodies only.
type InspectReport struct {
	// Mapped is the item as the UI holds it, pretty-printed.
	Mapped string `json:"mapped"`
	// RawKey is the catalog cache key Raw came from ("" when none).
	RawKey string `json:"rawKey"`
	// Raw is Apple's resource as the catalog cache holds it, pretty-printed;
	// "" when the cache has none (a library-only item, a station, an expired
	// entry that was never fetched).
	Raw string `json:"raw"`
	// Row is the item's library row in SQLite, pretty-printed; "" for a
	// catalog item.
	Row string `json:"row"`
	// Addon is what an add-on is sent for the item.
	Addon InspectAddon `json:"addon"`
	// URL is the item's music.apple.com page ("" when Sona has none: the
	// item's own url, else the cached resource's attributes.url).
	URL string `json:"url"`
	// GlobalID is a playlist's global ID, from the cached resource's play
	// parameters, when it differs from the catalogue ID ("" otherwise).
	GlobalID string `json:"globalId"`
	// Art is the item's artwork template and what it makes at the sizes Sona
	// shows; zero for an item with no artwork or one Go has no template for.
	Art InspectArt `json:"art"`
}

// InspectArt is an item's artwork as Apple serves it: the template (with
// {w}, {h}, {c} and {f} to fill) and its public URL at each size Sona shows.
// Only https URLs on Apple's artwork CDN are here, never a Go-served path.
type InspectArt struct {
	Template string           `json:"template"`
	Sizes    []InspectArtSize `json:"sizes"`
}

// InspectArtSize is one size of the artwork: Name is the field of the item's
// Art it belongs to ("small", "medium", "large"), Local the Go-served URL
// the UI shows, and URL the template at that size on Apple's CDN.
type InspectArtSize struct {
	Name   string `json:"name"`
	Local  string `json:"local"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// InspectAddon is the add-on protocol's view of an item, as the code that
// speaks the protocol builds it: the same functions, not copies. Each text is
// pretty-printed JSON, redacted, "" where an add-on is never sent one.
type InspectAddon struct {
	// Target is the target of an invoke of an action chosen on the item
	// ({"kind","id","name"}); "" for a station or a music video, which no
	// add-on action is offered on.
	Target string `json:"target"`
	// Track is the track of a now-playing event for the item, as the add-on
	// session writes it; "" unless the item is a song. An add-on is never sent
	// a private listen or a preview: this is the user's own inspection, so it
	// shows the item regardless.
	Track string `json:"track"`
}

// Inspect reports on one item from the caches and the database; it never
// calls Apple.
func (s *InspectService) Inspect(ctx context.Context, q InspectQuery) (InspectReport, error) {
	var out InspectReport
	var err error
	if out.Mapped, err = redactedJSON(q.Item); err != nil {
		return out, err
	}
	if out.Addon, err = s.addon(q.Item); err != nil {
		return out, err
	}
	out.Art = s.art(q.Item)
	out.URL = appleLink(itemLink(q.Item))
	if q.CatalogID != "" && s.env.Catalog != nil {
		key, body, ok, err := s.env.Catalog.Raw(ctx, q.Kind, q.CatalogID)
		if err != nil && !errors.Is(err, context.Canceled) {
			return out, err
		}
		if ok {
			out.RawKey = logging.Redact(key)
			out.Raw, err = redactedJSON(json.RawMessage(body))
			if err != nil {
				return out, err
			}
			attrs := resourceAttributes(body, q.CatalogID)
			if out.URL == "" {
				out.URL = appleLink(attrs.URL)
			}
			if g := attrs.PlayParams.GlobalID; g != q.CatalogID {
				out.GlobalID = logging.Redact(g)
			}
		}
	}
	if q.LibraryID != "" {
		row, err := s.libraryRow(ctx, q.Kind, q.LibraryID)
		if err != nil {
			return out, err
		}
		if row != nil {
			if out.Row, err = redactedJSON(row); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

// itemLink is the music.apple.com page the item's own DTO names.
func itemLink(item any) string {
	switch t := item.(type) {
	case Station:
		return t.URL
	case MusicVideo:
		return t.URL
	}
	return ""
}

// appleLink is u when it is a page on music.apple.com, the one place Sona
// opens a link to for an item (OpenInAppleMusic's rule), and "" otherwise.
func appleLink(u string) string {
	if !strings.HasPrefix(u, "https://music.apple.com/") {
		return ""
	}
	return logging.Redact(u)
}

// resourceAttrs are the attributes of a cached Apple resource the inspector
// reads: its page and its play parameters.
type resourceAttrs struct {
	URL        string `json:"url"`
	PlayParams struct {
		GlobalID string `json:"globalId"`
	} `json:"playParams"`
}

// resourceAttributes finds the resource id in a cached response body and
// returns its attributes (zero when it has none): the response is
// {"data":[{…}]} or holds the resource under a page's own keys, so it looks
// for the first object with that id and attributes.
func resourceAttributes(body []byte, id string) resourceAttrs {
	var v any
	if json.Unmarshal(body, &v) != nil {
		return resourceAttrs{}
	}
	var find func(v any, depth int) json.RawMessage
	find = func(v any, depth int) json.RawMessage {
		if depth > 8 {
			return nil
		}
		switch t := v.(type) {
		case map[string]any:
			if t["id"] == id {
				if a, ok := t["attributes"]; ok {
					b, _ := json.Marshal(a)
					return b
				}
			}
			for _, k := range slices.Sorted(maps.Keys(t)) {
				if b := find(t[k], depth+1); b != nil {
					return b
				}
			}
		case []any:
			for _, e := range t {
				if b := find(e, depth+1); b != nil {
					return b
				}
			}
		}
		return nil
	}
	var out resourceAttrs
	if b := find(v, 0); b != nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

// art is the item's artwork template, found again through the artwork cache,
// and its public URL at each size the DTO carries; zero without a template.
func (s *InspectService) art(item any) InspectArt {
	var a Art
	switch t := item.(type) {
	case Song:
		a = t.Art
	case Album:
		a = t.Art
	case Playlist:
		a = t.Art
	case Artist:
		a = t.Art
	case Station:
		a = t.Art
	case MusicVideo:
		a = t.Art
	}
	if s.env.Art == nil {
		return InspectArt{}
	}
	tmpl, ok := s.env.Art.Template(a.Large)
	if !ok {
		return InspectArt{}
	}
	out := InspectArt{Template: logging.Redact(tmpl)}
	for _, sz := range []struct{ name, local string }{{"small", a.Small}, {"medium", a.Medium}, {"large", a.Large}} {
		w, h := artDims(sz.local)
		if w == 0 {
			continue
		}
		out.Sizes = append(out.Sizes, InspectArtSize{Name: sz.name, Local: sz.local, URL: logging.Redact(publicArtSized(tmpl, w, h)), Width: w, Height: h})
	}
	return out
}

// artSizePart is the "<w>x<h>.jpg" a Go-served art URL ends in.
var artSizePart = regexp.MustCompile(`/(\d+)x(\d+)\.jpg(?:$|\?)`)

// artDims is the size in a Go-served art URL (0, 0 when it names none).
func artDims(local string) (w, h int) {
	m := artSizePart.FindStringSubmatch(local)
	if m == nil {
		return 0, 0
	}
	w, _ = strconv.Atoi(m[1])
	h, _ = strconv.Atoi(m[2])
	return w, h
}

// addon is what an add-on is sent for item: the invoke target the item menu
// builds (AddonTargetOf, then the request's own encoding) and, for a song, the
// track of the now-playing event (the command line's track mapper, then the
// session's event encoding).
func (s *InspectService) addon(item any) (InspectAddon, error) {
	var out InspectAddon
	if t, ok := AddonTargetOf(item); ok {
		var err error
		if out.Target, err = redactedJSON(invokeTarget(t)); err != nil {
			return out, err
		}
	}
	song, ok := item.(Song)
	if !ok {
		return out, nil
	}
	var template string
	if s.env.Art != nil {
		template, _ = s.env.Art.Template(song.Art.Large)
	}
	id := cmp.Or(song.CatalogID, song.LibraryID, song.ID)
	t := track(itemOf(id, song.Name, song.Artist, song.Album, song.DurationMs, template, song.Explicit))
	data, err := cli.AddonEventData(cli.NowPlaying{Track: &t})
	if err != nil {
		return out, err
	}
	var event struct {
		Track json.RawMessage `json:"track"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return out, err
	}
	out.Track, err = redactedJSON(event.Track)
	return out, err
}

// libraryRow is the database row of a library item (nil: none of that kind
// or ID).
func (s *InspectService) libraryRow(ctx context.Context, kind, id string) (any, error) {
	libID := sql.NullString{String: id, Valid: true}
	var row any
	var err error
	switch kind {
	case "song":
		row, err = s.env.DB.RQ.GetTrackByLibraryID(ctx, libID)
	case "album":
		row, err = s.env.DB.RQ.GetAlbumByLibraryID(ctx, libID)
	case "playlist":
		row, err = s.env.DB.RQ.GetPlaylistByLibraryID(ctx, libID)
	default:
		return nil, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return row, err
}

// redactedJSON is v pretty-printed with its secrets scrubbed (logging.Redact).
func redactedJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		return "", err
	}
	return logging.Redact(buf.String()), nil
}
