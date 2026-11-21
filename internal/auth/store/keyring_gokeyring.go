//go:build !darwin

package store

import "github.com/zalando/go-keyring"

// SystemKeyring is the system keyring through go-keyring: the Secret Service
// or Credential Manager (SystemBackend).
type SystemKeyring struct{}

func (SystemKeyring) Get(service, user string) (string, error) { return keyring.Get(service, user) }
func (SystemKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}
func (SystemKeyring) Delete(service, user string) error { return keyring.Delete(service, user) }
