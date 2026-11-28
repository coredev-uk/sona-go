package services

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// profileEvery is how long the stored profile name is trusted: Apple is
// asked again at the next start or sign-in after this long, never sooner.
const profileEvery = 24 * time.Hour

// ProfileService keeps the name on the signed-in person's Apple Music
// profile for the For You greeting. The name is read from SQLite, so a cold
// start needs no request; Apple is asked once the network opens, when a
// person signs in (the account row is new then) and at most once a day. It
// stays on this computer: it is never logged (only that it was fetched) and
// sign-out clears it with the account row.
type ProfileService struct {
	e *Env

	mu       sync.Mutex
	fetching bool
}

func newProfileService(e *Env) *ProfileService {
	s := &ProfileService{e: e}
	e.OnNetwork(func() { s.refreshSoon(false) })
	prev := e.Auth.Status().State
	e.Auth.OnChange(func(st auth.Status) {
		was := prev
		prev = st.State
		if st.State == auth.StateSignedIn && was != auth.StateSignedIn {
			// A sign-in the person just made (or made again, after Apple
			// refused the old one) asks at once: it may be another account.
			s.refreshSoon(was == auth.StateSigningIn || was == auth.StateNeedsReauth)
		}
	})
	e.OnSignOut("profile name", func(context.Context) error {
		// The account row, and the name on it, is deleted by the sign-out
		// itself; tell the UI so the greeting drops it now.
		s.announce(GreetingName{})
		return nil
	})
	return s
}

// GreetingName returns what the greeting calls the person, from SQLite
// alone ("" when there is nothing).
func (s *ProfileService) GreetingName(ctx context.Context) (GreetingName, error) {
	row, err := s.e.DB.RQ.GetAccountProfile(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return GreetingName{}, nil
	}
	if err != nil {
		return GreetingName{}, err
	}
	return GreetingName{Name: givenName(row.ProfileName)}, nil
}

// refreshSoon asks for the profile once the network is open, off the
// caller's goroutine; force asks even when the stored answer is fresh.
func (s *ProfileService) refreshSoon(force bool) {
	s.e.Go("profile name", func() {
		s.e.OnNetwork(func() { s.e.Go("profile name fetch", func() { s.refresh(force) }) })
	})
}

// refresh asks Apple for the profile when someone is signed in and the
// stored answer is missing or older than profileEvery (or force), and
// stores it. A failed request keeps what is stored and is tried again at the
// next start or sign-in.
func (s *ProfileService) refresh(force bool) {
	s.mu.Lock()
	if s.fetching {
		s.mu.Unlock()
		return
	}
	s.fetching = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.fetching = false
		s.mu.Unlock()
	}()

	if s.e.Auth.Status().State != auth.StateSignedIn || !s.e.hasMUT() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	now := s.e.Now()
	row, err := s.e.DB.RQ.GetAccountProfile(ctx)
	if err != nil {
		return // no account row yet: the next sign-in asks
	}
	if !force && row.ProfileCheckedAt.Valid && now.Sub(db.Time(row.ProfileCheckedAt.Int64)) < profileEvery {
		return
	}
	p, err := s.e.API.OwnSocialProfile(ctx)
	name := ""
	switch {
	case errors.Is(err, applemusic.ErrNotFound): // no profile: nothing to say
	case err != nil:
		s.e.Report(err)
		s.e.Logger.Warn("profile: not fetched", "err", err)
		return
	case p.IsOnboarded:
		name = strings.TrimSpace(p.Name)
	}
	n, err := s.e.DB.Q.SetAccountProfile(ctx, dbq.SetAccountProfileParams{ProfileName: name, ProfileCheckedAt: db.NullMillis(now)})
	if err != nil {
		s.e.Logger.Warn("profile: not stored", "err", err)
		return
	}
	if n == 0 { // signed out meanwhile: the account row is gone
		return
	}
	s.e.Logger.Info("profile: fetched")
	s.announce(GreetingName{Name: givenName(name)})
}

func (s *ProfileService) announce(g GreetingName) {
	if s.e.Emit != nil {
		s.e.Emit.Emit(EventProfileChanged, g)
	}
}

// givenName is the part of an Apple Music profile name the greeting uses.
// Where the script puts the given name first (Latin, Cyrillic, Greek,
// Armenian, Georgian) it is the first word: "Alex Morgan" is "Alex". Any
// other script, or a name mixing scripts, is used whole: Han, Kana and
// Hangul names put the family name first ("山田 太郎"), and other scripts
// vary too much to cut at a space. A name of one word is that word.
func givenName(name string) string {
	words := strings.Fields(name)
	if len(words) < 2 || !givenNameFirst(name) {
		return strings.Join(words, " ")
	}
	return words[0]
}

// givenNameFirst reports whether every letter of name is in a script that
// puts the given name first, and there is a letter.
func givenNameFirst(name string) bool {
	letters := false
	for _, r := range name {
		if !unicode.IsLetter(r) {
			continue
		}
		if !unicode.In(r, unicode.Latin, unicode.Cyrillic, unicode.Greek, unicode.Armenian, unicode.Georgian) {
			return false
		}
		letters = true
	}
	return letters
}
