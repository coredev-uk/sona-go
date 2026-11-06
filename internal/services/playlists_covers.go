package services

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
	"github.com/coredev-uk/sona-go/internal/library"
)

// The file chooser's text for a playlist cover, in the user's language
// (Deps.Language).
const (
	msgCoverTitle  l10n.Message = "Choose a cover"       // the file chooser's title
	msgCoverAccept l10n.Message = "Choose"               // the file chooser's open button
	msgCoverFilter l10n.Message = "Images (JPEG or PNG)" // the file chooser's file type
)

// coverPatterns are the file chooser's globs. The portal's and GTK's are
// case-sensitive, so each extension is also given with brackets ("IMG.JPG").
var coverPatterns = []string{
	"*.jpg", "*.jpeg", "*.png",
	"*.[jJ][pP][gG]", "*.[jJ][pP][eE][gG]", "*.[pP][nN][gG]",
}

// Playlist covers are Sona's own. Apple's web API cannot set a playlist's
// artwork (docs/apple-music-reference.md, "Playlist artwork"), so the
// choice lives in SQLite (library.CoverChoice) and an uploaded image in
// the data directory (artwork.SaveCover); nothing is sent to Apple.

// CoverOptions returns the cover picker of library playlist id: the
// automatic cover, none, the image the user chose before (when there is
// one) and Apple's designs of the cover it generated for the playlist
// (artwork.Patterns; when it generated one: a playlist with songs and no
// photo of its own).
func (s *PlaylistsService) CoverOptions(ctx context.Context, id string) (PlaylistCovers, error) {
	if err := requireLibraryPlaylist(id); err != nil {
		return PlaylistCovers{}, err
	}
	choice, err := s.e.Library.PlaylistCover(ctx, id)
	if err != nil {
		return PlaylistCovers{}, err
	}
	auto, err := s.e.Library.AutoCover(ctx, id)
	if err != nil {
		return PlaylistCovers{}, err
	}
	out := PlaylistCovers{
		Current:   CoverAuto,
		CanUpload: s.e.Platform.Files != nil && s.e.Art != nil,
		Limits:    CoverLimits{MaxBytes: artwork.MaxCoverBytes, MinEdge: artwork.MinCoverEdge, MaxMpx: artwork.MaxCoverPixels / 1_000_000},
		Options: []CoverOption{
			{ID: CoverAuto, Kind: "auto", Art: s.m.artOf(auto)},
			{ID: CoverNone, Kind: "none"},
		},
	}
	if choice.Upload != "" {
		out.Options = append(out.Options, CoverOption{ID: CoverUpload, Kind: "upload", Art: s.m.artOf(choice.Upload)})
	}
	for _, p := range artwork.Patterns {
		if t, ok := artwork.WithPattern(auto, p); ok {
			out.Options = append(out.Options, CoverOption{ID: p, Kind: "pattern", Art: s.m.artOf(t)})
		}
	}
	switch choice.Kind {
	case library.CoverNone:
		out.Current = CoverNone
	case library.CoverUpload:
		out.Current = CoverUpload
	case library.CoverPreset:
		// A design shows only while Apple generates the playlist's cover;
		// otherwise the automatic cover is what shows (library.FillCovers).
		if _, ok := artwork.WithPattern(auto, choice.Template); ok {
			out.Current = choice.Template
		}
	}
	return out, nil
}

// SetCover shows option (an ID of CoverOptions) as library playlist id's
// cover on every surface and returns the picker as it is now.
func (s *PlaylistsService) SetCover(ctx context.Context, id, option string) (PlaylistCovers, error) {
	if err := requireLibraryPlaylist(id); err != nil {
		return PlaylistCovers{}, err
	}
	defer s.lock(id)()
	defer s.e.lockCovers()()
	choice, err := s.e.Library.PlaylistCover(ctx, id)
	if err != nil {
		return PlaylistCovers{}, err
	}
	next := library.CoverChoice{Kind: library.CoverAuto, Upload: choice.Upload}
	switch {
	case option == CoverAuto:
	case option == CoverNone:
		next.Kind = library.CoverNone
	case option == CoverUpload && choice.Upload != "":
		next.Kind, next.Template = library.CoverUpload, choice.Upload
	case slices.Contains(artwork.Patterns, option):
		auto, err := s.e.Library.AutoCover(ctx, id)
		if err != nil {
			return PlaylistCovers{}, err
		}
		if _, ok := artwork.WithPattern(auto, option); !ok {
			return PlaylistCovers{}, fmt.Errorf("%w: Apple generated no cover for playlist %s", ErrInvalidArgument, id)
		}
		next.Kind, next.Template = library.CoverPreset, option
	default:
		return PlaylistCovers{}, fmt.Errorf("%w: no cover %q", ErrInvalidArgument, option)
	}
	if err := s.storeCover(ctx, id, next); err != nil {
		return PlaylistCovers{}, err
	}
	return s.CoverOptions(ctx, id)
}

