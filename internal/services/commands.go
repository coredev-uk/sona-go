package services

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

// commandArtSize is the artwork edge in command-line answers.
const commandArtSize = 600

// commandServer answers `sona <command>` (internal/cli) in the running
// Sona, through what the UI uses: the coordinator for playback, the
// playback service for the volume (saved in the settings like the
// slider's), the library service for favourites, catalogue search, the
// launch opener for links and private listening's Mode. Every change
// reaches the UI through the usual events.
type commandServer struct {
	e      *Env
	open   *opener
	play   *PlaybackService
	lib    *LibraryService
	search *SearchService
	feed   *cli.Feed

	mu     sync.Mutex
	closed bool
}

// newCommandServer builds the server and publishes the status on every
// coordinator or private listening change until Core.Close.
func newCommandServer(e *Env, open *opener, svcs []application.Service) *commandServer {
	s := &commandServer{
		e: e, open: open,
		play: instance[PlaybackService](svcs), lib: instance[LibraryService](svcs), search: instance[SearchService](svcs),
	}
	s.feed = cli.NewFeed(func() cli.Status { return s.status(e.Coord.Snapshot()) })
	e.Private.OnChange(func(privatemode.State) { s.feed.Publish(s.status(e.Coord.Snapshot())) })
	snaps, cancel := e.Coord.Subscribe()
	quit, done := make(chan struct{}), make(chan struct{})
	e.Go("command status", func() {
		defer close(done)
		defer cancel()
		for {
			select {
			case <-quit:
				return
			case snap, ok := <-snaps:
				if !ok {
					return
				}
				s.feed.Publish(s.status(snap))
			}
		}
	})
	e.OnClose(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(quit)
		<-done
		s.feed.Close()
	})
	return s
}

// instance finds the registered service of type T.
func instance[T any](svcs []application.Service) *T {
	for _, s := range svcs {
		if t, ok := s.Instance().(*T); ok {
			return t
		}
	}
	panic(fmt.Sprintf("services: no registered %T", new(T)))
}

// Watch implements cli.Server.
func (s *commandServer) Watch() (<-chan cli.Status, func()) { return s.feed.Watch() }

// Call implements cli.Server.
func (s *commandServer) Call(ctx context.Context, req cli.Request) (any, error) {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, &cli.Error{Code: cli.CodeShuttingDown}
	}
	c := s.e.Coord
	switch req.Command {
	case "play":
		return s.transport(ctx, "resume", c.Resume)
	case "pause":
		return s.transport(ctx, "pause", c.Pause)
	case "toggle":
		return s.transport(ctx, "toggle", c.Toggle)
	case "next":
		return s.transport(ctx, "next", c.Next)
	case "prev":
		return s.transport(ctx, "previous", c.Previous)
	case "status":
		return s.status(c.Snapshot()), nil
	case "queue":
		return s.queue(c.Snapshot()), nil
	case "search":
		return s.searchCatalog(ctx, req)
	case "open":
		return s.openLink(req.Args)
	case "volume":
		return s.volume(ctx, req.Args)
	case "like":
		return s.like(ctx, req.Args)
	case "private":
		return s.private(req.Args)
	}
	return nil, &cli.Error{Code: cli.CodeUnknownCommand, Message: req.Command}
}

// fail is err as the command's error, with Sona's error code.
func (s *commandServer) fail(err error) error {
	s.e.Report(err)
	code := ErrorCode(err)
	if code == CodeUnknown {
		code = cli.CodeFailed
	}
	return &cli.Error{Code: code, Message: err.Error()}
}

// transport runs a playback command and answers with the status after it.
func (s *commandServer) transport(ctx context.Context, name string, run func(context.Context) error) (any, error) {
	s.e.Logger.Info("playback: command", "cmd", name, "from", "cli")
	if err := run(ctx); err != nil {
		return nil, s.fail(err)
	}
	return s.status(s.e.Coord.Snapshot()), nil
}

// status maps the coordinator's state for the command line.
func (s *commandServer) status(snap coordinator.Snapshot) cli.Status {
	st := cli.Status{
		Running:  true,
		State:    cmp.Or(string(snap.State), cli.StateIdle),
		Position: position(snap, time.Now()).Seconds(),
		Volume:   percent(snap.Volume),
		Shuffle:  snap.Shuffle,
		Repeat:   cmp.Or(string(snap.Repeat), string(coordinator.RepeatOff)),
		Private:  s.e.Private.On(),
		Preview:  snap.Preview,
		Error:    snap.Error,
	}
	if snap.Current != nil {
		t := track(snap.Current.Item)
		if snap.Duration > 0 {
			t.Duration = snap.Duration.Seconds()
		}
		st.Track = &t
	}
	return st
}

func percent(v float64) int { return int(math.Round(min(max(v, 0), 1) * 100)) }

func track(it coordinator.Item) cli.Track {
	return cli.Track{
		ID: it.ID, Title: it.Title, Artist: it.Artist, Album: it.Album,
		Duration: it.Duration.Seconds(), Explicit: it.Explicit,
		Artwork: publicArtURL(it.Artwork, commandArtSize),
	}
}

func (s *commandServer) queue(snap coordinator.Snapshot) cli.Queue {
	q := cli.Queue{Index: snap.Index, Tracks: make([]cli.QueueTrack, len(snap.Queue))}
	if snap.Current == nil {
		q.Index = -1
	}
	for i, e := range snap.Queue {
		q.Tracks[i] = cli.QueueTrack{Track: track(e.Item), AutoPlay: e.Auto}
	}
	return q
}

