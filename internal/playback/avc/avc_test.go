package avc

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
)

var (
	testSPS = []byte{0x67, 0x64, 0x00, 0x1e, 0xac}
	testPPS = []byte{0x68, 0xee, 0x3c, 0x80}
)

// record builds an avcC body: version 1, profile 100, compatibility 0,
// level 30, lengthByte's low 2 bits the NAL length size less one, and
// the given parameter sets.
func record(lengthByte byte, sps, pps [][]byte) []byte {
	b := []byte{1, 100, 0, 30, 0xfc | lengthByte, 0xe0 | byte(len(sps))}
	for _, s := range sps {
		b = append(b, byte(len(s)>>8), byte(len(s)))
		b = append(b, s...)
	}
	b = append(b, byte(len(pps)))
	for _, p := range pps {
		b = append(b, byte(len(p)>>8), byte(len(p)))
		b = append(b, p...)
	}
	return b
}

// nal is one NAL unit with an L-byte length prefix.
func nal(L int, payload ...byte) []byte {
	b := make([]byte, L, L+len(payload))
	for i, l := L-1, len(payload); i >= 0; i, l = i-1, l>>8 {
		b[i] = byte(l)
	}
	return append(b, payload...)
}

// annexB is the start-code form of the NAL unit payloads.
func annexB(nals ...[]byte) []byte {
	var b []byte
	for _, n := range nals {
		b = append(append(b, 0, 0, 0, 1), n...)
	}
	return b
}

