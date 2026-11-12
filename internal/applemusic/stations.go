package applemusic

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// Station kinds (attributes.kind) observed on amp-api.
const (
	StationKindStreaming    = "streaming"    // live station or radio episode (format "stream")
	StationKindArtistSeeded = "artistSeeded" // ra.<artistId>
	StationKindSongSeeded   = "songSeeded"   // ra.<songId>, ra.cp-<songId> (AutoPlay)
	StationKindAlbumSeeded  = "albumSeeded"  // ra.<albumId>
	StationKindArtistOnly   = "artistOnly"   // ra.a-<artistId>
	StationKindUser         = "user"         // ra.u-… (the user's station)
	StationKindQuery        = "query"        // ra.q-… (moods)
	StationKindDiscovery    = "discovery"    // ra.q-… (Discovery Station)
	StationKindEditorial    = "editorial"    // genre / editorial track stations
)

// Station play formats (playParams.format).
const (
	StationFormatTracks = "tracks" // algorithmic: POST next-tracks, then webPlayback per song
	StationFormatStream = "stream" // live or episode: GET /v1/play/assets → HLS
)

// StationPlayParams are a station's play parameters.
type StationPlayParams struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`   // "radioStation"
	Format      string `json:"format"` // StationFormatTracks or StationFormatStream
	HasDRM      bool   `json:"hasDrm"`
	MediaType   int    `json:"mediaType"`
	StationHash string `json:"stationHash"`
	// StreamingKind: 1 Apple radio episode, 2 third-party (Shoutcast)
	// stream; 0 for Apple live stations and track stations.
	StreamingKind int `json:"streamingKind"`
}

// StationNotes are a station's editorial notes.
type StationNotes struct {
	Name     string `json:"name"`
	Short    string `json:"short"`
	Standard string `json:"standard"`
	Tagline  string `json:"tagline"`
}

// AirTime is an episode's original broadcast slot (RFC 3339).
type AirTime struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// StationAttributes are the station attributes Sona uses.
type StationAttributes struct {
	Name                 string             `json:"name"`
	Artwork              *Artwork           `json:"artwork"`
	EditorialNotes       *StationNotes      `json:"editorialNotes"`
	IsLive               bool               `json:"isLive"`
	Kind                 string             `json:"kind"`
	MediaKind            string             `json:"mediaKind"`
	PlayParams           *StationPlayParams `json:"playParams"`
	RequiresSubscription bool               `json:"requiresSubscription"`
	StationProviderName  string             `json:"stationProviderName"`
	SupportedDRMs        []string           `json:"supportedDrms"`
	URL                  string             `json:"url"`
	AirTime              *AirTime           `json:"airTime"`
	DurationInMillis     int64              `json:"durationInMillis"`
	// Motion artwork (station page and editorial shelves,
	// extend=editorialVideo; Apple's own live stations).
	EditorialVideo EditorialVideo `json:"editorialVideo,omitempty"`
}

// StationShowAttributes are a radio show's attributes (an apple-curator
// of kind "Show").
type StationShowAttributes struct {
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	ShowHostName   string   `json:"showHostName"`
	Artwork        *Artwork `json:"artwork"`
	EditorialNotes *Notes   `json:"editorialNotes"`
	URL            string   `json:"url"`
}

// StationEventAttributes are one scheduled programme of a live station.
type StationEventAttributes struct {
	Title       string   `json:"title"`
	Description *Notes   `json:"description"`
	StartTime   string   `json:"startTime"`
	EndTime     string   `json:"endTime"`
	Artwork     *Artwork `json:"artwork"`
}

// Station is a catalog station ("stations").
type Station struct {
	Resource[StationAttributes]
	Relationships struct {
		RadioShow Relationship[Resource[StationShowAttributes]]  `json:"radio-show"`
		Events    Relationship[Resource[StationEventAttributes]] `json:"events"`
	} `json:"relationships"`
}

// Format returns the station's play format (StationFormatTracks when
// unknown, as for every algorithmic station).
func (s Station) Format() string {
	if p := s.Attributes.PlayParams; p != nil && p.Format != "" {
		return p.Format
	}
	return StationFormatTracks
}

// StationDetail fetches a station with its radio show and, for live
// stations, its schedule (station-events).
func (c *Client) StationDetail(ctx context.Context, sf, id string) (Station, error) {
	return AMPResource[Station](ctx, c, sf, "stations", id, AMPQuery{
		Include: []string{"radio-show"},
		Extend:  []string{"editorialArtwork", "editorialVideo"},
		Extra:   url.Values{"include[stations]": {"events,radio-show"}, "art[url]": {"f"}},
	})
}

// Stations fetches stations by ID (unknown IDs are omitted). Create Station
// resolves ra.<songId|artistId> through it.
func (c *Client) Stations(ctx context.Context, sf string, ids []string) ([]Station, error) {
	return AMPResources[Station](ctx, c, sf, "stations", ids, AMPQuery{Extra: url.Values{"art[url]": {"f"}}})
}

// LiveStations returns Apple Music's live radio stations.
func (c *Client) LiveStations(ctx context.Context, sf string) ([]Station, error) {
	body, err := AMP[Page[Station]](ctx, c, CatalogPath(sf, "stations"), AMPQuery{
		Extra: url.Values{"filter[featured]": {"apple-music-live-radio"}, "art[url]": {"f"}},
	})
	return body.Data, err
}

// RecentStations returns the stations the user played recently, newest
// first (Apple's own history, shared with the other Apple Music apps),
// with their motion artwork (personal, mood and live stations have it).
func (c *Client) RecentStations(ctx context.Context, limit int) ([]Station, error) {
	body, err := AMP[Page[Station]](ctx, c, "/v1/me/recent/radio-stations", AMPQuery{
		Limit:  limit,
		Extend: []string{"editorialVideo"},
		Extra:  url.Values{"include[stations]": {"radio-show"}, "omit[resource]": {"autos"}},
	})
	return body.Data, err
}

// NextTracks returns the next batch of an algorithmic station's songs
// (POST, no body). Each call advances the station; songs carry their album
// and artist IDs.
func (c *Client) NextTracks(ctx context.Context, stationID string, limit int) ([]Song, error) {
	var body Page[Song]
	q := AMPQuery{Limit: limit, Extra: url.Values{"relate[songs]": {"albums,artists"}}}
	err := c.Do(ctx, Request{
		Method: http.MethodPost,
		URL:    c.ampBase() + "/v1/me/stations/next-tracks/" + url.PathEscape(stationID),
		Query:  q.Values(),
	}, &body)
	if err != nil {
		return nil, err
	}
	return body.Data, nil
}

// StationSeed is one seed item of a continuous (AutoPlay) station.
type StationSeed struct {
	ID        string // catalog song ID, or library song ID with Library
	Library   bool
	Container string // the album/playlist the seed played from; "" if none
}

// ContinuousStation creates the AutoPlay station ("ra.cp-<songId>") that
// continues after seeds.
func (c *Client) ContinuousStation(ctx context.Context, seeds []StationSeed) (Station, error) {
	if len(seeds) == 0 {
		return Station{}, errors.New("applemusic: continuous station needs a seed")
	}
	type container struct {
		ID string `json:"id"`
	}
	type meta struct {
		Container *container `json:"container,omitempty"`
	}
	type seed struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Meta *meta  `json:"meta,omitempty"`
	}
	data := make([]seed, len(seeds))
	for i, s := range seeds {
		data[i] = seed{ID: s.ID, Type: TypeSongs}
		if s.Library {
			data[i].Type = "library-songs"
		}
		if s.Container != "" {
			data[i].Meta = &meta{Container: &container{ID: s.Container}}
		}
	}
	var body struct {
		Results struct {
			Station *Station `json:"station"`
		} `json:"results"`
	}
	err := c.Do(ctx, Request{
		Method: http.MethodPost,
		URL:    c.ampBase() + "/v1/me/stations/continuous",
		Query:  AMPQuery{}.Values(),
		Body:   map[string]any{"data": data},
	}, &body)
	if err != nil {
		return Station{}, err
	}
	if body.Results.Station == nil || body.Results.Station.ID == "" {
		return Station{}, fmt.Errorf("applemusic: continuous station: %w", ErrNotFound)
	}
	return *body.Results.Station, nil
}

// RadioAsset is the stream of a live station or radio episode.
type RadioAsset struct {
	URL          string `json:"url"`                       // HLS master playlist
	KeyServerURL string `json:"keyServerUrl"`              // Widevine licence server ("" for clear streams)
	CertURL      string `json:"widevineKeyCertificateUrl"` // Widevine service certificate
}

// RadioAssets resolves a stream station (live or episode) to its HLS
// asset: GET /v1/play/assets with the station's play parameters.
func (c *Client) RadioAssets(ctx context.Context, p StationPlayParams) (RadioAsset, error) {
	q := url.Values{
		"id":        {p.ID},
		"kind":      {"radioStation"},
		"format":    {"stream"},
		"hasDrm":    {strconv.FormatBool(p.HasDRM)},
		"mediaType": {strconv.Itoa(p.MediaType)},
		"keyFormat": {"web"},
	}
	if p.StationHash != "" {
		q.Set("stationHash", p.StationHash)
	}
	var body struct {
		Results struct {
			Assets []RadioAsset `json:"assets"`
		} `json:"results"`
	}
	if err := c.Do(ctx, Request{URL: c.ampBase() + "/v1/play/assets", Query: q}, &body); err != nil {
		return RadioAsset{}, err
	}
	return first(body.Results.Assets, "assets for "+p.ID)
}

// RadioLicenseRequest is one Widevine licence exchange for a live station
// or radio episode.
type RadioLicenseRequest struct {
	URL       string // RadioAsset.KeyServerURL
	Challenge []byte // CDM licence request or renewal message
	KeyURI    string // the Widevine EXT-X-KEY URI (a data: URI holding the pssh box)
	SlotToken string // License.SlotToken from the previous exchange, if any
}

// radioLicenseEvent is MusicKit's event for live audio stations ("amtv" is
// live video).
const radioLicenseEvent = "beats1"

// AcquireRadioLicense posts a Widevine challenge to the radio key server
// (linear.tv.apple.com/v1/radio/streaming-key-delivery) in MusicKit's
// live-station shape: challenge, uri, key-system, event, stkn. HTTP
// 401/403 wrap ErrUnauthorized; a non-zero status is a *LicenseError.
func (c *Client) AcquireRadioLicense(ctx context.Context, r RadioLicenseRequest) (License, error) {
	if len(r.Challenge) == 0 {
		return License{}, errors.New("radio licence: empty challenge")
	}
	if r.URL == "" {
		return License{}, errors.New("radio licence: no key server")
	}
	body := map[string]any{
		"challenge":  base64.StdEncoding.EncodeToString(r.Challenge),
		"uri":        r.KeyURI,
		"key-system": WidevineKeySystem,
		"event":      radioLicenseEvent,
	}
	return c.postLicense(ctx, r.URL, body, r.SlotToken, "radio licence")
}
