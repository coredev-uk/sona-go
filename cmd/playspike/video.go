//go:build linux || (darwin && cgo)

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/hls"
	"github.com/coredev-uk/sona-go/internal/playback/native"
	"github.com/coredev-uk/sona-go/internal/playback/video"
)

// runProbeVideo prints, for every rendition of a music video's master
// playlist, its format and what Apple's licence server and this CDM make
// of its Widevine key: issued and usable, issued but restricted (key
// status), or refused. Nothing plays.
func runProbeVideo(log *slog.Logger, envFile, id, lib string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	api, _, err := signIn(ctx, envFile)
	if err != nil {
		return err
	}
	info, master, err := videoMaster(ctx, api, id)
	if err != nil {
		return err
	}
	variants, err := hls.ParseMaster(master)
	if err != nil {
		return err
	}
	renditions, err := hls.ParseRenditions(master)
	if err != nil {
		return err
	}
	h, err := cdmhost.Start(ctx, cdmhost.Options{CDMPath: lib, Sink: "fakesink", Logger: log})
	if err != nil {
		return fmt.Errorf("cdm host: %w", err)
	}
	defer func() { _ = h.Close() }()
	if err := setCert(ctx, api, h, info.CertURL); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "RENDITION\tRESOLUTION\tCODECS\tKBPS\tHDCP\tWIDEVINE\tKID\tRESULT")
	results := map[string]string{} // by key URI
	row := func(name, res, codecs string, kbps int, hdcp, cpc, uri string) {
		u, err := hls.Resolve(info.MasterURL, uri)
		if err != nil {
			return
		}
		keys, err := variantKeys(ctx, log, u)
		if err != nil {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t-\t%v\n", name, res, codecs, kbps, hdcp, cpc, err)
			return
		}
		for _, k := range keys {
			r, ok := results[k.key.URI]
			if !ok {
				r = probeLicence(ctx, api, h, info.SongID, k.key)
				results[k.key.URI] = r
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%x\t%s\n", name, res, codecs, kbps, hdcp, cpc, k.key.KID, r)
		}
	}
	for _, r := range renditions {
		if r.Type == "AUDIO" && r.URI != "" {
			row(r.GroupID, "-", "-", 0, "-", "-", r.URI)
		}
	}
	for _, v := range variants {
		if v.Width == 0 {
			continue
		}
		cpc := "software"
		if !v.SoftwareWidevine() {
			cpc = "hardware"
		}
		row("video", fmt.Sprintf("%dx%d", v.Width, v.Height), v.Codecs, v.Bandwidth/1000, v.HDCPLevel, cpc, v.URI)
	}
	return tw.Flush()
}

// videoMaster asks webPlayback for music video id and fetches its master
// playlist.
func videoMaster(ctx context.Context, api *applemusic.Client, id string) (applemusic.VideoPlaybackInfo, []byte, error) {
	info, err := api.VideoPlayback(ctx, id)
	if err != nil {
		return info, nil, err
	}
	master, err := get(ctx, info.MasterURL)
	if err != nil {
		// The master's URL names the account (dsid): never print it.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		} else if i := strings.LastIndex(err.Error(), ": "); i >= 0 {
			err = errors.New(err.Error()[i+2:])
		}
		return info, nil, fmt.Errorf("master playlist: %w", err)
	}
	return info, master, nil
}

func setCert(ctx context.Context, api *applemusic.Client, h *cdmhost.Host, url string) error {
	cert, err := api.WidevineCert(ctx, url)
	if err != nil {
		return err
	}
	return h.SetServerCertificate(ctx, cert)
}

