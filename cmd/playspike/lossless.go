//go:build linux || (darwin && cgo)

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/hls"
)

// enhancedSong is the part of a catalog song the probe reads.
type enhancedSong struct {
	ID         string `json:"id"`
	Attributes struct {
		Name              string   `json:"name"`
		ArtistName        string   `json:"artistName"`
		AudioTraits       []string `json:"audioTraits"`
		ExtendedAssetURLs struct {
			EnhancedHLS string `json:"enhancedHls"`
		} `json:"extendedAssetUrls"`
	} `json:"attributes"`
}

// probeKey is one Widevine key of a variant's media playlist.
type probeKey struct {
	key      *hls.Key
	label    string // the FairPlay skd:// path of the same key ("s1/e1", "c23"), for reading
	segments int
}

// runProbeLossless checks which keys of a song's enhancedHls master
// (lossless ALAC, Dolby Atmos, AAC) Apple's licence server issues to this
// CDM: for every variant it loads the media playlist and requests a
// Widevine licence for each key through applemusic.AcquireLicense, the
// way playback does, and prints one row per variant and key.
func runProbeLossless(log *slog.Logger, envFile, songID, lib string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	api, sf, err := signIn(ctx, envFile)
	if err != nil {
		return err
	}
	song, err := applemusic.AMPResource[enhancedSong](ctx, api, sf.ID, "songs", songID, applemusic.AMPQuery{Extend: []string{"extendedAssetUrls"}})
	if err != nil {
		return fmt.Errorf("catalog song %s: %w", songID, err)
	}
	a := song.Attributes
	step("song", "id", song.ID, "name", fmt.Sprintf("%q", a.Name), "artist", fmt.Sprintf("%q", a.ArtistName), "audioTraits", strings.Join(a.AudioTraits, ","))
	master := a.ExtendedAssetURLs.EnhancedHLS
	if master == "" {
		return fmt.Errorf("song %s has no extendedAssetUrls.enhancedHls", songID)
	}
	body, err := get(ctx, master)
	if err != nil {
		return fmt.Errorf("enhancedHls master: %w", err)
	}
	log.Debug("enhancedHls master", "url", master, "body", string(body))
	variants, err := hls.ParseMaster(body)
	if err != nil {
		return err
	}

	h, err := cdmhost.Start(ctx, cdmhost.Options{CDMPath: lib, Sink: "fakesink", Logger: log})
	if err != nil {
		return fmt.Errorf("cdm host: %w", err)
	}
	defer func() { _ = h.Close() }()
	info := h.Info()
	step("cdm", "version", info.CDMVersion, "path", info.CDMPath)
	cert, err := api.WidevineCert(ctx, "")
	if err != nil {
		return err
	}
	if err := h.SetServerCertificate(ctx, cert); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "VARIANT\tCODECS\tKBPS\tKEY\tKID\tSEGMENTS\tRESULT")
	results := map[string]string{} // by key URI: a key shared by variants is requested once
	seenVariant := map[string]bool{}
	for _, v := range variants {
		name := v.Audio
		if name == "" {
			name = v.URI
		}
		if seenVariant[name+v.Codecs] { // the same rendition on another CDN
			continue
		}
		seenVariant[name+v.Codecs] = true
		u, err := hls.Resolve(master, v.URI)
		if err != nil {
			return err
		}
		keys, err := variantKeys(ctx, log, u)
		if err != nil {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t-\t-\t-\tplaylist: %v\n", name, v.Codecs, v.Bandwidth/1000, err)
			continue
		}
		for _, k := range keys {
			res, ok := results[k.key.URI]
			if !ok {
				res = probeLicence(ctx, api, h, song.ID, k.key)
				results[k.key.URI] = res
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%x\t%d\t%s\n", name, v.Codecs, v.Bandwidth/1000, k.label, k.key.KID, k.segments, res)
		}
	}
	return tw.Flush()
}

// variantKeys lists the Widevine keys of the media playlist at u in order
// of first use, each labelled with the FairPlay key path set alongside it.
func variantKeys(ctx context.Context, log *slog.Logger, u string) ([]probeKey, error) {
	body, err := get(ctx, u)
	if err != nil {
		return nil, err
	}
	pl, err := hls.Parse(body)
	if err != nil {
		return nil, err
	}
	// hls.Parse keeps only the Widevine key per segment; the FairPlay URI
	// of the same EXT-X-KEY group carries Apple's readable key name.
	labels := map[string]string{} // Widevine URI → skd path
	var skd string
	for _, line := range strings.Split(string(body), "\n") {
		attrs, ok := strings.CutPrefix(strings.TrimSpace(line), "#EXT-X-KEY:")
		if !ok {
			continue
		}
		uri := attr(attrs, "URI")
		switch {
		case strings.HasPrefix(uri, "skd://"):
			skd = strings.TrimPrefix(uri, "skd://")
		case attr(attrs, "KEYFORMAT") == hls.WidevineKeyFormat && skd != "":
			labels[uri] = skd
		}
	}
	var out []probeKey
	idx := map[string]int{}
	for _, s := range pl.Segments {
		if s.Key == nil || s.Key.KeyFormat != hls.WidevineKeyFormat {
			continue
		}
		i, ok := idx[s.Key.URI]
		if !ok {
			i = len(out)
			idx[s.Key.URI] = i
			label := labels[s.Key.URI]
			if label == "" {
				label = "?"
			}
			out = append(out, probeKey{key: s.Key, label: label})
		}
		out[i].segments++
	}
	if len(out) == 0 {
		log.Debug("variant without Widevine keys", "url", u, "body", string(body))
		if skd != "" {
			return nil, errors.New("FairPlay only: no Widevine key in the playlist")
		}
		return nil, errors.New("no Widevine key")
	}
	return out, nil
}

// probeLicence requests a licence for k and reports what came of it.
func probeLicence(ctx context.Context, api *applemusic.Client, h *cdmhost.Host, adamID string, k *hls.Key) string {
	sid, challenge, err := h.CreateSession(ctx, k.InitData)
	if err != nil {
		return "challenge failed: " + err.Error()
	}
	defer func() { _ = h.CloseSession(context.WithoutCancel(ctx), sid) }()
	lic, err := api.AcquireLicense(ctx, applemusic.LicenseRequest{Challenge: challenge, KeyURI: k.URI, AdamID: adamID})
	if err != nil {
		var le *applemusic.LicenseError
		if errors.As(err, &le) {
			return fmt.Sprintf("REJECTED status %d %s", le.Status, le.Message)
		}
		return "licence request failed: " + err.Error()
	}
	if err := h.UpdateSession(ctx, sid, lic.Data); err != nil {
		if errors.Is(err, cdmhost.ErrNoUsableKey) {
			return "licence issued, key NOT usable"
		}
		return "licence issued, CDM refused it: " + err.Error()
	}
	return "licence issued, key usable"
}

// attr returns the value of name in an HLS attribute list.
func attr(list, name string) string {
	for list != "" {
		k, rest, ok := strings.Cut(list, "=")
		if !ok {
			return ""
		}
		var v string
		if strings.HasPrefix(rest, `"`) {
			end := strings.Index(rest[1:], `"`)
			if end < 0 {
				return ""
			}
			v, rest = rest[1:end+1], rest[end+2:]
		} else {
			v, rest, _ = strings.Cut(rest, ",")
		}
		if strings.TrimSpace(k) == name {
			return v
		}
		list = strings.TrimPrefix(strings.TrimSpace(rest), ",")
	}
	return ""
}

// get fetches a CDN URL (playlists need no credentials).
func get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", u, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
