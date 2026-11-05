package services

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
)

// chooser is the desktop's file dialog answering with a fixed path.
type chooser struct {
	path string
	err  error
	asks []app.OpenFileOptions
}

func (c *chooser) OpenFile(_ context.Context, o app.OpenFileOptions) (string, error) {
	c.asks = append(c.asks, o)
	return c.path, c.err
}

func (c *chooser) SaveFile(context.Context, app.SaveFileOptions) (string, error) { return "", nil }

type coverRig struct {
	pl     *PlaylistsService
	ls     *LibraryService
	e      *Env
	rec    *recorder
	lib    *library.Library
	d      *db.DB
	covers string
}

func albumArt(n string) string {
	return "https://is1-ssl.mzstatic.com/image/thumb/Music/" + n + "/{w}x{h}bb.jpg"
}

// newCoverRig is the playlists service over a real database and artwork
// cache, with files as the platform's file chooser (nil: none).
func newCoverRig(t *testing.T, files FileChooser) *coverRig {
	t.Helper()
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	covers := filepath.Join(t.TempDir(), "covers")
	art, err := artwork.New(artwork.Options{Dir: t.TempDir(), CoverDir: covers})
	if err != nil {
		t.Fatal(err)
	}
	lib := library.New(d, nil)
	rec := &recorder{}
	e := &Env{Deps: Deps{
		DB: d, Library: lib, Art: art, Emit: rec, Storefront: func() string { return "gb" },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now, Platform: Platform{Files: files},
	}, m: mapper{art: art}, members: newMembership(d)}
	r := &coverRig{
		pl:  &PlaylistsService{e: e, m: e.m, locks: map[string]*sync.Mutex{}},
		ls:  &LibraryService{o: e, m: e.m},
		e:   e,
		rec: rec, lib: lib, d: d, covers: covers,
	}
	r.store("p.mix", "", "a", "b", "c", "d", "e")
	r.store("p.other", "", "a", "b", "c", "d")
	r.store("p.apple", albumArt("own"), "a", "b")
	return r
}

func (r *coverRig) store(id, own string, albums ...string) {
	songs := make([]applemusic.Song, len(albums))
	for i, a := range albums {
		songs[i] = applemusic.Song{Resource: applemusic.Resource[applemusic.SongAttributes]{
			ID: fmt.Sprintf("i.%s.%d", id, i), Type: applemusic.TypeLibrarySongs,
			Attributes: applemusic.SongAttributes{Name: "S", Artwork: &applemusic.Artwork{URL: albumArt(a)}},
		}}
	}
	p := applemusic.Playlist{Resource: applemusic.Resource[applemusic.PlaylistAttributes]{
		ID: id, Type: applemusic.TypeLibraryPlaylists,
		Attributes: applemusic.PlaylistAttributes{Name: id, CanEdit: true},
	}}
	if own != "" {
		p.Attributes.Artwork = &applemusic.Artwork{URL: own}
	}
	if err := r.lib.StorePlaylist(context.Background(), p, "", songs); err != nil {
		panic(err)
	}
}