func mustConfig(t *testing.T, lengthByte byte) *Config {
	t.Helper()
	c, err := ParseConfig(record(lengthByte, [][]byte{testSPS}, [][]byte{testPPS}))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestParseConfig(t *testing.T) {
	one := [][]byte{testSPS}
	two := [][]byte{testPPS}
	valid := record(3, one, two)
	for _, tc := range []struct {
		name   string
		b      []byte
		length int
		err    bool
	}{
		{name: "4-byte lengths", b: valid, length: 4},
		{name: "2-byte lengths", b: record(1, one, two), length: 2},
		{name: "1-byte lengths", b: record(0, one, two), length: 1},
		{name: "short", b: valid[:6], err: true},
		{name: "version 2", b: append([]byte{2}, valid[1:]...), err: true},
		{name: "3-byte lengths", b: record(2, one, two), err: true},
		{name: "SPS truncated", b: valid[:9], err: true},
		{name: "SPS length truncated", b: valid[:7], err: true},
		{name: "no PPS count", b: valid[:6+2+len(testSPS)], err: true},
		{name: "PPS truncated", b: valid[:len(valid)-1], err: true},
		{name: "empty SPS", b: record(3, [][]byte{{}}, two), err: true},
		{name: "no SPS", b: record(3, nil, two), err: true},
		{name: "no PPS", b: record(3, one, nil), err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := ParseConfig(tc.b)
			if tc.err {
				if !errors.Is(err, ErrConfig) {
					t.Fatalf("err %v, want ErrConfig", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.Profile != 100 || c.Compatibility != 0 || c.Level != 30 || c.LengthSize != tc.length {
				t.Fatalf("config %+v", c)
			}
			if len(c.SPS) != 1 || !bytes.Equal(c.SPS[0], testSPS) || len(c.PPS) != 1 || !bytes.Equal(c.PPS[0], testPPS) {
				t.Fatalf("parameter sets %x %x", c.SPS, c.PPS)
			}
			if want := annexB(testSPS, testPPS); !bytes.Equal(c.paramSets, want) {
				t.Fatalf("paramSets %x, want %x", c.paramSets, want)
			}
		})
	}
}

func TestAnnexBClear(t *testing.T) {
	slice := []byte{0x41, 0x9a, 0x02, 0x03}
	sei := []byte{0x06, 0x05, 0x01}
	idr := []byte{0x65, 0x88, 0x84, 0x00}
	for _, tc := range []struct {
		name       string
		lengthByte byte
		sample     []byte
		want       []byte
	}{
		{
			name: "4-byte prefixes", lengthByte: 3,
			sample: slices.Concat(nal(4, sei...), nal(4, slice...)),
			want:   annexB(sei, slice),
		},
		{
			name: "2-byte prefixes", lengthByte: 1,
			sample: slices.Concat(nal(2, sei...), nal(2, slice...)),
			want:   annexB(sei, slice),
		},
		{
			name: "IDR gets parameter sets", lengthByte: 3,
			sample: slices.Concat(nal(4, sei...), nal(4, idr...)),
			want:   annexB(testSPS, testPPS, sei, idr),
		},
		{
			name: "IDR with in-band SPS", lengthByte: 1,
			sample: slices.Concat(nal(2, testSPS...), nal(2, testPPS...), nal(2, idr...)),
			want:   annexB(testSPS, testPPS, idr),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := mustConfig(t, tc.lengthByte)
			got, subs, err := c.AnnexB(nil, tc.sample, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if subs != nil {
				t.Fatalf("subsamples %v for a clear sample", subs)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("got %x\nwant %x", got, tc.want)
			}
		})
	}
}

// cipherRuns is the encrypted bytes of b under subs, with each run's offset.
func cipherRuns(b []byte, subs []fmp4.Subsample) (data []byte, offsets []int) {
	off := 0
	for _, s := range subs {
		off += int(s.Clear)
		offsets = append(offsets, off)
		data = append(data, b[off:off+int(s.Cipher)]...)
		off += int(s.Cipher)
	}
	return data, offsets
}

func TestAnnexBEncrypted(t *testing.T) {
	enc := func(seed byte) []byte {
		b := make([]byte, 16)
		for i := range b {
			b[i] = seed + byte(i)
		}
		return b
	}
	sei := []byte{0x06, 0x05, 0x01}
	idr := slices.Concat([]byte{0x65, 0x88, 0x84}, enc(0x10)) // 3 clear bytes, then 16 encrypted
	slice := slices.Concat([]byte{0x41, 0x9a}, enc(0x40))     // 2 clear, 16 encrypted
	for _, tc := range []struct {
		name       string
		lengthByte byte
		L          int
		nals       [][]byte
		subs       []fmp4.Subsample
		want       []fmp4.Subsample
		params     bool // SPS and PPS prepended
	}{
		{
			name: "2-byte prefixes, IDR", lengthByte: 1, L: 2,
			nals: [][]byte{sei, idr, slice},
			// SEI and the IDR's head share the first subsample.
			subs: []fmp4.Subsample{{Clear: 2 + 3 + 2 + 3, Cipher: 16}, {Clear: 2 + 2, Cipher: 16}},
			want: []fmp4.Subsample{
				{Clear: 2 + 3 + 2 + 3 + 2*2 + uint32(len(annexB(testSPS, testPPS))), Cipher: 16},
				{Clear: 2 + 2 + 2, Cipher: 16},
			},
			params: true,
		},
		{
			name: "1-byte prefixes, no IDR", lengthByte: 0, L: 1,
			nals: [][]byte{slice, slice},
			subs: []fmp4.Subsample{{Clear: 1 + 2, Cipher: 16}, {Clear: 1 + 2, Cipher: 16}},
			want: []fmp4.Subsample{{Clear: 1 + 2 + 3, Cipher: 16}, {Clear: 1 + 2 + 3, Cipher: 16}},
		},
		{
			name: "4-byte prefixes, IDR", lengthByte: 3, L: 4,
			nals: [][]byte{idr, slice},
			subs: []fmp4.Subsample{{Clear: 4 + 3, Cipher: 16}, {Clear: 4 + 2, Cipher: 16}},
			want: []fmp4.Subsample{
				{Clear: 4 + 3 + uint32(len(annexB(testSPS, testPPS))), Cipher: 16},
				{Clear: 4 + 2, Cipher: 16},
			},
			params: true,
		},
		{
			name: "IDR with in-band SPS", lengthByte: 1, L: 2,
			nals: [][]byte{testSPS, testPPS, idr},
			subs: []fmp4.Subsample{{Clear: 2 + 5 + 2 + 4 + 2 + 3, Cipher: 16}},
			want: []fmp4.Subsample{{Clear: 2 + 5 + 2 + 4 + 2 + 3 + 3*2, Cipher: 16}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := mustConfig(t, tc.lengthByte)
			var sample []byte
			for _, n := range tc.nals {
				sample = append(sample, nal(tc.L, n...)...)
			}
			subs := slices.Clone(tc.subs)
			got, gotSubs, err := c.AnnexB(nil, sample, subs, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(subs, tc.subs) {
				t.Fatalf("input subsamples changed: %v", subs)
			}
			if !slices.Equal(gotSubs, tc.want) {
				t.Fatalf("subsamples %v, want %v", gotSubs, tc.want)
			}
			want := annexB(tc.nals...)
			if tc.params {
				want = slices.Concat(annexB(testSPS, testPPS), want)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("got %x\nwant %x", got, want)
			}
			total := 0
			for _, s := range gotSubs {
				total += int(s.Clear) + int(s.Cipher)
			}
			if total != len(got) {
				t.Fatalf("subsamples cover %d of %d bytes", total, len(got))
			}
			// The encrypted bytes are the same, each run moved by the
			// clear bytes added before it.
			inData, inOff := cipherRuns(sample, tc.subs)
			outData, outOff := cipherRuns(got, gotSubs)
			if !bytes.Equal(inData, outData) {
				t.Fatalf("encrypted bytes %x, want %x", outData, inData)
			}
			for i := range inOff {
				grow := 0
				for j := range i + 1 {
					grow += int(gotSubs[j].Clear) - int(tc.subs[j].Clear)
				}
				if outOff[i] != inOff[i]+grow {
					t.Fatalf("run %d at %d, want %d", i, outOff[i], inOff[i]+grow)
				}
			}
		})
	}
}

func TestAnnexBErrors(t *testing.T) {
	c := mustConfig(t, 3)
	slice := []byte{0x41, 0x9a, 0x02, 0x03, 0x04, 0x05}
	two := slices.Concat(nal(4, slice...), nal(4, slice...))
	for _, tc := range []struct {
		name   string
		sample []byte
		subs   []fmp4.Subsample
	}{
		{name: "NAL past the end", sample: []byte{0, 0, 0, 9, 0x41, 0x9a}},
		{name: "prefix past the end", sample: slices.Concat(nal(4, slice...), []byte{0, 0})},
		{name: "first prefix encrypted", sample: two, subs: []fmp4.Subsample{{Clear: 2, Cipher: uint32(len(two) - 2)}}},
		{name: "NAL header encrypted", sample: two, subs: []fmp4.Subsample{{Clear: 4, Cipher: uint32(len(two) - 4)}}},
		{
			name: "second prefix encrypted", sample: two,
			subs: []fmp4.Subsample{{Clear: 5, Cipher: uint32(len(two) - 5)}},
		},
		{name: "subsamples short", sample: two, subs: []fmp4.Subsample{{Clear: 5, Cipher: 5}}},
		{name: "subsamples long", sample: two, subs: []fmp4.Subsample{{Clear: 5, Cipher: uint32(len(two))}}},
		{name: "encrypted whole", sample: two, subs: []fmp4.Subsample{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := c.AnnexB(nil, tc.sample, tc.subs, nil); !errors.Is(err, ErrSample) {
				t.Fatalf("err %v, want ErrSample", err)
			}
		})
	}
}
