package services

import (
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/motion"
)

func TestAlbumPageMotion(t *testing.T) {
	mc, err := motion.New(motion.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	s := detailsService(t)
	s.e.m = mapper{motion: mc}
	s.m = s.e.m
	// Trimmed from a live album with extend=editorialVideo: the square
	// flavours and a tall one.
	a := decode[applemusic.Album](t, `{"id":"7","type":"albums","attributes":{"name":"Midnights","editorialVideo":{
		"motionDetailTall":{"video":"https://mvod.itunes.apple.com/itunes-assets/HLSMusic122/tall_default.m3u8"},
		"motionSquareVideo1x1":{"video":"https://mvod.itunes.apple.com/itunes-assets/HLSMusic112/square1x1_default.m3u8"},
		"motionDetailSquare":{"video":"https://mvod.itunes.apple.com/itunes-assets/HLSMusic112/square_default.m3u8","previewFrame":{"url":"https://is1-ssl.mzstatic.com/image/thumb/Video112/x/{w}x{h}bb.jpg","width":3840,"height":3840}}}}}`)
	got := s.albumPage(t.Context(), a).Header.Motion
	want := mc.URL("https://mvod.itunes.apple.com/itunes-assets/HLSMusic112/square_default.m3u8")
	if got == "" || got != want || !strings.HasPrefix(got, "/motion/") {
		t.Errorf("motion = %q, want the motionDetailSquare URL %q", got, want)
	}

	// No motion artwork, or a video off Apple's CDNs: none offered.
	for _, body := range []string{
		`{"id":"8","type":"albums","attributes":{"name":"Plain"}}`,
		`{"id":"9","type":"albums","attributes":{"name":"Odd","editorialVideo":{"motionDetailSquare":{"video":"https://example.com/x.m3u8"}}}}`,
	} {
		if m := s.albumPage(t.Context(), decode[applemusic.Album](t, body)).Header.Motion; m != "" {
			t.Errorf("%s: motion = %q", body, m)
		}
	}
}