// runVideo plays music video id through the engine as the app does: its
// pictures go to a video.Surface, and this prints what arrives there
// (pictures per second, size, how much detail they hold, and how far each
// one is from the audio heard when it arrives). It plays from the start,
// seeks to seekTo, plays for listen, then plays the last seconds through
// to the end. Pictures stay in memory; nothing is reported as a play.
func runVideo(log *slog.Logger, envFile, id string, seekTo float64, listen time.Duration, lib string) error {
	ctx, cancel := context.WithTimeout(context.Background(), listen+5*time.Minute)
	defer cancel()
	api, _, err := signIn(ctx, envFile)
	if err != nil {
		return err
	}
	surface := video.NewSurface()
	eng := native.New(native.Options{Client: api, CDM: native.CDMPath(lib), Logger: log, IdleTimeout: -1, Video: surface})
	defer func() { _ = eng.Close() }()
	if err := eng.SetOutputDevice(ctx, outputDevice); err != nil {
		return err
	}
	rec := &recorder{events: make(chan stamped, 256)}
	go func() {
		for ev := range eng.Events() {
			rec.events <- stamped{time.Now(), ev}
		}
		close(rec.events)
	}()
	ps := &pictureStats{arrived: make(chan video.Frame, 1)}
	go ps.watch(ctx, surface, eng)

	t0 := time.Now()
	if err := eng.Play(ctx, playback.VideoID(id), 0); err != nil {
		return fmt.Errorf("play: %w", err)
	}
	st := eng.Status()
	info, _ := eng.Host()
	step("play → first audio (cold)", "latency", time.Since(t0).Round(time.Millisecond), "duration", st.Duration.Round(time.Millisecond))
	step("host", "pid", info.PID, "cdm", info.CDMVersion, "cdm_interface", info.CDMInterface, "sandbox", info.Sandbox.Kind())
	f, err := ps.wait(ctx, 10*time.Second, func(video.Frame) bool { return true })
	if err != nil {
		return fmt.Errorf("first picture: %w", err)
	}
	step("first picture", "after", time.Since(t0).Round(time.Millisecond), "size", fmt.Sprintf("%dx%d", f.Width, f.Height), "position", f.Position.Round(time.Millisecond))
	time.Sleep(8 * time.Second)
	if err := ps.report("from the start"); err != nil {
		return err
	}
	memory("memory while playing", info.PID)

	if seekTo > 0 {
		target := time.Duration(seekTo * float64(time.Second))
		t1 := time.Now()
		if err := eng.Seek(ctx, target); err != nil {
			return fmt.Errorf("seek: %w", err)
		}
		f, err := ps.wait(ctx, 10*time.Second, func(f video.Frame) bool { return f.Position >= target-time.Second })
		if err != nil {
			return fmt.Errorf("seek: no picture: %w", err)
		}
		step("seek → first picture", "target", target, "latency", time.Since(t1).Round(time.Millisecond), "position", f.Position.Round(time.Millisecond))
		ps.reset()
		time.Sleep(listen)
		if err := ps.report("after the seek"); err != nil {
			return err
		}
	}

	// The end: the last seconds play out, and the item ends.
	dur := eng.Status().Duration
	rec.drain()
	if err := eng.Seek(ctx, dur-8*time.Second); err != nil {
		return fmt.Errorf("seek to the end: %w", err)
	}
	if _, err := rec.waitFor(ctx, func(ev playback.Event) bool {
		return ev.Kind == playback.EventState && ev.Status.State == playback.StateEnded
	}); err != nil {
		return fmt.Errorf("waiting for the end: %w", err)
	}
	ps.mu.Lock()
	last := ps.last
	ps.mu.Unlock()
	step("played to the end", "duration", dur.Round(time.Millisecond), "last_picture", last.Position.Round(time.Millisecond))
	if last.Position < dur-time.Second {
		return fmt.Errorf("the last picture (%v) is more than a second before the end (%v)", last.Position, dur)
	}
	return eng.Stop(ctx)
}

// pictureStats watches a video surface.
type pictureStats struct {
	arrived chan video.Frame // the latest picture, for wait

	mu      sync.Mutex
	since   time.Time
	n       int
	last    video.Frame
	offsets []time.Duration // picture position less the audio position on arrival
	detail  []float64       // luma standard deviation of sampled pictures
	sizes   map[string]int
}

func (p *pictureStats) watch(ctx context.Context, s *video.Surface, eng *native.Engine) {
	var seq uint64
	for {
		f, ok := s.Next(ctx, seq)
		if ctx.Err() != nil {
			return
		}
		seq = f.Seq
		if !ok {
			continue
		}
		st := eng.Status()
		p.mu.Lock()
		if p.since.IsZero() {
			p.since = time.Now()
		}
		p.n++
		p.last = f
		if st.State == playback.StatePlaying {
			p.offsets = append(p.offsets, f.Position-(st.Position+time.Since(st.UpdatedAt)))
		}
		if p.n%25 == 1 {
			p.detail = append(p.detail, lumaDetail(f))
		}
		if p.sizes == nil {
			p.sizes = map[string]int{}
		}
		p.sizes[fmt.Sprintf("%dx%d", f.Width, f.Height)]++
		p.mu.Unlock()
		select {
		case <-p.arrived:
		default:
		}
		p.arrived <- f
	}
}

// wait returns the first picture to arrive within d that ok accepts.
func (p *pictureStats) wait(ctx context.Context, d time.Duration, ok func(video.Frame) bool) (video.Frame, error) {
	timeout := time.After(d)
	for {
		select {
		case f := <-p.arrived:
			if ok(f) {
				return f, nil
			}
		case <-timeout:
			return video.Frame{}, fmt.Errorf("none within %v", d)
		case <-ctx.Done():
			return video.Frame{}, ctx.Err()
		}
	}
}

func (p *pictureStats) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.since, p.n, p.offsets, p.detail, p.sizes = time.Time{}, 0, nil, nil, nil
}

