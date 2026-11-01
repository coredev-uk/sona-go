package broadcast

import (
	"io"
	"strings"
	"unicode/utf8"

	"github.com/coredev-uk/sona-go/internal/playback/id3"
)

// Shoutcast/Icecast ("ICY") streams interleave metadata with the audio
// when the request asks for it (Icy-MetaData: 1): after every metaint
// audio bytes (the icy-metaint response header) comes one length byte
// (×16) and that many bytes of metadata, "StreamTitle='…';StreamUrl='…';"
// padded with NULs. A length of 0 means unchanged.

// icyReader splits an ICY stream into audio (Read) and metadata (meta).
type icyReader struct {
	r       io.Reader
	metaint int // audio bytes between metadata blocks; 0 for none
	left    int // audio bytes until the next metadata block
	meta    func(title string)
	block   [255 * 16]byte
}

func newICYReader(r io.Reader, metaint int, meta func(title string)) *icyReader {
	return &icyReader{r: r, metaint: metaint, left: metaint, meta: meta}
}

// Read reads audio bytes, handing the metadata blocks it passes to meta.
func (ir *icyReader) Read(p []byte) (int, error) {
	if ir.metaint <= 0 {
		return ir.r.Read(p)
	}
	if ir.left == 0 {
		if err := ir.readMeta(); err != nil {
			return 0, err
		}
		ir.left = ir.metaint
	}
	n, err := ir.r.Read(p[:min(len(p), ir.left)])
	ir.left -= n
	return n, err
}

func (ir *icyReader) readMeta() error {
	var l [1]byte
	if _, err := io.ReadFull(ir.r, l[:]); err != nil {
		return err
	}
	n := int(l[0]) * 16
	if n == 0 {
		return nil
	}
	b := ir.block[:n]
	if _, err := io.ReadFull(ir.r, b); err != nil {
		return noEOF(err)
	}
	if title, ok := streamTitle(string(b)); ok {
		ir.meta(title)
	}
	return nil
}

// noEOF turns an EOF in the middle of something into ErrUnexpectedEOF.
func noEOF(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

// streamTitle returns the StreamTitle of an ICY metadata block, and
// whether the block has one.
func streamTitle(block string) (string, bool) {
	const key = "StreamTitle='"
	i := strings.Index(block, key)
	if i < 0 {
		return "", false
	}
	v := block[i+len(key):]
	// The value ends at "';" (titles may contain quotes), or at the end.
	if j := strings.Index(v, "';"); j >= 0 {
		v = v[:j]
	} else {
		v = strings.TrimRight(v, "\x00")
		v = strings.TrimSuffix(v, "'")
	}
	// Servers send UTF-8 or, older ones, ISO-8859-1.
	if !utf8.ValidString(v) {
		v = id3.Decode(0, []byte(v))
	}
	return strings.TrimSpace(v), true
}
