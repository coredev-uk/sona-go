package services

import (
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

// memKeyring is an in-memory Secret Service.
type memKeyring struct {
	mu    sync.Mutex
	items map[string]string
}

func (k *memKeyring) Get(service, user string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.items[service+"/"+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (k *memKeyring) Set(service, user, password string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.items == nil {
		k.items = map[string]string{}
	}
	k.items[service+"/"+user] = password
	return nil
}

func (k *memKeyring) Delete(service, user string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.items[service+"/"+user]; !ok {
		return keyring.ErrNotFound
	}
	delete(k.items, service+"/"+user)
	return nil
}

func (k *memKeyring) all() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	var b strings.Builder
	for key, v := range k.items {
		b.WriteString(key + "=" + v + "\n")
	}
	return b.String()
}
