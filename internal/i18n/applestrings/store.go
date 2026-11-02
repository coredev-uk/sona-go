package applestrings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/diag"
)

// RefreshInterval is how often, at most, the store asks Apple whether its
// bundles changed. Between checks, and offline, the cache serves.
const RefreshInterval = 24 * time.Hour

// maxWanted bounds the preferred tags a session fetches for.
const maxWanted = 8

// maxCacheBytes caps a cache file read back (a locale's file is about
// 10 kB).
const maxCacheBytes = 1 << 20

// cacheFormat is the cache files' layout version.
const cacheFormat = 1

// Options configure a Store.
type Options struct {
	// Dir is the cache directory (created as needed).
	Dir string
	// HTTP fetches Apple's assets; Sona's is held until the first paint.
	HTTP *http.Client
	// Origin replaces music.apple.com (tests).
	Origin string
	// Catalogue defaults to the one built into Sona (Embedded).
	Catalogue *Catalogue
	Logger    *slog.Logger
	Now       func() time.Time
}

// Locale is one locale's Apple-sourced messages.
type Locale struct {
	// Tag is Apple's locale in BCP 47 ("fr-FR"); "" when there is none.
	Tag string
	// Messages are Lingui (ICU) messages keyed by Sona's message ID.
	Messages map[string]string
	// Total is how many messages Sona has.
	Total int
	// Version is Apple's bundle (its content-hashed chunk name).
	Version   string
	FetchedAt time.Time
}

// Coverage is the share of Sona's messages the locale translates.
func (l Locale) Coverage() float64 {
	if l.Total == 0 {
		return 0
	}
	return float64(len(l.Messages)) / float64(l.Total)
}

// indexFile caches the web app's locale table.
type indexFile struct {
	Format    int               `json:"format"`
	Script    string            `json:"script"`
	Locales   map[string]string `json:"locales"` // Apple tag → bundle URL
	CheckedAt time.Time         `json:"checkedAt"`
}

// localeFile caches one locale's converted messages.
type localeFile struct {
	Format    int               `json:"format"`
	Locale    string            `json:"locale"`  // Apple's tag
	Bundle    string            `json:"bundle"`  // the bundle's URL
	Mapping   string            `json:"mapping"` // Catalogue.Version
	FetchedAt time.Time         `json:"fetchedAt"`
	Messages  map[string]string `json:"messages"`
}

// Store keeps Apple's translations of Sona's mapped messages for the
// locales the UI asks for, in its cache directory. Lookup serves the cache
// only; after Start (the network opening after the first paint) it
// downloads what the cache lacks and, at most once a RefreshInterval,
// checks for a new Apple release, telling subscribers what changed.
type Store struct {
	dir   string
	fetch *Fetcher
	log   *slog.Logger
	now   func() time.Time
	// catalogue is read on first use, off the cold start.
	catalogue func() (Catalogue, error)

	mu           sync.Mutex
	cat          Catalogue // set by catalogueLocked
	catErr       error
	catLoaded    bool
	index        *indexFile
	indexLoaded  bool
	locales      map[string]*localeFile // Apple tag → cache (nil: none)
	wanted       []string               // preferred tags, lower case
	attempted    map[string]bool        // Apple tags fetched this session
	indexChecked bool
	started      bool
	subs         map[int]func(Locale)
	nextSub      int

	kick   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New returns a store over o.Dir. Nothing is read until Lookup.
func New(o Options) (*Store, error) {
	if o.Dir == "" || o.HTTP == nil {
		return nil, errors.New("applestrings: Dir and HTTP are required")
	}
	catalogue := Embedded
	if o.Catalogue != nil {
		c := *o.Catalogue
		catalogue = func() (Catalogue, error) { return c, nil }
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	fetch := &Fetcher{Origin: o.Origin}
	// Redirects stay on Apple's origin (at most 5).
	hc := *o.HTTP
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme+"://"+req.URL.Host != fetch.origin() {
			return fmt.Errorf("redirect off %s: %s", fetch.origin(), req.URL.Host)
		}
		return nil
	}
	fetch.HTTP = &hc
	ctx, cancel := context.WithCancel(context.Background())
	return &Store{
		dir: o.Dir, fetch: fetch, catalogue: catalogue, log: o.Logger, now: o.Now,
		locales: map[string]*localeFile{}, attempted: map[string]bool{}, subs: map[int]func(Locale){},
		kick: make(chan struct{}, 1), ctx: ctx, cancel: cancel,
	}, nil
}

// catalogueLocked loads Sona's catalogue once; false when it is unusable
// (a broken build), which leaves the store empty.
func (s *Store) catalogueLocked() bool {
	if !s.catLoaded {
		s.catLoaded = true
		if s.cat, s.catErr = s.catalogue(); s.catErr != nil {
			s.log.Error("apple strings: catalogue", "err", s.catErr)
		}
	}
	return s.catErr == nil
}

// Lookup returns the cached messages for Apple's locale matching a
// preferred tag (MatchLocale), and remembers the tag so the store fetches
// for it once started. English tags have none: English is Sona's source.
func (s *Store) Lookup(tag string) Locale {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" || Skip(tag) != "" {
		return Locale{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.catalogueLocked() {
		return Locale{}
	}
	if !slices.Contains(s.wanted, tag) && len(s.wanted) < maxWanted {
		s.wanted = append(s.wanted, tag)
		if s.started {
			s.signal()
		}
	}
	idx := s.loadIndexLocked()
	if idx == nil {
		return Locale{}
	}
	apple := MatchLocale(tag, usable(idx.Locales))
	if apple == "" {
		return Locale{}
	}
	return s.locale(s.loadLocaleLocked(apple))
}

// Start begins fetching: call it once the network opens.
func (s *Store) Start() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	s.wg.Add(1)
	diag.Go(s.log, "apple strings", func() {
		defer s.wg.Done()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-s.kick:
				s.refresh(s.ctx)
			}
		}
	})
	s.signal()
}

