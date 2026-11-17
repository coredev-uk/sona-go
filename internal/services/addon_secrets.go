package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/coredev-uk/sona-go/internal/auth/store"
)

// Keys of the secrets of Sona's own add-ons, in their keyring item
// `addon.<id>` (addons.BundledSecret lists what each may read): the Last.fm
// session key and the ListenBrainz user token.
const (
	lastfmSecretKey       = "session"
	listenbrainzSecretKey = "token"
)

// addonSecrets is the keyring item `addon.<id>` of each add-on: one JSON
// object of key → value, in the system keyring with the explicit 0600 file
// fallback. It is the one place that reads and writes those items, so the
// settings rows of an add-on (AddonsService) and the sign-in of Sona's own
// (ScrobbleService) never race on one item. The values never reach a log.
type addonSecrets struct {
	e  *Env
	mu sync.Mutex
}

// addonSecrets is the store of every add-on's secrets (opened over
// Env.openSecret at each use).
func (e *Env) addonSecrets() *addonSecrets {
	e.secretsOnce.Do(func() { e.secretStore = &addonSecrets{e: e} })
	return e.secretStore
}

func addonItem(id string) string { return "addon." + id }

func (a *addonSecrets) readLocked(id string) (store.Secret, map[string]string, error) {
	sec, err := a.e.openSecret(addonItem(id))
	if err != nil {
		return nil, nil, err
	}
	raw, err := sec.Get()
	if errors.Is(err, store.ErrNotFound) {
		return sec, map[string]string{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, nil, fmt.Errorf("addon secrets: %w", err)
	}
	return sec, m, nil
}

func writeSecret(sec store.Secret, m map[string]string) error {
	if len(m) == 0 {
		return sec.Delete()
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return sec.Set(string(b))
}

// all returns every secret of add-on id by key.
func (a *addonSecrets) all(id string) (map[string]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, m, err := a.readLocked(id)
	return m, err
}

// get returns one secret; store.ErrNotFound while there is none.
func (a *addonSecrets) get(id, key string) (string, error) {
	m, err := a.all(id)
	if err != nil {
		return "", err
	}
	if m[key] == "" {
		return "", store.ErrNotFound
	}
	return m[key], nil
}

// update reads the add-on's secrets, lets fn change them and writes them
// back, all under the one lock; an empty result removes the item.
func (a *addonSecrets) update(id string, fn func(map[string]string)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	sec, m, err := a.readLocked(id)
	if err != nil {
		return err
	}
	fn(m)
	return writeSecret(sec, m)
}

// slot is one key of the add-on's secrets as a store.Secret: how a service
// that keeps the sign-in of Sona's own add-on reads, writes and deletes it.
func (a *addonSecrets) slot(id, key string) store.Secret { return addonSlot{a, id, key} }

type addonSlot struct {
	a       *addonSecrets
	id, key string
}

func (s addonSlot) Get() (string, error) { return s.a.get(s.id, s.key) }

func (s addonSlot) Set(v string) error {
	return s.a.update(s.id, func(m map[string]string) { m[s.key] = v })
}

func (s addonSlot) Delete() error {
	return s.a.update(s.id, func(m map[string]string) { delete(m, s.key) })
}

func (s addonSlot) Backend() store.Backend {
	s.a.mu.Lock()
	defer s.a.mu.Unlock()
	sec, err := s.a.e.openSecret(addonItem(s.id))
	if err != nil {
		return store.SystemBackend
	}
	return sec.Backend()
}

// migrateSecret moves the sign-in a version before add-ons kept in the
// keyring item `from` (the whole value) to key of add-on id's item, once and
// with no moment when the sign-in is in neither place: copy, check the copy, then
// delete the old item. A crash after the copy leaves both, and the next
// start finishes (the check passes, the old item goes); a crash before it
// leaves the old one alone. If the new place already holds a different value
// (an account connected after the move) it is the newer, and wins; the old
// one is deleted either way.
func (e *Env) migrateSecret(log *slog.Logger, from, id, key string) error {
	old, err := e.openSecret(from)
	if err != nil {
		return err
	}
	v, err := old.Get()
	if errors.Is(err, store.ErrNotFound) {
		return nil // nothing to move, or already moved
	}
	if err != nil {
		return err
	}
	ss := e.addonSecrets()
	switch cur, err := ss.get(id, key); {
	case errors.Is(err, store.ErrNotFound):
		if err := ss.update(id, func(m map[string]string) { m[key] = v }); err != nil {
			return fmt.Errorf("copy: %w", err)
		}
		if got, err := ss.get(id, key); err != nil || got != v {
			return errors.New("the copy doesn't read back; the old item is kept")
		}
	case err != nil:
		return err
	case cur != v:
		log.Info("secrets: the new place holds a newer sign-in; the old one is dropped", "add-on", id)
	}
	if err := old.Delete(); err != nil {
		return fmt.Errorf("delete the old item: %w", err)
	}
	log.Info("secrets: moved to the add-on's own item", "add-on", id)
	return nil
}
