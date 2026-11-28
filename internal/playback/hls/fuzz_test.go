package hls

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The seeds of these targets are in testdata/fuzz: Apple's song and radio
// playlists, a music video master, and the byte ranges of
// GHSA-q88j-3h4f-gqgf (a BYTERANGE of 2^48 bytes, offsets that continue one
// another).

// FuzzParse feeds Parse a media playlist (testdata/fuzz/FuzzParse: Apple's
// song playlist with its implicit byte ranges, a radio playlist with three
// key formats and a programme change, a video playlist with a Widevine pssh
// key, huge byte ranges).
//
// It checks that Parse does not panic, and that a playlist it returns, on
// success, is one the player can index and fetch from:
//   - it has segments, numbered from MediaSequence in order, each with a URI
//     that is not a tag;
//   - durations are not negative and the start times add up (segment i+1
//     starts where segment i ends), so Duration is the last segment's end;
//   - a byte range starts at a non-negative offset and ends within int64, so
//     the Range header is "bytes=a-b" with b at least a, or empty;
//   - a map has a URI and a non-negative range; a key has a method, and its
//     key ID is 16 bytes;
//   - SegmentAt gives an index of the playlist for any time, the segment
//     that starts last at or before it;
//   - written out again (URI, range, duration, title, discontinuity) and
//     parsed once more, the segments come back the same.
func FuzzParse(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := Parse(data)
		if err != nil {
			if p != nil {
				t.Fatalf("playlist with %v", err)
			}
			return
		}
		n := len(p.Segments)
		if n == 0 {
			t.Fatal("no segments")
		}
		small := true
		for i, s := range p.Segments {
			if s.URI == "" || strings.HasPrefix(s.URI, "#") {
				t.Fatalf("segment %d URI %q", i, s.URI)
			}
			if s.Seq != p.MediaSequence+i {
				t.Fatalf("segment %d has sequence %d, want %d", i, s.Seq, p.MediaSequence+i)
			}
			if s.Duration < 0 {
				t.Fatalf("segment %d lasts %v", i, s.Duration)
			}
			if i == 0 && s.Start != 0 || i > 0 && s.Start != p.Segments[i-1].Start+p.Segments[i-1].Duration {
				t.Fatalf("segment %d starts at %v", i, s.Start)
			}
			if s.Start < 0 {
				t.Fatalf("segment %d starts at %v", i, s.Start)
			}
			small = small && s.Duration < 24*time.Hour
			checkRange(t, i, s.Range)
			if m := s.Map; m != nil {
				if m.URI == "" {
					t.Fatalf("segment %d has a map without a URI", i)
				}
				checkRange(t, i, m.Range)
			}
			if k := s.Key; k != nil {
				if k.Method == "" || k.Method == "NONE" {
					t.Fatalf("segment %d key method %q", i, k.Method)
				}
				if k.KID != nil && len(k.KID) != 16 {
					t.Fatalf("segment %d key ID of %d bytes", i, len(k.KID))
				}
			}
			_, _ = Resolve("https://a.example/b/c.m3u8", s.URI)
		}
		last := p.Segments[n-1]
		if p.Duration() != last.Start+last.Duration || p.Duration() < 0 {
			t.Fatalf("duration %v, last segment ends at %v", p.Duration(), last.Start+last.Duration)
		}
		for _, at := range []time.Duration{math.MinInt64, -1, 0, p.Duration() / 2, p.Duration(), math.MaxInt64} {
			i := p.SegmentAt(at)
			if i < 0 || i >= n {
				t.Fatalf("SegmentAt(%v) = %d of %d", at, i, n)
			}
			if at >= 0 && p.Segments[i].Start > at || i+1 < n && at >= 0 && p.Segments[i+1].Start <= at {
				t.Fatalf("SegmentAt(%v) = %d, starting at %v (next at %v)", at, i, p.Segments[i].Start, p.Segments[min(i+1, n-1)].Start)
			}
		}
		if small {
			roundTrip(t, p)
		}
	})
}

func checkRange(t *testing.T, i int, r ByteRange) {
	t.Helper()
	if r.Offset < 0 || r.Length < 0 || r.Length > math.MaxInt64-r.Offset {
		t.Fatalf("segment %d: byte range %+v", i, r)
	}
	h := r.Header()
	if r.Length == 0 {
		if h != "" {
			t.Fatalf("segment %d: header %q for the whole resource", i, h)
		}
		return
	}
	var lo, hi int64
	if _, err := fmt.Sscanf(h, "bytes=%d-%d", &lo, &hi); err != nil || lo != r.Offset || hi < lo || hi-lo+1 != r.Length {
		t.Fatalf("segment %d: header %q for %+v (%v)", i, h, r, err)
	}
}

