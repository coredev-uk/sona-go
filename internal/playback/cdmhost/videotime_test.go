//go:build linux || (darwin && cgo)

package cdmhost

import (
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
	"github.com/coredev-uk/sona-go/internal/playback/widevine"
)

func TestPlanFrames(t *testing.T) {
	ms := func(v ...float64) []time.Duration {
		out := make([]time.Duration, len(v))
		for i, x := range v {
			out[i] = time.Duration(x * float64(time.Millisecond))
		}
		return out
	}
	const m = time.Millisecond
	for _, tc := range []struct {
		name    string
		pts     []time.Duration
		start   time.Duration
		pos     time.Duration
		havePos bool
		shown   bool
		want    framePlan
	}{
		{name: "empty queue", want: framePlan{wait: presentPoll}},
		{name: "empty queue, shown", havePos: true, shown: true, want: framePlan{wait: presentPoll}},
		{
			name: "first picture at once", pts: ms(0, 40, 80),
			want: framePlan{show: true, wait: presentMin},
		},
		{
			// Not shown: the position is not consulted.
			name: "first picture at once, audio far behind", pts: ms(5000), start: 5000 * m, pos: 0, havePos: true,
			want: framePlan{show: true, wait: presentMin},
		},
		{
			name: "superseded before start dropped", pts: ms(0, 40, 80, 120), start: 90 * m,
			want: framePlan{drop: 2, show: true, wait: presentMin},
		},
		{
			name: "picture exactly at start kept", pts: ms(0, 40, 80, 120), start: 80 * m,
			want: framePlan{drop: 2, show: true, wait: presentMin},
		},
		{
			name: "lone early picture kept", pts: ms(900), start: 1000 * m,
			want: framePlan{show: true, wait: presentMin},
		},
		{
			name: "lone picture at the stale bound kept", pts: ms(800), start: 1000 * m,
			want: framePlan{show: true, wait: presentMin},
		},
		{
			name: "lone stale picture dropped", pts: ms(799), start: 1000 * m,
			want: framePlan{drop: 1, wait: presentPoll},
		},
		{
			name: "all superseded, last stale", pts: ms(0, 40, 80), start: 1000 * m, havePos: true, shown: true,
			want: framePlan{drop: 3, wait: presentPoll},
		},
		{
			name: "newest due shown, earlier late", pts: ms(1000, 1040, 1080, 1120), pos: 1050 * m, havePos: true, shown: true,
			want: framePlan{drop: 1, show: true, wait: presentMin},
		},
		{
			name: "due within the lead", pts: ms(1000, 1040, 1070, 1120), pos: 1050 * m, havePos: true, shown: true,
			want: framePlan{drop: 2, show: true, wait: presentMin},
		},
		{
			name: "head due", pts: ms(1020, 1060), pos: 1000 * m, havePos: true, shown: true,
			want: framePlan{show: true, wait: presentMin},
		},
		{
			name: "not due yet", pts: ms(1030, 1070), pos: 1000 * m, havePos: true, shown: true,
			want: framePlan{wait: 10 * m},
		},
		{
			name: "not due, wait capped", pts: ms(1100, 1140), pos: 1000 * m, havePos: true, shown: true,
			want: framePlan{wait: presentPoll},
		},
		{
			name: "not due, wait floored", pts: ms(1020.5), pos: 1000 * m, havePos: true, shown: true,
			want: framePlan{wait: presentMin},
		},
		{
			name: "no position after shown", pts: ms(1000, 1040), pos: 5000 * m, shown: true,
			want: framePlan{wait: presentPoll},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := planFrames(tc.pts, tc.start, tc.pos, tc.havePos, tc.shown); got != tc.want {
				t.Fatalf("plan %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestVideoTime(t *testing.T) {
	const tick = 3000 * time.Second / 90000 // 3000 units of 90 kHz
	for _, tc := range []struct {
		name string
		s    fmp4.Sample
		in   fmp4.Init
		want time.Duration
	}{
		{
			name: "no edit", s: fmp4.Sample{DecodeTime: 2 * 90000, CTO: 3000},
			in: fmp4.Init{Timescale: 90000}, want: 2*time.Second + tick,
		},
		{
			name: "edit ignored without HasEdit", s: fmp4.Sample{DecodeTime: 2 * 90000},
			in: fmp4.Init{Timescale: 90000, EditStart: 9000}, want: 2 * time.Second,
		},
		{
			name: "edit", s: fmp4.Sample{DecodeTime: 2 * 90000, CTO: 3000},
			in: fmp4.Init{Timescale: 90000, HasEdit: true, EditStart: 9000}, want: 2*time.Second + tick - 100*time.Millisecond,
		},
		{
			name: "before the edit", s: fmp4.Sample{},
			in: fmp4.Init{Timescale: 90000, HasEdit: true, EditStart: 9000}, want: -100 * time.Millisecond,
		},
		{
			name: "negative composition offset", s: fmp4.Sample{CTO: -3000},
			in: fmp4.Init{Timescale: 90000}, want: -tick,
		},
		{
			name: "other timescale", s: fmp4.Sample{DecodeTime: 1001, CTO: 1001},
			in: fmp4.Init{Timescale: 24000}, want: time.Second * 2002 / 24000,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := videoTime(&tc.s, &tc.in); got != tc.want {
				t.Fatalf("videoTime %v, want %v", got, tc.want)
			}
		})
	}
}

// testAVCC is an avcC body of profile_idc profile with one SPS and one PPS
// and 4-byte NAL lengths.
func testAVCC(profile byte) []byte {
	return []byte{
		1, profile, 0, 30, 0xff, 0xe1,
		0, 4, 0x67, profile, 0, 30,
		1, 0, 4, 0x68, 0xee, 0x3c, 0x80,
	}
}

func TestVideoConfig(t *testing.T) {
	video := func(codec string, profile byte, scheme string) *fmp4.Init {
		return &fmp4.Init{
			Timescale: 90000, Codec: codec, Width: 864, Height: 486, VideoConfig: testAVCC(profile),
			Protected: scheme != "", Scheme: scheme,
		}
	}
	sized := func(w, h int) *fmp4.Init {
		in := video("avc1", 100, "cbcs")
		in.Width, in.Height = w, h
		return in
	}
	for _, tc := range []struct {
		name string
		in   *fmp4.Init
		want widevine.VideoConfig
		err  string
	}{
		{
			name: "High cbcs", in: video("avc1", 100, "cbcs"),
			want: widevine.VideoConfig{Codec: widevine.CodecH264, Profile: widevine.ProfileH264High, Width: 864, Height: 486, CBCS: true},
		},
		{
			name: "Main cenc", in: video("avc1", 77, "cenc"),
			want: widevine.VideoConfig{Codec: widevine.CodecH264, Profile: widevine.ProfileH264Main, Width: 864, Height: 486},
		},
		{
			name: "Baseline clear avc3", in: video("avc3", 66, ""),
			want: widevine.VideoConfig{Codec: widevine.CodecH264, Profile: widevine.ProfileH264Baseline, Width: 864, Height: 486},
		},
		{
			name: "largest size", in: sized(MaxVideoSize, MaxVideoSize),
			want: widevine.VideoConfig{Codec: widevine.CodecH264, Profile: widevine.ProfileH264High, Width: MaxVideoSize, Height: MaxVideoSize, CBCS: true},
		},
		{
			name: "audio init", err: "no video track",
			in: &fmp4.Init{Timescale: 44100, Codec: "mp4a", AudioConfig: []byte{0x12, 0x10}, Protected: true, Scheme: "cbcs"},
		},
		{name: "hvc1", in: video("hvc1", 100, "cbcs"), err: "H.264 only"},
		{name: "hev1", in: video("hev1", 100, "cbcs"), err: "H.264 only"},
		{name: "cens", in: video("avc1", 100, "cens"), err: "encryption scheme"},
		{name: "unknown profile", in: video("avc1", 44, "cbcs"), err: "profile 44"},
		{name: "bad avcC", in: func() *fmp4.Init { in := video("avc1", 100, "cbcs"); in.VideoConfig = []byte{1, 100}; return in }(), err: "decoder configuration"},
		{name: "too wide", in: sized(MaxVideoSize+1, 486), err: "video size"},
		{name: "too tall", in: sized(864, MaxVideoSize+1), err: "video size"},
		{name: "no size", in: sized(0, 486), err: "video size"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, c, err := videoConfig(tc.in)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err %v, want one mentioning %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Codec != tc.want.Codec || got.Profile != tc.want.Profile || got.Width != tc.want.Width ||
				got.Height != tc.want.Height || got.CBCS != tc.want.CBCS || got.ExtraData != nil {
				t.Fatalf("config %+v, want %+v", got, tc.want)
			}
			if c == nil || c.LengthSize != 4 || c.Profile != tc.in.VideoConfig[1] {
				t.Fatalf("avc config %+v", c)
			}
		})
	}
}

func TestI420Size(t *testing.T) {
	for _, tc := range []struct{ w, h, want int }{
		{864, 486, 864*486 + 2*432*243},
		{2, 2, 6},
		{1, 1, 3},
		{3, 3, 9 + 2*2*2},
		{5, 2, 10 + 2*3*1},
		{0, 0, 0},
	} {
		if got := I420Size(tc.w, tc.h); got != tc.want {
			t.Errorf("I420Size(%d, %d) = %d, want %d", tc.w, tc.h, got, tc.want)
		}
	}
}

func TestVideoFrameValid(t *testing.T) {
	frame := func(w, h, n int) *VideoFrame { return &VideoFrame{Width: w, Height: h, Data: make([]byte, n)} }
	for _, tc := range []struct {
		name string
		f    *VideoFrame
		want bool
	}{
		{name: "SD", f: frame(864, 486, I420Size(864, 486)), want: true},
		{name: "odd size", f: frame(3, 5, I420Size(3, 5)), want: true},
		{name: "largest", f: frame(MaxVideoSize, 2, I420Size(MaxVideoSize, 2)), want: true},
		{name: "short data", f: frame(864, 486, I420Size(864, 486)-1)},
		{name: "long data", f: frame(3, 5, I420Size(3, 5)+1)},
		{name: "odd size, even chroma", f: frame(3, 5, 15+2*(3/2)*(5/2))},
		{name: "no width", f: frame(0, 2, 0)},
		{name: "negative height", f: frame(2, -2, 0)},
		{name: "too wide", f: frame(MaxVideoSize+1, 2, I420Size(MaxVideoSize+1, 2))},
		{name: "too tall", f: frame(2, MaxVideoSize+1, I420Size(2, MaxVideoSize+1))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.f.valid(); got != tc.want {
				t.Fatalf("valid() = %v, want %v", got, tc.want)
			}
		})
	}
}
