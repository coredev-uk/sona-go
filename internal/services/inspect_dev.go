//go:build dev

package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/coredev-uk/sona-go/internal/logging"
)

func init() {
	features = append(features, feature{"inspect", svc(newInspectService)})
}

// InspectService is the development build's media item inspector (right-click
// → Inspect): what Go mapped for an item and what Apple sent, from Go's own
// caches. It exists only under the dev build tag; release builds hold neither
// it nor the menu action (docs/ui-architecture.md, "Developer actions").
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
}

// Inspect reports on one item from the caches and the database; it never
// calls Apple.
func (s *InspectService) Inspect(ctx context.Context, q InspectQuery) (InspectReport, error) {
	var out InspectReport
	var err error
	if out.Mapped, err = redactedJSON(q.Item); err != nil {
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