// roundTrip writes p's segments as a playlist, with every byte range spelled
// out, and checks that Parse reads the same segments back.
func roundTrip(t *testing.T, p *Playlist) {
	t.Helper()
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:" + strconv.Itoa(p.MediaSequence) + "\n")
	for _, s := range p.Segments {
		if s.Discontinuity {
			b.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		fmt.Fprintf(&b, "#EXTINF:%s,%s\n", strconv.FormatFloat(s.Duration.Seconds(), 'f', 9, 64), s.Title)
		if s.Range.Length > 0 {
			fmt.Fprintf(&b, "#EXT-X-BYTERANGE:%d@%d\n", s.Range.Length, s.Range.Offset)
		}
		b.WriteString(s.URI + "\n")
	}
	q, err := Parse([]byte(b.String()))
	if err != nil {
		t.Fatalf("parsing what Parse returned: %v\n%s", err, b.String())
	}
	if len(q.Segments) != len(p.Segments) || q.MediaSequence != p.MediaSequence {
		t.Fatalf("round trip: %d segments from %d\n%s", len(q.Segments), len(p.Segments), b.String())
	}
	for i, s := range p.Segments {
		g := q.Segments[i]
		if g.URI != s.URI || g.Range != s.Range || g.Duration != s.Duration || g.Start != s.Start || g.Title != s.Title || g.Discontinuity != s.Discontinuity || g.Seq != s.Seq {
			t.Fatalf("round trip, segment %d: %+v, was %+v\n%s", i, g, s, b.String())
		}
	}
}

// FuzzParseMaster feeds the master playlist parsers ParseMaster,
// ParseRenditions and IsMaster, and the selection of what to play from
// them (testdata/fuzz/FuzzParseMaster: Apple's music video master with its
// HDCP levels, ALLOWED-CPC lists and audio groups, a radio master).
//
// It checks that none of them panics, and that on success:
//   - ParseMaster returns variants that each have a URI, and IsMaster is true
//     for the data (it holds a stream variant);
//   - BestAAC returns one of the variants: the highest-bandwidth AAC-LC
//     one when there is any, else the highest-bandwidth one, the first of
//     equals (whatever the bandwidths, so a huge one cannot overflow into
//     losing);
//   - SelectVideo returns, when it returns, an H.264 variant with a picture,
//     no HDCP and software Widevine, the highest-bandwidth of those; and
//     when it does not, no variant qualifies;
//   - SelectAudio returns a rendition of type AUDIO with a URI, from the
//     stereo AAC 256 group or the variant's.
func FuzzParseMaster(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		rs, rerr := ParseRenditions(data)
		if rerr != nil && rs != nil {
			t.Fatalf("renditions with %v", rerr)
		}
		vs, err := ParseMaster(data)
		if err != nil {
			if vs != nil {
				t.Fatalf("variants with %v", err)
			}
			return
		}
		if len(vs) == 0 || !IsMaster(data) {
			t.Fatalf("%d variants, IsMaster %v", len(vs), IsMaster(data))
		}
		for i, v := range vs {
			if v.URI == "" {
				t.Fatalf("variant %d has no URI", i)
			}
			_ = v.SoftwareWidevine()
		}
		checkBestAAC(t, vs)
		checkSelectVideo(t, vs)
		if rerr == nil {
			for _, v := range vs {
				if r, ok := SelectAudio(rs, v); ok && (r.Type != "AUDIO" || r.URI == "" || r.GroupID != stereoAAC256 && r.GroupID != v.Audio) {
					t.Fatalf("SelectAudio = %+v for audio group %q", r, v.Audio)
				}
			}
		}
	})
}

func isAAC(v Variant) bool { return strings.Contains(v.Codecs, "mp4a.40.2") }

func checkBestAAC(t *testing.T, vs []Variant) {
	t.Helper()
	got := BestAAC(vs)
	anyAAC := slices.ContainsFunc(vs, isAAC)
	want := -1
	for i, v := range vs {
		if anyAAC && !isAAC(v) {
			continue
		}
		if want < 0 || v.Bandwidth > vs[want].Bandwidth {
			want = i
		}
	}
	if got != vs[want] {
		t.Fatalf("BestAAC = %+v, want %+v", got, vs[want])
	}
}