// searchCatalog runs the catalogue search the Search page runs and lists
// up to req.Limit results of each kind (the top results, which repeat
// the others, left out), kinds in the order Apple ranks them first.
func (s *commandServer) searchCatalog(ctx context.Context, req cli.Request) (any, error) {
	term := strings.Join(req.Args, " ")
	res, err := s.search.Catalog(ctx, term)
	if err != nil {
		return nil, s.fail(err)
	}
	limit := cli.SearchLimit(req.Limit)
	var (
		order  []string
		byKind = map[string][]cli.SearchResult{}
		seen   = map[string]bool{}
	)
	for _, sec := range res.Sections {
		if sec.Group == applemusic.GroupTop {
			continue
		}
		for _, it := range sec.Items {
			key := it.Kind + "/" + it.ID
			if (req.Kind != "" && it.Kind != req.Kind) || seen[key] || len(byKind[it.Kind]) >= limit {
				continue
			}
			seen[key] = true
			if _, ok := byKind[it.Kind]; !ok {
				order = append(order, it.Kind)
			}
			byKind[it.Kind] = append(byKind[it.Kind], cli.SearchResult{
				Kind: it.Kind, ID: it.ID, Name: it.Name, Subtitle: it.Subtitle, Explicit: it.Explicit, URL: it.URL,
			})
		}
	}
	out := cli.SearchResults{Term: res.Term, Results: []cli.SearchResult{}}
	for _, k := range order {
		out.Results = append(out.Results, byKind[k]...)
	}
	return out, nil
}

// openLink opens an Apple Music link as a forwarded launch would: the
// window comes to the front and the frontend opens the link's page.
func (s *commandServer) openLink(args []string) (any, error) {
	t, err := app.ParseURL(args[0])
	if err != nil {
		return nil, &cli.Error{Code: cli.CodeBadLink, Message: err.Error()}
	}
	s.open.activate(app.Activation{Args: args}, true)
	return cli.Opened{Kind: string(t.Kind), ID: t.ID, Storefront: t.Storefront}, nil
}

// volume shows the volume, or sets it as the player bar's slider does.
func (s *commandServer) volume(ctx context.Context, args []string) (any, error) {
	cur := percent(s.e.Coord.Snapshot().Volume)
	if len(args) == 0 {
		return cli.Volume{Volume: cur}, nil
	}
	n, relative, err := cli.ParseVolume(args[0])
	if err != nil {
		return nil, err
	}
	if relative {
		n += cur
	}
	n = min(max(n, 0), 100)
	if !s.e.Engine.Capabilities().Volume {
		return nil, s.fail(unsupported("volume"))
	}
	if err := s.play.SetVolume(ctx, float64(n)/100); err != nil {
		return nil, s.fail(err)
	}
	return cli.Volume{Volume: n}, nil
}

// like favourites the playing song (on), un-favourites it (off), switches
// it (toggle) or reports it (status), through the library service the
// player bar's Favourite uses. The frontend hears of changes
// (EventItemState).
func (s *commandServer) like(ctx context.Context, args []string) (any, error) {
	verb := "on"
	if len(args) > 0 {
		verb = args[0]
	}
	cur := s.e.Coord.Snapshot().Current
	if cur == nil {
		return nil, &cli.Error{Code: cli.CodeNothingPlaying}
	}
	qi := s.play.item(cur.UID, cur.Item)
	ref := ItemRef{Kind: applemusic.TypeSongs, ID: qi.ID, Source: qi.Source, CatalogID: qi.CatalogID, LibraryID: qi.LibraryID}
	liked := func() (bool, error) {
		st, err := s.lib.States(ctx, []ItemRef{ref})
		if err != nil {
			return false, err
		}
		return st[0].Rating == applemusic.RatingFavorite, nil
	}
	var on bool
	switch verb {
	case "on", "off":
		on = verb == "on"
	case "toggle":
		was, err := liked()
		if err != nil {
			return nil, s.fail(err)
		}
		on = !was
	case "status":
		is, err := liked()
		if err != nil {
			return nil, s.fail(err)
		}
		return cli.Like{Track: track(cur.Item), Liked: is}, nil
	default:
		return nil, &cli.Error{Code: cli.CodeUsage}
	}
	st, err := s.lib.Favorite(ctx, ref, on)
	if err != nil {
		return nil, s.fail(err)
	}
	if s.e.Emit != nil {
		s.e.Emit.Emit(EventItemState, st)
	}
	return cli.Like{Track: track(cur.Item), Liked: st.Rating == applemusic.RatingFavorite}, nil
}

// private shows or switches private listening (the tray's and the
// player bar's switch).
func (s *commandServer) private(args []string) (any, error) {
	verb := "status"
	if len(args) > 0 {
		verb = args[0]
	}
	var err error
	switch verb {
	case "on", "off":
		_, err = s.e.Private.Set(verb == "on")
	case "toggle":
		_, err = s.e.Private.Toggle()
	case "status":
	default:
		return nil, &cli.Error{Code: cli.CodeUsage}
	}
	if err != nil {
		return nil, s.fail(err)
	}
	st := s.e.Private.State()
	return cli.Private{On: st.On, Source: string(st.Source), Saved: s.e.Private.Saved()}, nil
}