// shown is every surface's cover of playlist id (large URLs): the tree, the
// library list, the page, search and the pins.
func (r *coverRig) shown(t *testing.T, id string) map[string]string {
	t.Helper()
	ctx := context.Background()
	out := map[string]string{}
	tree, err := r.pl.Tree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range tree.Playlists {
		if p.Playlist.ID == id {
			out["tree"] = p.Playlist.Art.Large
		}
	}
	page, err := r.ls.Playlists(ctx, "title", false, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range page.Items {
		if p.ID == id {
			out["list"] = p.Art.Large
		}
	}
	d, err := r.pl.Playlist(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	out["page"] = d.Playlist.Art.Large
	out["backdrop"] = d.Header.Backdrop
	if err := library.RebuildSearch(ctx, r.d); err != nil {
		t.Fatal(err)
	}
	found, err := r.e.Library.SearchAll(ctx, id, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range found.Playlists {
		if p.LibraryID == id {
			out["search"] = r.e.m.libPlaylist(p).Art.Large
		}
	}
	pins, err := r.pl.Pins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pins {
		if p.ID == id {
			out["pin"] = p.Art.Large
		}
	}
	return out
}

func (r *coverRig) pin(t *testing.T, id string) {
	t.Helper()
	if err := r.d.Q.InsertPin(context.Background(), dbq.InsertPinParams{
		Kind: kindPlaylist, AppleID: id, Name: id, ArtworkTemplate: "", Position: 0, PinnedAt: 1, Action: applemusic.PinPlay,
	}); err != nil {
		t.Fatal(err)
	}
}

// everywhere fails unless every surface shows the cover tmpl (a template;
// "" for the placeholder), which has a backdrop of its own unless it is
// empty.
func (r *coverRig) everywhere(t *testing.T, id, tmpl string) {
	t.Helper()
	want := ""
	if tmpl != "" {
		want = r.e.Art.URL(tmpl, artLarge, artLarge)
	}
	got := r.shown(t, id)
	for _, surface := range []string{"tree", "list", "page", "search", "pin"} {
		if g, ok := got[surface]; !ok || g != want {
			t.Errorf("%s of %s shows %q (present %v), want %q", surface, id, g, ok, want)
		}
	}
	if (tmpl == "") != (got["backdrop"] == "") {
		t.Errorf("backdrop of %s = %q for cover %q", id, got["backdrop"], tmpl)
	}
	if tmpl != "" && got["backdrop"] != r.e.Art.BackdropURL(tmpl) {
		t.Errorf("backdrop of %s = %q, want the cover's own %q", id, got["backdrop"], r.e.Art.BackdropURL(tmpl))
	}
}

// writePNG writes a w×h picture to a file and returns its path.
func writePNG(t *testing.T, w, h int, c color.RGBA) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Rect, image.NewUniform(c), image.Point{}, draw.Src)
	path := filepath.Join(t.TempDir(), "cover.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func optionIDs(c PlaylistCovers) []string {
	ids := make([]string, len(c.Options))
	for i, o := range c.Options {
		ids[i] = o.ID
	}
	return ids
}

func TestCoverOptionsListTheAutomaticCoverNoneAndSixPresets(t *testing.T) {
	r := newCoverRig(t, nil)
	got, err := r.pl.CoverOptions(context.Background(), "p.mix")
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != CoverAuto || got.CanUpload {
		t.Errorf("Current = %q, CanUpload = %v; want the automatic cover and no upload without a file chooser", got.Current, got.CanUpload)
	}
	if got.Limits.MaxBytes != artwork.MaxCoverBytes || got.Limits.MinEdge != artwork.MinCoverEdge {
		t.Errorf("limits = %+v", got.Limits)
	}
	if len(got.Options) != 2+len(artwork.PresetStyles) || got.Options[0].ID != CoverAuto || got.Options[1].ID != CoverNone {
		t.Fatalf("options = %v", optionIDs(got))
	}
	mosaic := artwork.Mosaic([]string{albumArt("a"), albumArt("b"), albumArt("c"), albumArt("d")})
	if want := r.e.m.artOf(mosaic); got.Options[0].Art != want || got.Options[0].Art.Large == "" {
		t.Errorf("the automatic option shows %+v, want the mosaic %+v", got.Options[0].Art, want)
	}
	if got.Options[1].Art != (Art{}) {
		t.Errorf("none shows %+v", got.Options[1].Art)
	}
	for i, o := range got.Options[2:] {
		if o.Kind != "preset" || o.Style != artwork.PresetStyles[i] || !artwork.IsPreset(o.ID) || o.Art.Medium == "" {
			t.Errorf("preset option %d = %+v", i, o)
		}
	}
	// A playlist Apple gave artwork offers that as its automatic cover.
	apple, _ := r.pl.CoverOptions(context.Background(), "p.apple")
	if apple.Options[0].Art != r.e.m.artOf(albumArt("own")) {
		t.Errorf("Apple's cover is not the automatic option: %+v", apple.Options[0].Art)
	}
	for _, bad := range []string{"pl.catalog", "", "i.1"} {
		if _, err := r.pl.CoverOptions(context.Background(), bad); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("CoverOptions(%q) = %v, want invalid argument", bad, err)
		}
	}
	if _, err := r.pl.CoverOptions(context.Background(), "p.nobody"); CodeOf(err) != CodeNotFound {
		t.Errorf("an unknown playlist: %v", err)
	}
}

func TestChoosingAPresetNoneAndAutomaticShowsEverywhere(t *testing.T) {
	r := newCoverRig(t, nil)
	ctx := context.Background()
	r.pin(t, "p.mix")
	mosaic := artwork.Mosaic([]string{albumArt("a"), albumArt("b"), albumArt("c"), albumArt("d")})
	// The pin Apple gave no artwork has none of its own: the choice is what shows.
	opts, _ := r.pl.CoverOptions(ctx, "p.mix")
	preset := opts.Options[4].ID

	for _, step := range []struct {
		name, option, want string
	}{
		{"a preset", preset, preset},
		{"none", CoverNone, ""},
		{"automatic", CoverAuto, mosaic},
	} {
		got, err := r.pl.SetCover(ctx, "p.mix", step.option)
		if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if got.Current != step.option {
			t.Errorf("%s: Current = %q", step.name, got.Current)
		}
		if step.name != "automatic" {
			r.everywhere(t, "p.mix", step.want)
		} else {
			// The pin keeps what Apple gave it, which is nothing.
			shown := r.shown(t, "p.mix")
			want := r.e.Art.URL(mosaic, artLarge, artLarge)
			for _, s := range []string{"tree", "list", "page", "search"} {
				if shown[s] != want {
					t.Errorf("automatic: %s shows %q, want %q", s, shown[s], want)
				}
			}
		}
		// Another playlist is never touched.
		if g := r.shown(t, "p.other"); g["tree"] != r.e.Art.URL(mosaic, artLarge, artLarge) {
			t.Errorf("%s changed p.other: %q", step.name, g["tree"])
		}
	}

	// Each choice told the surfaces that show the playlist.
	var changes []PlaylistsChanged
	r.rec.mu.Lock()
	for i, ev := range r.rec.events {
		if ev == EventPlaylistsChanged {
			changes = append(changes, r.rec.data[i].(PlaylistsChanged))
		}
	}
	r.rec.mu.Unlock()
	if len(changes) != 3 || !changes[0].Tree || !changes[0].Pins || len(changes[0].IDs) != 1 || changes[0].IDs[0] != "p.mix" {
		t.Errorf("playlists:changed = %+v", changes)
	}
}

func TestACoverOverApplesArtworkAndThePresetStaysOffered(t *testing.T) {
	r := newCoverRig(t, nil)
	ctx := context.Background()
	got, err := r.pl.SetCover(ctx, "p.apple", CoverNone)
	if err != nil || got.Current != CoverNone {
		t.Fatalf("SetCover = %+v, %v", got, err)
	}
	if shown := r.shown(t, "p.apple"); shown["tree"] != "" || shown["page"] != "" {
		t.Errorf("none left Apple's artwork showing: %v", shown)
	}
	// A preset drawn from a palette the playlist no longer has is still
	// offered, as the current choice.
	old := "preset:glow:112233:ddeeff"
	if _, err := r.pl.SetCover(ctx, "p.apple", old); err != nil {
		t.Fatal(err)
	}
	got, _ = r.pl.CoverOptions(ctx, "p.apple")
	if got.Current != old || got.Options[2].ID != old {
		t.Errorf("Current = %q, first preset = %q; want the chosen one offered first", got.Current, got.Options[2].ID)
	}
}

func TestSetCoverRefusesWhatIsNotAnOption(t *testing.T) {
	r := newCoverRig(t, nil)
	for _, bad := range []string{
		"", "rainbow", CoverUpload, // no image was chosen yet
		"preset:nope:112233:445566", "preset:glow:112233", "https://is1-ssl.mzstatic.com/x/{w}x{h}bb.jpg",
		"mosaic:" + albumArt("a"), "cover:0123456789abcdef0123456789abcdef", "https://evil.example/{w}x{h}bb.jpg",
	} {
		if _, err := r.pl.SetCover(context.Background(), "p.mix", bad); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("SetCover(%q) = %v, want invalid argument", bad, err)
		}
	}
	if _, err := r.pl.SetCover(context.Background(), "pl.catalog", CoverNone); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("a catalog playlist: %v", err)
	}
	if _, err := r.pl.SetCover(context.Background(), "p.nobody", CoverNone); CodeOf(err) != CodeNotFound {
		t.Errorf("an unknown playlist: %v", err)
	}
	if c, _ := r.lib.PlaylistCover(context.Background(), "p.mix"); c.Kind != library.CoverAuto {
		t.Errorf("a refused choice was stored: %+v", c)
	}
}

