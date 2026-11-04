package theme

import "testing"

func TestFontsAreEmbeddedTTFs(t *testing.T) {
	for _, name := range []string{"SchibstedGrotesk-Variable.ttf", "SchibstedGrotesk-Italic-Variable.ttf"} {
		b, err := fontFiles.ReadFile("fonts/" + name)
		if err != nil {
			t.Fatal(err)
		}
		// A TrueType file starts with 0x00010000 (or "true"/"OTTO").
		if len(b) < 100_000 || string(b[:4]) != "\x00\x01\x00\x00" {
			t.Errorf("%s: %d bytes, header %x", name, len(b), b[:4])
		}
	}
}

func TestRegisterFonts(t *testing.T) {
	if err := RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	if err := RegisterFonts(); err != nil {
		t.Fatalf("second call: %v", err)
	}
}
