package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/coredev-uk/sona-go/internal/fsutil"
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
	if err := fsutil.WriteFileAtomic(f.Path, b, 0o600); err != nil {
		return fmt.Errorf("coordinator: save queue: %w", err)
	}
	return nil
}