func checkSelectVideo(t *testing.T, vs []Variant) {
	t.Helper()
	qualifies := func(v Variant) bool {
		return v.Width != 0 && v.SoftwareWidevine() && (v.HDCPLevel == "" || v.HDCPLevel == "NONE") && strings.Contains(v.Codecs, "avc1")
	}
	got, ok := SelectVideo(vs)
	var qualifying []Variant
	for _, v := range vs {
		if qualifies(v) {
			qualifying = append(qualifying, v)
		}
	}
	if ok != (len(qualifying) > 0) {
		t.Fatalf("SelectVideo ok = %v with %d qualifying variants", ok, len(qualifying))
	}
	if !ok {
		return
	}
	sort.SliceStable(qualifying, func(i, j int) bool { return qualifying[i].Bandwidth > qualifying[j].Bandwidth })
	if got != qualifying[0] {
		t.Fatalf("SelectVideo = %+v, want %+v", got, qualifying[0])
	}
}

// FuzzParseAttributes feeds ParseAttributes an attribute list
// (testdata/fuzz/FuzzParseAttributes: EXT-X-STREAM-INF, EXT-X-KEY and
// EXT-X-MAP lists with quoted commas).
//
// It checks that it does not panic and that the list it returns, written out
// with every value quoted and parsed again, comes back the same (a value
// holding a quote, which an unquoted value may, is left out).
func FuzzParseAttributes(f *testing.F) {
	f.Fuzz(func(t *testing.T, s string) {
		attrs, err := ParseAttributes(s)
		if err != nil {
			if attrs != nil {
				t.Fatalf("attributes with %v", err)
			}
			return
		}
		names := make([]string, 0, len(attrs))
		for name, v := range attrs {
			if strings.Contains(v, `"`) {
				return
			}
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, len(names))
		for i, name := range names {
			parts[i] = name + `="` + attrs[name] + `"`
		}
		again, err := ParseAttributes(strings.Join(parts, ","))
		if err != nil || len(again) != len(attrs) {
			t.Fatalf("round trip of %q: %v, %v", attrs, again, err)
		}
		for name, v := range attrs {
			if again[name] != v {
				t.Fatalf("round trip of %q: %q is %q, was %q", attrs, name, again[name], v)
			}
		}
	})
}

// FuzzParse found durations and byte ranges that wrapped past int64 into
// negative ones: a segment of 1e10 seconds lasted −2562047 hours.
func TestParseRefusesNumbersThatWrap(t *testing.T) {
	for name, body := range map[string]string{
		"a duration past a Duration":   "#EXTM3U\n#EXTINF:1e10,\na.mp4\n",
		"an infinite duration":         "#EXTM3U\n#EXTINF:Inf,\na.mp4\n",
		"a duration that is no number": "#EXTM3U\n#EXTINF:NaN,\na.mp4\n",
		"a target duration past one":   "#EXTM3U\n#EXT-X-TARGETDURATION:1e300\n#EXTINF:1,\na.mp4\n",
		"a playlist longer than one":   "#EXTM3U\n#EXTINF:9e9,\na.mp4\n#EXTINF:9e9,\nb.mp4\n",
		"a range past int64":           "#EXTM3U\n#EXTINF:1,\n#EXT-X-BYTERANGE:5@9223372036854775807\na.mp4\n",
		"a map range past int64":       "#EXTM3U\n#EXT-X-MAP:URI=\"a.mp4\",BYTERANGE=\"5@9223372036854775807\"\n#EXTINF:1,\n#EXT-X-BYTERANGE:5\na.mp4\n",
		"an implicit range past it":    "#EXTM3U\n#EXTINF:1,\n#EXT-X-BYTERANGE:9223372036854775800@7\na.mp4\n#EXTINF:1,\n#EXT-X-BYTERANGE:5\na.mp4\n",
	} {
		if p, err := Parse([]byte(body)); err == nil {
			t.Errorf("%s: parsed, first segment %+v", name, p.Segments[0])
		}
	}
	// The longest a segment may be still parses, and stays positive.
	p, err := Parse([]byte("#EXTM3U\n#EXTINF:9e9,\na.mp4\n"))
	if err != nil || p.Segments[0].Duration != 9e9*time.Second {
		t.Errorf("9e9 s: %+v, %v", p, err)
	}
}

// FuzzParseMaster's seed of a bandwidth of 2^63-1: the bonus BestAAC added for
// AAC-LC wrapped it into a loss against a plain AAC variant.
func TestBestAACWithHugeBandwidth(t *testing.T) {
	vs := []Variant{
		{URI: "he", Bandwidth: math.MaxInt64, Codecs: "mp4a.40.5"},
		{URI: "lc", Bandwidth: 256000, Codecs: "mp4a.40.2"},
		{URI: "lc2", Bandwidth: math.MaxInt64, Codecs: "mp4a.40.2"},
	}
	if got := BestAAC(vs); got.URI != "lc2" {
		t.Errorf("BestAAC = %q, want lc2", got.URI)
	}
	if got := BestAAC(vs[:2]); got.URI != "lc" {
		t.Errorf("BestAAC = %q, want lc", got.URI)
	}
}
