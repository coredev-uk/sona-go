// Package tunein reads a station's public profile from TuneIn's OPML API
// (opml.radiotime.com, Describe.ashx): its description, genre and location,
// and the song it says is playing when the station feeds TuneIn one.
//
// Sona uses it only for the local broadcasters Apple Music hands over as
// TuneIn stations. Apple's play URL for such a station is TuneIn's own
// playlist (pls.tunein.com), and the TuneIn station ID it plays is in that
// URL's ad parameters (StationID), so no search or guess is involved: a
// station Apple does not serve through TuneIn is never looked up there.
// The parent process makes every request; the playback host and the
// frontend never contact TuneIn.
package tunein

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// DefaultBase is TuneIn's OPML API.
const DefaultBase = "https://opml.radiotime.com"

// ErrNotFound reports a station TuneIn does not know.
var ErrNotFound = errors.New("tunein: station not found")

// maxBody bounds a response (a profile is about 2 KiB).
const maxBody = 256 << 10

var (
	guideID = regexp.MustCompile(`^s[0-9]{1,12}$`)
	// idParam is the ad parameter naming the station TuneIn plays.
	idParam = "aw_0_1st.stationId"
)

// StationID returns the TuneIn station ID ("s…") that Apple's play URL
// for a TuneIn station names, or "" when playURL is not one (the station
// is not TuneIn's, or TuneIn changed the URL's shape).
func StationID(playURL string) string {
	u, err := url.Parse(playURL)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host != "tunein.com" && !strings.HasSuffix(host, ".tunein.com") {
		return ""
	}
	// Apple's URL carries the ad parameters as a query string inside a
	// query parameter, and TuneIn escapes that string once more.
	raw := u.Query().Get("ti_ad_params")
	for range 2 {
		ad, err := url.ParseQuery(raw)
		if err != nil {
			return ""
		}
		if id := ad.Get(idParam); guideID.MatchString(id) {
			return id
		}
		if raw, err = url.QueryUnescape(raw); err != nil {
			return ""
		}
	}
	return ""
}

// Station is what TuneIn says about a station.
type Station struct {
	ID          string
	Name        string
	Slogan      string
	Description string
	Genre       string
	Location    string
	Website     string
	// HasSong: the station feeds TuneIn what it is playing (Artist and
	// Title); most do not.
	HasSong bool
	Artist  string
	Title   string
}

// Client reads TuneIn's OPML API.
type Client struct {
	HTTP      *http.Client // nil: http.DefaultClient
	UserAgent string
	// Base overrides DefaultBase (tests).
	Base string
}

// profile is Describe.ashx's station element.
type profile struct {
	Element       string `json:"element"`
	GuideID       string `json:"guide_id"`
	Name          string `json:"name"`
	Slogan        string `json:"slogan"`
	Description   string `json:"description"`
	Genre         string `json:"genre_name"`
	Location      string `json:"location"`
	URL           string `json:"url"`
	HasSong       bool   `json:"has_song"`
	CurrentSong   string `json:"current_song"`
	CurrentArtist string `json:"current_artist"`
}

type describeResponse struct {
	Head struct {
		Status string `json:"status"`
		Fault  string `json:"fault"`
	} `json:"head"`
	Body []profile `json:"body"`
}

// Describe fetches station id's profile. A station TuneIn does not know
// (a fault in the response, or no station in it) is ErrNotFound.
func (c *Client) Describe(ctx context.Context, id string) (Station, error) {
	if !guideID.MatchString(id) {
		return Station{}, fmt.Errorf("tunein: %q is not a station ID", id)
	}
	base := c.Base
	if base == "" {
		base = DefaultBase
	}
	q := url.Values{"id": {id}, "render": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/Describe.ashx?"+q.Encode(), nil)
	if err != nil {
		return Station{}, err
	}
	req.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Station{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Station{}, fmt.Errorf("tunein: GET Describe.ashx: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Station{}, err
	}
	return parseDescribe(id, b)
}

// parseDescribe reads Describe.ashx's JSON for station id.
func parseDescribe(id string, b []byte) (Station, error) {
	var r describeResponse
	if err := json.Unmarshal(b, &r); err != nil {
		return Station{}, fmt.Errorf("tunein: describe %s: %w", id, err)
	}
	// TuneIn answers HTTP 200 with the failure in the head.
	if r.Head.Status != "200" {
		if r.Head.Status == "400" || r.Head.Status == "404" {
			return Station{}, fmt.Errorf("%w: %s (%s)", ErrNotFound, id, r.Head.Fault)
		}
		return Station{}, fmt.Errorf("tunein: describe %s: status %q %s", id, r.Head.Status, r.Head.Fault)
	}
	for _, p := range r.Body {
		if p.Element != "station" || p.GuideID != id {
			continue
		}
		s := Station{
			ID:          id,
			Name:        strings.TrimSpace(p.Name),
			Slogan:      strings.TrimSpace(p.Slogan),
			Description: strings.TrimSpace(p.Description),
			Genre:       strings.TrimSpace(p.Genre),
			Location:    strings.TrimSpace(p.Location),
			Website:     webURL(p.URL),
			HasSong:     p.HasSong,
		}
		if p.HasSong {
			s.Artist, s.Title = strings.TrimSpace(p.CurrentArtist), strings.TrimSpace(p.CurrentSong)
		}
		return s, nil
	}
	return Station{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

// webURL returns raw when it is an http(s) URL (TuneIn's profiles hold
// whatever the broadcaster typed), else "".
func webURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return ""
	}
	return u.String()
}