func TestUploadingACoverShowsEverywhereAndIsKeptOnThisComputer(t *testing.T) {
	ctx := context.Background()
	files := &chooser{}
	r := newCoverRig(t, files)
	r.pin(t, "p.mix")
	files.path = writePNG(t, 640, 480, color.RGBA{200, 40, 40, 255})

	got, err := r.pl.UploadCover(ctx, "p.mix")
	if err != nil {
		t.Fatal(err)
	}
	if !got.CanUpload || got.Current != CoverUpload || got.Options[2].ID != CoverUpload || got.Options[2].Art.Large == "" {
		t.Fatalf("after the upload: current %q, options %v", got.Current, optionIDs(got))
	}
	if len(files.asks) != 1 || files.asks[0].Title == "" || files.asks[0].Filters[0].Name == "" || !strings.Contains(strings.Join(files.asks[0].Filters[0].Patterns, " "), "*.png") {
		t.Errorf("the file chooser was asked %+v", files.asks)
	}
	choice, err := r.lib.PlaylistCover(ctx, "p.mix")
	if err != nil || choice.Kind != library.CoverUpload || !artwork.IsCover(choice.Template) || choice.Upload != choice.Template {
		t.Fatalf("stored choice = %+v, %v", choice, err)
	}
	r.everywhere(t, "p.mix", choice.Template)
	stored, err := os.ReadDir(r.covers)
	if err != nil || len(stored) != 1 {
		t.Fatalf("covers directory = %v, %v", stored, err)
	}
	// It is the user's picture, squared: the artwork cache serves it.
	b, err := r.e.Art.Get(ctx, choice.Template, 200, 200)
	if err != nil || len(b) == 0 {
		t.Fatalf("serving the cover: %d bytes, %v", len(b), err)
	}

	// Another cover shows; the image is kept to go back to.
	opts, _ := r.pl.CoverOptions(ctx, "p.mix")
	if got, err = r.pl.SetCover(ctx, "p.mix", opts.Options[3].ID); err != nil || got.Current != opts.Options[3].ID {
		t.Fatalf("SetCover(preset) = %+v, %v", got, err)
	}
	if got.Options[2].ID != CoverUpload {
		t.Errorf("the chosen image is no longer offered: %v", optionIDs(got))
	}
	if _, err := os.Stat(coverPath(r.covers, choice.Template)); err != nil {
		t.Errorf("the image went with the choice of another cover: %v", err)
	}
	if got, err = r.pl.SetCover(ctx, "p.mix", CoverUpload); err != nil || got.Current != CoverUpload {
		t.Fatalf("back to the image: %+v, %v", got, err)
	}
	r.everywhere(t, "p.mix", choice.Template)

	// A new image replaces it and the old file goes.
	files.path = writePNG(t, 500, 500, color.RGBA{40, 40, 200, 255})
	if _, err := r.pl.UploadCover(ctx, "p.mix"); err != nil {
		t.Fatal(err)
	}
	next, _ := r.lib.PlaylistCover(ctx, "p.mix")
	if next.Template == choice.Template || next.Upload != next.Template {
		t.Fatalf("second choice = %+v", next)
	}
	if stored, _ = os.ReadDir(r.covers); len(stored) != 1 {
		t.Errorf("covers directory holds %d files after a replacement, want 1", len(stored))
	}
	r.everywhere(t, "p.mix", next.Template)

	// Removing it goes back to the automatic cover and deletes the file.
	got, err = r.pl.RemoveCoverUpload(ctx, "p.mix")
	if err != nil || got.Current != CoverAuto {
		t.Fatalf("RemoveCoverUpload = %+v, %v", got, err)
	}
	for _, o := range got.Options {
		if o.ID == CoverUpload {
			t.Error("the removed image is still offered")
		}
	}
	if stored, _ = os.ReadDir(r.covers); len(stored) != 0 {
		t.Errorf("covers directory holds %d files after removal", len(stored))
	}
}

