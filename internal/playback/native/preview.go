//go:build linux || (cgo && (darwin || windows))

package native

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
)

// Song previews (Options.Preview) are Apple's 30-second clips of catalog
// songs, what Apple Music on the web plays when signed out (MusicKit JS:
// instance.previewOnly, which sets playbackMode to PREVIEW_ONLY). A song
// carries its clip as previews[].url: plain AAC in an MP4 file on Apple's
// CDN, with no DRM and no credentials. The engine looks the song up with
// the developer token alone, downloads the clip and streams it to a host
// that loads no CDM (cdmhost.Options.Clear), which decodes it like a
// library upload: no webPlayback, service certificate, licence or Music
// User Token, and no CDM needs to be downloaded. The host keeps its
// sandbox and still makes no network requests. Every status carries
// Preview, so nothing reports a preview as a play (the coordinator) and
// the player labels it.

// preparePreview looks up m's preview and starts downloading it.
// Stations and library uploads have none (applemusic.ErrNotPlayable).
func (e *Engine) preparePreview(ctx context.Context, m *media) error {
	if IsStation(m.id) || strings.HasPrefix(m.id, "i.") {
		return fmt.Errorf("native: %s has no preview: %w", m.id, applemusic.ErrNotPlayable)
	}
	var sf string
	if e.sf != nil {
		sf = e.sf()
	}
	if sf == "" {
		return errors.New("native: preview: no storefront to look songs up in")
	}
	t0 := time.Now()
	song, err := e.api.Song(ctx, sf, m.id)
	if err != nil {
		return fmt.Errorf("native: preview %s: %w", m.id, err)
	}
	var url string
	for _, p := range song.Attributes.Previews {
		if p.URL != "" {
			url = p.URL
			break
		}
	}
	if url == "" {
		return fmt.Errorf("native: %s has no preview: %w", m.id, applemusic.ErrNotPlayable)
	}
	tSong := time.Now()
	if err := e.prepareFile(ctx, m, url); err != nil {
		return fmt.Errorf("native: preview %s: %w", m.id, err)
	}
	// The clip's iTunSMPB tag counts the whole song, so the host's demuxer
	// would report the song's length; the movie header has the clip's.
	// prepareFile has the head, with the movie box Apple writes first.
	if d, ok := fmp4.MovieDuration(m.file.downloaded()); ok {
		m.duration, m.clip = d, true
	}
	e.log.Info("prepared preview", "id", m.id, "storefront", sf, "duration", m.duration, "song", tSong.Sub(t0), "file", time.Since(tSong))
	return nil
}
