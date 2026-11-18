package services

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"

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
