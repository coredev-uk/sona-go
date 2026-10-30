package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FileStore is a Store keeping the queue as JSON in one file
// (e.g. $XDG_STATE_HOME/sona/queue.json). Writes are atomic.
type FileStore struct{ Path string }

// LoadQueue implements Store; a missing file is no saved queue.
func (f FileStore) LoadQueue(context.Context) (*Saved, error) {
	b, err := os.ReadFile(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("coordinator: load queue: %w", err)
	}
	var s Saved
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("coordinator: load queue: %w", err)
	}
	return &s, nil
}

// SaveQueue implements Store.
func (f FileStore) SaveQueue(_ context.Context, s Saved) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	dir := filepath.Dir(f.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("coordinator: save queue: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".queue-*")
	if err != nil {
		return fmt.Errorf("coordinator: save queue: %w", err)
	}
	_, err = tmp.Write(b)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), f.Path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("coordinator: save queue: %w", err)
	}
	return nil
}
