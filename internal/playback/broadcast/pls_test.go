package broadcast

import (
	"slices"
	"testing"
)

// Shape of TuneIn's PLS for a broadcaster: its ident, then the stream.
const tuneInPLS = `[playlist]
NumberOfEntries=3
File2=http://stream.example/live?aw_0_1st.playerid=tunein
Title2=Live
Length2=-1
File1=http://cdn-cms.tunein.com/service/Audio/notcompatible.enUS.mp3
Title1=Ident
Length1=-1
file10=  http://stream.example/backup  
File3=
FileX=http://bad.example/
File=http://bad.example/
Version=2
`

func TestParsePLS(t *testing.T) {
	want := []string{
		"http://cdn-cms.tunein.com/service/Audio/notcompatible.enUS.mp3",
		"http://stream.example/live?aw_0_1st.playerid=tunein",
		"http://stream.example/backup",
	}
	if got := parsePLS([]byte(tuneInPLS)); !slices.Equal(got, want) {
		t.Fatalf("parsePLS = %q", got)
	}
	if got := parsePLS([]byte("[playlist]\r\nFile1=http://a/\r\nFile1=http://b/\r\n")); !slices.Equal(got, []string{"http://a/", "http://b/"}) {
		t.Fatalf("CRLF and duplicates: %q", got)
	}
	if got := parsePLS([]byte("[playlist]\nNumberOfEntries=0\n")); len(got) != 0 {
		t.Fatalf("no entries: %q", got)
	}
}

func TestIsPLS(t *testing.T) {
	for _, c := range []struct {
		b    string
		want bool
	}{
		{"[playlist]\nFile1=x", true},
		{"[Playlist]\nFile1=x", true},
		{"\ufeff\r\n  [PLAYLIST]\r\n", true},
		{"#EXTM3U\n", false},
		{"File1=x\n[playlist]", false},
		{"", false},
	} {
		if got := isPLS([]byte(c.b)); got != c.want {
			t.Errorf("isPLS(%q) = %v", c.b, got)
		}
	}
}

func TestParseM3U(t *testing.T) {
	for name, c := range map[string]struct {
		in   string
		want []string
	}{
		"plain":              {"http://a.example/one\r\n#EXTINF:-1,Station\n\n  http://a.example/two  \nrelative.mp3\n", []string{"http://a.example/one", "http://a.example/two", "relative.mp3"}},
		"BOM before a URL":   {"\ufeffhttp://a.example/one\n", []string{"http://a.example/one"}},
		"BOM before #EXTM3U": {"\ufeff#EXTM3U\n#EXTINF:-1,Station\nhttp://a.example/one\n", []string{"http://a.example/one"}},
		"comments only":      {"#EXTM3U\n# nothing\n", nil},
	} {
		if got := parseM3U([]byte(c.in)); !slices.Equal(got, c.want) {
			t.Errorf("%s: parseM3U = %q", name, got)
		}
	}
}

func TestIsHLS(t *testing.T) {
	if !isHLS([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXTINF:10,\na.ts\n")) {
		t.Error("media playlist")
	}
	if !isHLS([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nv.m3u8\n")) {
		t.Error("master playlist")
	}
	if isHLS([]byte("#EXTM3U\n#EXTINF:-1,Station\nhttp://a.example/live\n")) {
		t.Error("plain M3U")
	}
}
