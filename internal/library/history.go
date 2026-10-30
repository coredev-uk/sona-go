package library

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// Play is one listen recorded on this computer (playback_history).
type Play struct {
	PlayedAt  time.Time
	CatalogID string // the song's catalog ID ("" for uploads), through the library song when needed
	LibraryID string // "" when the song is not in the library
	Name      string
	Artist    string
	Album     string
	// ArtworkTemplate is Apple's artwork template.
	ArtworkTemplate string
	Duration        time.Duration
	Played          time.Duration
	Context         string // the queue label it was played from
}

// HistoryEntry is one play in the listening history: a Play from this
// computer, or (Play nil) a song Apple's recently played list reports from
// the account's other devices, whose time Apple does not give.
type HistoryEntry struct {
	Play  *Play
	Apple *applemusic.Song
}

// HistoryDay groups the entries of one local day, newest first. Day is
// local midnight; the zero Day holds the plays from other devices that are
// newer than every play here.
type HistoryDay struct {
	Day     time.Time
	Entries []HistoryEntry
}

// historyLimit bounds the plays History reads.
const historyLimit = 5000

// History returns the plays recorded here since since, newest first, at
// most historyLimit; more reports whether older plays exist. last maps
// every song key ever played here (catalog ID, library ID) to its last
// play, for MergeHistory.
func (l *Library) History(ctx context.Context, since time.Time) (plays []Play, last map[string]time.Time, more bool, err error) {
	rows, err := l.db.RQ.HistorySince(ctx, dbq.HistorySinceParams{PlayedAt: db.Millis(since), Limit: historyLimit})
	if err != nil {
		return nil, nil, false, fmt.Errorf("library: history: %w", err)
	}
	plays = make([]Play, len(rows))
	for i, r := range rows {
		plays[i] = Play{
			PlayedAt: db.Time(r.PlayedAt), CatalogID: r.SongCatalogID, LibraryID: r.LibraryID,
			Name: r.Name, Artist: r.ArtistName, Album: r.AlbumName, ArtworkTemplate: r.ArtworkTemplate,
			Duration: time.Duration(r.DurationMs) * time.Millisecond, Played: time.Duration(r.PlayedMs) * time.Millisecond,
			Context: r.Source,
		}
	}
	keys, err := l.db.RQ.HistoryLastPlayed(ctx)
	if err != nil {
		return nil, nil, false, fmt.Errorf("library: history: %w", err)
	}
	last = make(map[string]time.Time, len(keys))
	for _, k := range keys {
		t := db.Time(k.PlayedAt)
		for _, id := range []string{k.CatalogID, k.LibraryID} {
			if id != "" && t.After(last[id]) {
				last[id] = t
			}
		}
	}
	oldest := since
	if len(plays) == historyLimit {
		oldest = plays[len(plays)-1].PlayedAt
	}
	more, err = l.db.RQ.HistoryBefore(ctx, db.Millis(oldest))
	if err != nil {
		return nil, nil, false, fmt.Errorf("library: history: %w", err)
	}
	return plays, last, more, nil
}

// MergeHistory merges the plays recorded here (newest first, with last
// from History) with Apple's recently played songs (newest first; Apple
// lists each song once, at its latest play, from every device) and groups
// them by local day in loc. started are listens begun here that
// playback_history does not hold yet (the song playing or paused: a listen
// is recorded when it ends), with PlayedAt when they began.
//
// Every local play is kept. An Apple song also played here is that play
// (deduplicated). A started listen of a song Apple lists, begun after the
// song's last recorded play here, is played here too: Apple reports a
// listen as it starts, and that one is this computer's, not another
// device's (started listens Apple does not list yet are left out). An
// Apple song not played here was played on another device at a time Apple
// does not give: it is placed right after the nearest newer song in
// Apple's list that was played here, in that play's day, or in the leading
// undated group when no newer one was. Apple's list stops being merged at
// the first song whose last play here is older than since (everything
// after it is older too).
func MergeHistory(plays, started []Play, last map[string]time.Time, apple []applemusic.Song, since time.Time, loc *time.Location) []HistoryDay {
	plays, last = addStarted(plays, started, last, apple)
	// after[i] holds the Apple-only songs that follow local play i; lead
	// the ones newer than every play here.
	var lead []HistoryEntry
	after := make(map[int][]HistoryEntry)
	// first play index (newest) of each song key.
	index := make(map[string]int, len(plays))
	for i, p := range plays {
		for _, id := range []string{p.CatalogID, p.LibraryID} {
			if _, ok := index[id]; id != "" && !ok {
				index[id] = i
			}
		}
	}
	anchor := -1 // index of the newest local play the Apple walk has passed
	for i := range apple {
		s := &apple[i]
		keys := []string{s.ID, s.CatalogID()}
		if at, ok := playedHere(keys, last); ok {
			if at.Before(since) {
				break
			}
			if j, ok := firstIndex(keys, index); ok && j > anchor {
				anchor = j
			}
			continue
		}
		e := HistoryEntry{Apple: s}
		if anchor < 0 {
			lead = append(lead, e)
		} else {
			after[anchor] = append(after[anchor], e)
		}
	}

	var days []HistoryDay
	add := func(day time.Time, e HistoryEntry) {
		if n := len(days); n == 0 || !days[n-1].Day.Equal(day) {
			days = append(days, HistoryDay{Day: day})
		}
		days[len(days)-1].Entries = append(days[len(days)-1].Entries, e)
	}
	for _, e := range lead {
		add(time.Time{}, e)
	}
	for i := range plays {
		t := plays[i].PlayedAt.In(loc)
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		add(day, HistoryEntry{Play: &plays[i]})
		for _, e := range after[i] {
			add(day, e)
		}
	}
	return days
}

// addStarted adds to plays (newest first) and last the started listens of
// songs in apple that began after the song's last recorded play here.
func addStarted(plays, started []Play, last map[string]time.Time, apple []applemusic.Song) ([]Play, map[string]time.Time) {
	listed := make(map[string]bool, 2*len(apple))
	for i := range apple {
		for _, k := range []string{apple[i].ID, apple[i].CatalogID()} {
			listed[k] = k != ""
		}
	}
	var add []Play
	for _, p := range started {
		keys := []string{p.CatalogID, p.LibraryID}
		if !listed[p.CatalogID] && !listed[p.LibraryID] {
			continue
		}
		if at, ok := playedHere(keys, last); ok && !p.PlayedAt.After(at) {
			continue
		}
		if len(add) == 0 {
			last = maps.Clone(last)
			if last == nil {
				last = make(map[string]time.Time)
			}
		}
		for _, k := range keys {
			if k != "" {
				last[k] = p.PlayedAt
			}
		}
		add = append(add, p)
	}
	if len(add) == 0 {
		return plays, last
	}
	plays = slices.Concat(plays, add)
	slices.SortStableFunc(plays, func(a, b Play) int { return b.PlayedAt.Compare(a.PlayedAt) })
	return plays, last
}

func playedHere(keys []string, last map[string]time.Time) (time.Time, bool) {
	for _, k := range keys {
		if t, ok := last[k]; ok && k != "" {
			return t, true
		}
	}
	return time.Time{}, false
}

func firstIndex(keys []string, index map[string]int) (int, bool) {
	for _, k := range keys {
		if i, ok := index[k]; ok && k != "" {
			return i, true
		}
	}
	return 0, false
}