// UploadCover asks for an image (the desktop's file chooser, Platform.Files),
// makes the square cover of it (artwork.SaveCover: a JPEG or PNG, cropped
// from the centre) and shows it as library playlist id's cover. When the
// user cancels, nothing changes. The image is kept on this computer only.
func (s *PlaylistsService) UploadCover(ctx context.Context, id string) (PlaylistCovers, error) {
	if err := requireLibraryPlaylist(id); err != nil {
		return PlaylistCovers{}, err
	}
	if s.e.Platform.Files == nil || s.e.Art == nil {
		return PlaylistCovers{}, unsupported("the file chooser")
	}
	p := s.e.Language.Printer()
	path, err := s.e.Platform.Files.OpenFile(ctx, app.OpenFileOptions{
		Title:       p.T(msgCoverTitle),
		AcceptLabel: p.T(msgCoverAccept),
		Filters:     []app.FileFilter{{Name: p.T(msgCoverFilter), Patterns: coverPatterns}},
	})
	if err != nil {
		return PlaylistCovers{}, err
	}
	if path == "" {
		return s.CoverOptions(ctx, id)
	}
	f, err := os.Open(path)
	if err != nil {
		return PlaylistCovers{}, fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}
	defer func() { _ = f.Close() }()
	defer s.lock(id)()
	// From the stored file to the record that refers to it, no tidying runs.
	defer s.e.lockCovers()()
	tmpl, err := s.e.Art.SaveCover(f)
	if err != nil {
		return PlaylistCovers{}, err
	}
	if err := s.storeCover(ctx, id, library.CoverChoice{Kind: library.CoverUpload, Template: tmpl, Upload: tmpl}); err != nil {
		tidyCoverFiles(ctx, s.e) // the image just stored refers to nothing: it goes now
		return PlaylistCovers{}, err
	}
	s.e.Logger.Info("playlist cover chosen", "playlist", id)
	return s.CoverOptions(ctx, id)
}

// RemoveCoverUpload forgets the image the user chose for library playlist
// id and deletes it from this computer; a playlist that showed it goes back
// to its automatic cover.
func (s *PlaylistsService) RemoveCoverUpload(ctx context.Context, id string) (PlaylistCovers, error) {
	if err := requireLibraryPlaylist(id); err != nil {
		return PlaylistCovers{}, err
	}
	defer s.lock(id)()
	defer s.e.lockCovers()()
	choice, err := s.e.Library.PlaylistCover(ctx, id)
	if err != nil {
		return PlaylistCovers{}, err
	}
	next := library.CoverChoice{Kind: choice.Kind, Template: choice.Template}
	if choice.Kind == library.CoverUpload {
		next = library.CoverChoice{Kind: library.CoverAuto}
	}
	if err := s.storeCover(ctx, id, next); err != nil {
		return PlaylistCovers{}, err
	}
	return s.CoverOptions(ctx, id)
}

// lockCovers takes coverFiles; the caller defers what it returns.
func (e *Env) lockCovers() func() {
	e.coverFiles.Lock()
	return e.coverFiles.Unlock
}

// storeCover saves choice, deletes the image files nothing refers to now
// and tells every surface that shows the playlist (the tree, the lists, the
// pins and the page). The caller holds coverFiles.
func (s *PlaylistsService) storeCover(ctx context.Context, id string, choice library.CoverChoice) error {
	if err := s.e.Library.SetPlaylistCover(ctx, id, choice); err != nil {
		return err
	}
	tidyCoverFiles(ctx, s.e)
	s.changed(PlaylistsChanged{IDs: []string{id}, Tree: true, Pins: true})
	return nil
}

// pruneCoverFiles deletes the stored cover images no playlist refers to: a
// replaced or removed image, one whose playlist left the library.
func pruneCoverFiles(ctx context.Context, e *Env) {
	defer e.lockCovers()()
	tidyCoverFiles(ctx, e)
}

// tidyCoverFiles is pruneCoverFiles for a caller that holds coverFiles.
func tidyCoverFiles(ctx context.Context, e *Env) {
	if e.Art == nil {
		return
	}
	keep, err := e.Library.CoverUploads(ctx)
	if err == nil {
		_, err = e.Art.RemoveCovers(keep)
	}
	if err != nil {
		e.Logger.Warn("playlist covers: tidy", "err", err)
	}
}

// coverOfPin is the cover the user chose for a pinned playlist, "" when
// the pin keeps the artwork Apple gave it.
func (s *PlaylistsService) coverOfPin(ctx context.Context, libraryID string) (string, bool) {
	choice, err := s.e.Library.PlaylistCover(ctx, libraryID)
	if err != nil {
		return "", false
	}
	switch choice.Kind {
	case library.CoverNone:
		return "", true
	case library.CoverUpload:
		return choice.Template, true
	case library.CoverPreset:
		// Drawn from the library's cover, not the pin's: the pin's is
		// Apple's as of the last pin refresh, the name in it maybe an old one.
		if auto, err := s.e.Library.AutoCover(ctx, libraryID); err == nil {
			if t, ok := artwork.WithPattern(auto, choice.Template); ok {
				return t, true
			}
		}
	}
	return "", false
}
