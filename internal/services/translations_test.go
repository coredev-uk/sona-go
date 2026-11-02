package services

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/i18n/applestrings"
)

type chanEmitter chan AppleTranslations

func (c chanEmitter) Emit(name string, data ...any) bool {
	if name == EventAppleTranslations {
		c <- data[0].(AppleTranslations)
	}
	return true
}

func TestTranslationsServiceFetchesAfterFirstPaintAndAnnounces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/us/home":
			_, _ = io.WriteString(w, `<script src="/assets/index~ab12.js"></script>`)
		case "/assets/index~ab12.js":
			_, _ = io.WriteString(w, `{"../locales/de-de/translations.json":()=>i(()=>import("./translations~cd34.js"),[])}`)
		case "/assets/translations~cd34.js":
			_, _ = io.WriteString(w, `const o={"A":"Hinzufügen","B":"Wiedergabe von @@x@@"};export{o as default};`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cat := applestrings.Catalogue{
		Mappings: []applestrings.Mapping{{Message: "Add", Key: "A"}, {Message: "Play", Key: "B"}},
		Messages: 4,
		Version:  "v",
	}
	store, err := applestrings.New(applestrings.Options{Dir: t.TempDir(), HTTP: srv.Client(), Origin: srv.URL, Catalogue: &cat})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	events := make(chanEmitter, 2)
	gate := NewGate(time.Hour, nil)
	e := &Env{Deps: Deps{AppleStrings: store, Gate: gate, Emit: events, Logger: slog.New(slog.DiscardHandler), Started: time.Now()}}
	s := newTranslationsService(e)
	defer e.close()

	if got := s.Apple("de-AT"); got.Locale != "" || got.Messages == nil || len(got.Messages) != 0 {
		t.Fatalf("before the first paint = %+v", got)
	}
	gate.Open()
	var got AppleTranslations
	select {
	case got = <-events:
	case <-time.After(5 * time.Second):
		t.Fatal("no translations:apple event")
	}
	// "Play" is refused: Apple's text has a placeholder Sona's lacks.
	add := applestrings.MessageID("Add", "")
	if got.Locale != "de-DE" || len(got.Messages) != 1 || got.Messages[add] != "Hinzufügen" || got.Coverage != 0.25 ||
		got.Version != "translations~cd34.js" || got.FetchedAt == 0 {
		t.Fatalf("event = %+v", got)
	}
	if again := s.Apple("de"); again.Messages[add] != "Hinzufügen" {
		t.Fatalf("Apple after download = %+v", again)
	}
	if none := (&TranslationsService{e: &Env{}}).Apple("de"); none.Locale != "" || none.Messages == nil {
		t.Fatalf("without a store = %+v", none)
	}
}

type languagesEmitter chan AppleLanguages

func (c languagesEmitter) Emit(name string, data ...any) bool {
	if name == EventAppleLanguages {
		c <- data[0].(AppleLanguages)
	}
	return true
}

// TestTranslationsServiceLanguages: the Language setting's Apple locales
// come only when the mapping could reach the share asked for, and the
// locale table, fetched after the first paint, is announced.
func TestTranslationsServiceLanguages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/us/home":
			_, _ = io.WriteString(w, `<script src="/assets/index~ab12.js"></script>`)
		case "/assets/index~ab12.js":
			_, _ = io.WriteString(w, `{"../locales/de-de/translations.json":()=>i(()=>import("./translations~cd34.js"),[]),"../locales/fr-ca/translations.json":()=>i(()=>import("./translations~ef56.js"),[])}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cat := applestrings.Catalogue{Mappings: []applestrings.Mapping{{Message: "Add", Key: "A"}, {Message: "Play", Key: "B"}}, Messages: 4, Version: "v"}
	store, err := applestrings.New(applestrings.Options{Dir: t.TempDir(), HTTP: srv.Client(), Origin: srv.URL, Catalogue: &cat})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	events := make(languagesEmitter, 2)
	gate := NewGate(time.Hour, nil)
	e := &Env{Deps: Deps{AppleStrings: store, Gate: gate, Emit: events, Logger: slog.New(slog.DiscardHandler), Started: time.Now()}}
	s := newTranslationsService(e)
	defer e.close()

	if got := s.Languages(0.9); got.Mapped != 0.5 || got.Locales == nil || len(got.Locales) != 0 {
		t.Fatalf("Languages(0.9) = %+v, want the mapped share only", got)
	}
	if got := s.Languages(0.5); len(got.Locales) != 0 {
		t.Fatalf("before the first paint = %+v", got)
	}
	gate.Open()
	var got AppleLanguages
	select {
	case got = <-events:
	case <-time.After(5 * time.Second):
		t.Fatal("no translations:languages event")
	}
	want := []AppleLanguage{{Tag: "de-DE", Coverage: 0.5}, {Tag: "fr-CA", Coverage: 0.5}}
	if len(got.Locales) != 2 || got.Locales[0] != want[0] || got.Locales[1] != want[1] {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
	if again := s.Languages(0.5); len(again.Locales) != 2 {
		t.Fatalf("Languages after the table = %+v", again)
	}
	if none := (&TranslationsService{e: &Env{}}).Languages(0); none.Locales == nil || none.Mapped != 0 {
		t.Fatalf("without a store = %+v", none)
	}
}