// Close stops fetching and waits for a running fetch to end.
func (s *Store) Close() {
	s.cancel()
	s.wg.Wait()
}

// Subscribe calls fn with a locale's messages whenever they arrive or
// change. The returned function unsubscribes.
func (s *Store) Subscribe(fn func(Locale)) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextSub
	s.nextSub++
	s.subs[id] = fn
	return func() {
		s.mu.Lock()
		delete(s.subs, id)
		s.mu.Unlock()
	}
}

func (s *Store) signal() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// usable lists Apple's locales Sona uses.
func usable(locales map[string]string) []string {
	var tags []string
	for t := range locales {
		if Skip(t) == "" {
			tags = append(tags, t)
		}
	}
	return tags
}

func (s *Store) locale(f *localeFile) Locale {
	if f == nil {
		return Locale{}
	}
	return Locale{
		Tag: BCP47(f.Locale), Messages: maps.Clone(f.Messages), Total: s.cat.Messages,
		Version: chunkName(f.Bundle), FetchedAt: f.FetchedAt,
	}
}

// chunkName is an asset URL's file name.
func chunkName(url string) string { return url[strings.LastIndexByte(url, '/')+1:] }

// refresh brings the cache up to date for the wanted tags.
func (s *Store) refresh(ctx context.Context) {
	s.mu.Lock()
	wanted := slices.Clone(s.wanted)
	if len(wanted) == 0 {
		s.mu.Unlock()
		return // nothing asked for yet: no request at all
	}
	idx := s.loadIndexLocked()
	check := !s.indexChecked && (idx == nil || s.now().Sub(idx.CheckedAt) >= RefreshInterval)
	s.indexChecked = s.indexChecked || check
	s.mu.Unlock()

	if check {
		fresh, err := s.checkIndex(ctx, idx)
		if err != nil {
			s.log.Warn("apple strings: index check failed", "err", err)
		} else {
			idx = fresh
			s.mu.Lock()
			s.index = fresh
			s.mu.Unlock()
			if err := s.save("index.json", fresh); err != nil {
				s.log.Warn("apple strings: save index", "err", err)
			}
		}
	}
	if idx == nil {
		return
	}
	for _, tag := range wanted {
		apple := MatchLocale(tag, usable(idx.Locales))
		if apple == "" {
			continue
		}
		s.mu.Lock()
		done := s.attempted[apple]
		s.attempted[apple] = true
		cur := s.loadLocaleLocked(apple)
		s.mu.Unlock()
		url := idx.Locales[apple]
		if done || cur != nil && cur.Bundle == url && cur.Mapping == s.cat.Version {
			continue
		}
		if err := s.fetchLocale(ctx, apple, url); err != nil {
			s.log.Warn("apple strings: download failed", "locale", apple, "err", err)
		}
	}
}

