package broadcast

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"testing"
	"testing/iotest"
)

type icyTitle struct {
	at    int // audio bytes read before it
	title string
}

// readICY reads r to its end through an icyReader with buffers of n bytes.
func readICY(r io.Reader, metaint, n int) ([]byte, []icyTitle, error) {
	var (
		audio  []byte
		titles []icyTitle
	)
	ir := newICYReader(r, metaint, func(title string) { titles = append(titles, icyTitle{len(audio), title}) })
	buf := make([]byte, n)
	for {
		k, err := ir.Read(buf)
		audio = append(audio, buf[:k]...)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}
			return audio, titles, err
		}
	}
}

func TestICYReader(t *testing.T) {
	const metaint = 32
	a := func(fill byte) []byte { return bytes.Repeat([]byte{fill}, metaint) }
	stream := slices.Concat(
		a(1), icyBlock("StreamTitle='Artist - Song';StreamUrl='https://example.com';"),
		a(2), []byte{0}, // unchanged
		a(3), icyBlock("StreamUrl='x';"), // no title
		a(4), icyBlock("StreamTitle='';"),
		a(5), icyBlock("StreamTitle='Next';"),
		a(6)[:10], // the stream ends mid-way
	)
	wantAudio := slices.Concat(a(1), a(2), a(3), a(4), a(5), a(6)[:10])
	wantTitles := []icyTitle{{32, "Artist - Song"}, {4 * 32, ""}, {5 * 32, "Next"}}
	for _, c := range []struct {
		name string
		r    func() io.Reader
		n    int
	}{
		{"whole", func() io.Reader { return bytes.NewReader(stream) }, 4096},
		{"small buffer", func() io.Reader { return bytes.NewReader(stream) }, 5},
		{"one byte at a time", func() io.Reader { return iotest.OneByteReader(bytes.NewReader(stream)) }, 4096},
		{"half reads", func() io.Reader { return iotest.HalfReader(bytes.NewReader(stream)) }, 64},
	} {
		audio, titles, err := readICY(c.r(), metaint, c.n)
		if err != nil || !bytes.Equal(audio, wantAudio) || !slices.Equal(titles, wantTitles) {
			t.Errorf("%s: %d audio bytes, titles %+v, err %v", c.name, len(audio), titles, err)
		}
	}
}

func TestICYReaderWithoutMetadata(t *testing.T) {
	stream := []byte("StreamTitle='not metadata';" + string(make([]byte, 100)))
	audio, titles, err := readICY(bytes.NewReader(stream), 0, 16)
	if err != nil || !bytes.Equal(audio, stream) || len(titles) != 0 {
		t.Fatalf("%d bytes, %+v, %v", len(audio), titles, err)
	}
}

func TestICYReaderEnds(t *testing.T) {
	audio := bytes.Repeat([]byte{7}, 16)
	block := icyBlock("StreamTitle='Song';")
	for _, c := range []struct {
		name string
		b    []byte
		want error
	}{
		{"at a metadata block", audio, nil},
		{"in a metadata block", slices.Concat(audio, block[:5]), io.ErrUnexpectedEOF},
		{"after a metadata block", slices.Concat(audio, block), nil},
	} {
		got, _, err := readICY(bytes.NewReader(c.b), 16, 64)
		if !errors.Is(err, c.want) || (c.want == nil && err != nil) || !bytes.Equal(got, audio) {
			t.Errorf("%s: %d bytes, %v; want %v", c.name, len(got), err, c.want)
		}
	}
	// Other errors pass through.
	boom := errors.New("connection reset")
	_, _, err := readICY(io.MultiReader(bytes.NewReader(audio[:8]), iotest.ErrReader(boom)), 16, 64)
	if !errors.Is(err, boom) {
		t.Fatalf("read error: %v", err)
	}
}

func TestStreamTitle(t *testing.T) {
	for _, c := range []struct {
		block string
		want  string
		ok    bool
	}{
		{"StreamTitle='Artist - Song';", "Artist - Song", true},
		{"StreamTitle='Artist - Song';StreamUrl='http://x';\x00\x00\x00", "Artist - Song", true},
		{"StreamUrl='http://x';StreamTitle='Song';", "Song", true},
		{"StreamTitle='Guns N' Roses - Sweet Child O' Mine';", "Guns N' Roses - Sweet Child O' Mine", true},
		{"StreamTitle='  padded  ';", "padded", true},
		{"StreamTitle='unterminated'\x00\x00", "unterminated", true},
		{"StreamTitle='no quote\x00\x00", "no quote", true},
		{"StreamTitle='';", "", true},
		{"StreamTitle='Caf\xe9 del Mar';", "Café del Mar", true}, // ISO-8859-1
		{"StreamTitle='Café del Mar';", "Café del Mar", true},
		{"StreamUrl='http://x';", "", false},
		{"", "", false},
		{"streamtitle='lower';", "", false},
	} {
		got, ok := streamTitle(c.block)
		if got != c.want || ok != c.ok {
			t.Errorf("streamTitle(%q) = %q, %v; want %q, %v", c.block, got, ok, c.want, c.ok)
		}
	}
}