func coverPath(dir, tmpl string) string {
	return filepath.Join(dir, strings.TrimPrefix(tmpl, "cover:")+".jpg")
}

func TestRemovingTheImageOfAnotherCoverLeavesTheChoice(t *testing.T) {
	ctx := context.Background()
	files := &chooser{}
	r := newCoverRig(t, files)
	files.path = writePNG(t, 400, 400, color.RGBA{9, 9, 9, 255})
	if _, err := r.pl.UploadCover(ctx, "p.mix"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.pl.SetCover(ctx, "p.mix", CoverNone); err != nil {
		t.Fatal(err)
	}
	got, err := r.pl.RemoveCoverUpload(ctx, "p.mix")
	if err != nil || got.Current != CoverNone {
		t.Fatalf("RemoveCoverUpload = %+v, %v; want none to stay", got, err)
	}
	if stored, _ := os.ReadDir(r.covers); len(stored) != 0 {
		t.Errorf("%d files left", len(stored))
	}
}

func TestUploadCoverCancelledRefusedAndUnsupported(t *testing.T) {
	ctx := context.Background()
	files := &chooser{}
	r := newCoverRig(t, files)

	// Cancel: nothing changes.
	got, err := r.pl.UploadCover(ctx, "p.mix")
	if err != nil || got.Current != CoverAuto {
		t.Fatalf("a cancelled chooser: %+v, %v", got, err)
	}

	// An image that cannot be a cover changes nothing and says why.
	for name, path := range map[string]string{
		"too small": writePNG(t, 100, 100, color.RGBA{1, 2, 3, 255}),
		"not an image": func() string {
			p := filepath.Join(t.TempDir(), "x.png")
			_ = os.WriteFile(p, []byte("hello"), 0o600)
			return p
		}(),
	} {
		files.path = path
		if _, err := r.pl.UploadCover(ctx, "p.mix"); CodeOf(err) != CodeImageUnusable {
			t.Errorf("%s: %v, want image_unusable", name, err)
		}
	}
	files.path = filepath.Join(t.TempDir(), "missing.png")
	if _, err := r.pl.UploadCover(ctx, "p.mix"); err == nil {
		t.Error("a file that is gone was accepted")
	}
	files.err = errors.New("portal went away")
	files.path = ""
	if _, err := r.pl.UploadCover(ctx, "p.mix"); err == nil {
		t.Error("a chooser failure was swallowed")
	}
	if c, _ := r.lib.PlaylistCover(ctx, "p.mix"); c.Kind != library.CoverAuto || c.Upload != "" {
		t.Errorf("a refused upload stored %+v", c)
	}
	if stored, _ := os.ReadDir(r.covers); len(stored) != 0 {
		t.Errorf("a refused upload left %d files", len(stored))
	}

	// No file chooser on this platform: unsupported, and the picker says so.
	none := newCoverRig(t, nil)
	if _, err := none.pl.UploadCover(ctx, "p.mix"); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("without a file chooser: %v", err)
	}
	if _, err := r.pl.UploadCover(ctx, "pl.catalog"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("a catalog playlist: %v", err)
	}
}