// checkIndex finds the current index script and, when it changed, reads
// its locale table.
func (s *Store) checkIndex(ctx context.Context, old *indexFile) (*indexFile, error) {
	script, err := s.fetch.IndexScript(ctx)
	if err != nil {
		return nil, err
	}
	if old != nil && old.Script == script {
		fresh := *old
		fresh.CheckedAt = s.now()
		return &fresh, nil
	}
	locales, err := s.fetch.Locales(ctx, script)
	if err != nil {
		return nil, err
	}
	for tag := range locales {
		if !appleTagRe.MatchString(tag) {
			delete(locales, tag)
		}
	}
	s.log.Info("apple strings: new Apple release", "script", chunkName(script), "locales", len(locales))
	return &indexFile{Format: cacheFormat, Script: script, Locales: locales, CheckedAt: s.now()}, nil
}

func (s *Store) fetchLocale(ctx context.Context, apple, url string) error {
	b, err := s.fetch.Bundle(ctx, url)
	if err != nil {
		return err
	}
	conv := Convert(b, s.cat.Mappings)
	f := &localeFile{
		Format: cacheFormat, Locale: apple, Bundle: url, Mapping: s.cat.Version,
		FetchedAt: s.now(), Messages: conv.Messages,
	}
	s.log.Info("apple strings: downloaded", "locale", apple, "bundle", chunkName(url),
		"messages", len(conv.Messages), "missing", len(conv.Missing), "refused", len(conv.Invalid))
	for _, inv := range conv.Invalid {
		s.log.Debug("apple strings: refused", "locale", apple, "detail", inv)
	}
	if err := s.save(apple+".json", f); err != nil {
		s.log.Warn("apple strings: save", "locale", apple, "err", err)
	}
	s.mu.Lock()
	prev := s.locales[apple]
	s.locales[apple] = f
	changed := prev == nil || !maps.Equal(prev.Messages, f.Messages)
	loc := s.locale(f)
	subs := slices.Collect(maps.Values(s.subs))
	s.mu.Unlock()
	if changed {
		for _, fn := range subs {
			fn(loc)
		}
	}
	return nil
}

// appleTagRe is the shape of Apple's locale tags, which name cache files.
var appleTagRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+){0,3}$`)

func (s *Store) loadIndexLocked() *indexFile {
	if s.indexLoaded {
		return s.index
	}
	s.indexLoaded = true
	var f indexFile
	if !s.load("index.json", &f) || f.Format != cacheFormat || !s.trusted(f.Script) {
		return nil
	}
	for tag, url := range f.Locales {
		if !appleTagRe.MatchString(tag) || !s.trusted(url) {
			delete(f.Locales, tag)
		}
	}
	s.index = &f
	return s.index
}

// loadLocaleLocked returns Apple's locale's cache, keeping only messages
// the current mapping has (an older Sona's cache may hold others).
func (s *Store) loadLocaleLocked(apple string) *localeFile {
	if f, ok := s.locales[apple]; ok {
		return f
	}
	var f *localeFile
	var c localeFile
	if s.load(apple+".json", &c) && c.Format == cacheFormat && c.Locale == apple && s.trusted(c.Bundle) {
		ids := make(map[string]bool, len(s.cat.Mappings))
		for _, m := range s.cat.Mappings {
			ids[m.ID()] = true
		}
		for id, msg := range c.Messages {
			if !ids[id] || msg == "" || len(msg) > maxMessageBytes {
				delete(c.Messages, id)
			}
		}
		f = &c
	}
	s.locales[apple] = f
	return f
}

// trusted: a cached URL must be one of Apple's assets.
func (s *Store) trusted(url string) bool {
	return strings.HasPrefix(url, s.fetch.origin()+"/assets/")
}

// load decodes a cache file strictly; false when it is missing or unusable.
func (s *Store) load(name string, v any) bool {
	f, err := os.Open(filepath.Join(s.dir, name))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			s.log.Warn("apple strings: read cache", "file", name, "err", err)
		}
		return false
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, maxCacheBytes+1))
	if err == nil && len(b) > maxCacheBytes {
		err = ErrTooLarge
	}
	if err == nil {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		err = dec.Decode(v)
	}
	if err != nil {
		s.log.Warn("apple strings: ignoring cache file", "file", name, "err", err)
		return false
	}
	return true
}

// save writes a cache file atomically.
func (s *Store) save(name string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, name+".*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(b)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), filepath.Join(s.dir, name))
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("%s: %w", name, werr)
	}
	return nil
}