// report prints the pictures since the last reset and fails when they
// are too few, hold no picture, or stray from the audio.
func (p *pictureStats) report(label string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	elapsed := time.Since(p.since)
	fps := float64(p.n) / elapsed.Seconds()
	off := slices.Clone(p.offsets)
	slices.Sort(off)
	var median, lo, hi time.Duration
	if len(off) > 0 {
		median, lo, hi = off[len(off)/2], off[len(off)/20], off[len(off)*19/20]
	}
	var detail float64
	for _, d := range p.detail {
		detail += d / float64(len(p.detail))
	}
	step("pictures "+label, "count", p.n, "fps", fmt.Sprintf("%.1f", fps), "sizes", fmt.Sprint(p.sizes),
		"ahead_of_audio_median", median.Round(time.Millisecond), "p5", lo.Round(time.Millisecond), "p95", hi.Round(time.Millisecond),
		"luma_detail", fmt.Sprintf("%.1f", detail))
	switch {
	case fps < 15:
		return fmt.Errorf("pictures %s: %.1f per second", label, fps)
	case detail < 4:
		return fmt.Errorf("pictures %s: no picture detail (luma deviation %.1f)", label, detail)
	case median < -40*time.Millisecond || median > 60*time.Millisecond:
		return fmt.Errorf("pictures %s: %v from the audio", label, median)
	}
	return nil
}

// lumaDetail is the standard deviation of a sample of f's luma: near 0
// for a blank picture.
func lumaDetail(f video.Frame) float64 {
	n := f.Width * f.Height
	if n == 0 || len(f.Data) < n {
		return 0
	}
	var sum, sq float64
	var k int
	for i := 0; i < n; i += 97 {
		v := float64(f.Data[i])
		sum, sq, k = sum+v, sq+v*v, k+1
	}
	mean := sum / float64(k)
	return math.Sqrt(max(sq/float64(k)-mean*mean, 0))
}

// Widevine licence policy fields (license_protocol.proto) the probe
// reads: License.key (3) → KeyContainer{id 1, type 4, level 5,
// required_protection 6 {hdcp 1}, track_label 12}. Key material (iv,
// key) is never read.
var (
	keyTypes  = map[uint64]string{1: "signing", 2: "content", 3: "key-control", 4: "operator-session", 5: "entitlement"}
	keyLevels = map[uint64]string{1: "SW_SECURE_CRYPTO", 2: "SW_SECURE_DECODE", 3: "HW_SECURE_CRYPTO", 4: "HW_SECURE_DECODE", 5: "HW_SECURE_ALL"}
)

// licencePolicy summarises the content keys' policy in a Widevine
// licence response (a SignedMessage whose msg, field 2, is the License):
// track label, security level and required HDCP of each.
func licencePolicy(signed []byte) string {
	msg := protoField(signed, 2)
	if msg == nil {
		return "?"
	}
	var out []string
	for _, kc := range protoFields(msg, 3) {
		typ := keyTypes[protoVarint(kc, 4)]
		if typ != "content" {
			continue
		}
		s := fmt.Sprintf("%s level=%s", string(protoField(kc, 12)), keyLevels[protoVarint(kc, 5)])
		if rp := protoField(kc, 6); rp != nil {
			s += fmt.Sprintf(" hdcp=%d", protoVarint(rp, 1))
		}
		out = append(out, s)
	}
	return strings.Join(out, "; ")
}

func protoField(b []byte, num uint64) []byte {
	if f := protoFields(b, num); len(f) > 0 {
		return f[0]
	}
	return nil
}

// protoFields returns the length-delimited fields num of message b.
func protoFields(b []byte, num uint64) [][]byte {
	var out [][]byte
	protoWalk(b, func(n, wire uint64, v uint64, data []byte) {
		if n == num && wire == 2 {
			out = append(out, data)
		}
	})
	return out
}

func protoVarint(b []byte, num uint64) uint64 {
	var r uint64
	protoWalk(b, func(n, wire uint64, v uint64, _ []byte) {
		if n == num && wire == 0 {
			r = v
		}
	})
	return r
}

func protoWalk(b []byte, fn func(num, wire, v uint64, data []byte)) {
	for len(b) > 0 {
		tag, n := binary.Uvarint(b)
		if n <= 0 {
			return
		}
		b = b[n:]
		switch wire := tag & 7; wire {
		case 0:
			v, n := binary.Uvarint(b)
			if n <= 0 {
				return
			}
			fn(tag>>3, 0, v, nil)
			b = b[n:]
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || uint64(len(b)-n) < l {
				return
			}
			fn(tag>>3, 2, 0, b[n:n+int(l)])
			b = b[n+int(l):]
		case 5:
			if len(b) < 4 {
				return
			}
			b = b[4:]
		case 1:
			if len(b) < 8 {
				return
			}
			b = b[8:]
		default:
			return
		}
	}
}
