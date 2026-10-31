//go:build !darwin && !windows

package store

// SystemBackend is where go-keyring keeps secrets on this OS: the freedesktop Secret Service.
const SystemBackend = BackendSecretService