func TestACoverGoesWithItsPlaylistAndSignOut(t *testing.T) {
	ctx := context.Background()
	files := &chooser{path: writePNG(t, 400, 400, color.RGBA{9, 9, 9, 255})}
	r := newCoverRig(t, files)
	for _, id := range []string{"p.mix", "p.other"} {
		if _, err := r.pl.UploadCover(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	// Both chose the same picture: one file, kept while either shows it.
	if stored, _ := os.ReadDir(r.covers); len(stored) != 1 {
		t.Fatalf("%d files for one picture", len(stored))
	}
	if err := r.lib.DropPlaylist(ctx, "p.mix"); err != nil {
		t.Fatal(err)
	}
	pruneCoverFiles(ctx, r.e)
	if stored, _ := os.ReadDir(r.covers); len(stored) != 1 {
		t.Fatalf("the picture p.other still shows was removed: %d files", len(stored))
	}
	if err := r.lib.DropPlaylist(ctx, "p.other"); err != nil {
		t.Fatal(err)
	}
	pruneCoverFiles(ctx, r.e)
	if stored, _ := os.ReadDir(r.covers); len(stored) != 0 {
		t.Fatalf("%d files left for no playlist", len(stored))
	}

	// Sign-out removes every chosen image, whatever the database says.
	r.store("p.again", "", "a")
	if _, err := r.pl.UploadCover(ctx, "p.again"); err != nil {
		t.Fatal(err)
	}
	if err := wipeUserArtwork(ctx, r.d, r.e.Art, r.e.Logger); err != nil {
		t.Fatal(err)
	}
	if err := r.d.WipeUserData(ctx); err != nil {
		t.Fatal(err)
	}
	if stored, _ := os.ReadDir(r.covers); len(stored) != 0 {
		t.Errorf("sign-out left %d chosen images", len(stored))
	}
	if uploads, err := r.lib.CoverUploads(ctx); err != nil || len(uploads) != 0 {
		t.Errorf("sign-out left choices %v, %v", uploads, err)
	}
}

// chooserOf answers each ask with the next path of a list, as concurrent
// uploads each get their own image.
type chooserOf struct {
	mu    sync.Mutex
	paths []string
}

func (c *chooserOf) OpenFile(context.Context, app.OpenFileOptions) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.paths[0]
	c.paths = c.paths[1:]
	return p, nil
}

func (c *chooserOf) SaveFile(context.Context, app.SaveFileOptions) (string, error) { return "", nil }

// Two playlists choosing images at once, and a library sync tidying the
// covers directory meanwhile, never take each other's images: an image is
// not on disk without a record for longer than the upload itself.
func TestConcurrentUploadsAndTidyingKeepEveryChosenImage(t *testing.T) {
	ctx := context.Background()
	files := &chooserOf{}
	r := newCoverRig(t, files)
	// A playlist has to exist for each upload.
	const rounds = 12
	for round := range rounds {
		a := writePNG(t, 400, 400, color.RGBA{uint8(10 + round), 40, 90, 255})
		b := writePNG(t, 400, 400, color.RGBA{uint8(10 + round), 140, 20, 255})
		files.paths = []string{a, b}
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, id := range []string{"p.mix", "p.other"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = r.pl.UploadCover(ctx, id)
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				pruneCoverFiles(ctx, r.e)
			}
		}()
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, id := range []string{"p.mix", "p.other"} {
			c, err := r.lib.PlaylistCover(ctx, id)
			if err != nil || c.Kind != library.CoverUpload {
				t.Fatalf("round %d: %s has %+v, %v", round, id, c, err)
			}
			if _, err := os.Stat(coverPath(r.covers, c.Template)); err != nil {
				t.Fatalf("round %d: the image of %s is gone: %v", round, id, err)
			}
		}
	}
	// Only the last round's two images are kept: replaced ones went.
	if stored, _ := os.ReadDir(r.covers); len(stored) != 2 {
		t.Errorf("%d images kept for two playlists", len(stored))
	}
}
