// Package devwatch notices changes in a source tree by polling it: the dev
// loop (cmd/devloop) rebuilds on them and development builds reload design
// tokens and catalogues with them. Polling is a few hundred stat calls every
// couple of hundred milliseconds, needs no native watcher, and behaves the
// same on every OS.
package devwatch

import (
	"context"
	"io/fs"
	"path/filepath"
	"slices"
	"time"
)

// Options say what a Watcher looks at.
type Options struct {
	// Roots are directories (walked recursively) and files.
	Roots []string
	// SkipDir, when set, is asked about every directory below a root by its
	// base name; true skips it and everything in it.
	SkipDir func(name string) bool
	// Match, when set, is asked about every file; false leaves it out.
	Match func(path string) bool
}

// Interval is how often Watch looks.
const Interval = 100 * time.Millisecond

type stamp struct {
	mod  time.Time
	size int64
}

// Watcher remembers what the tree looked like and reports what differs.
type Watcher struct {
	opts Options
	seen map[string]stamp
}

// New returns a Watcher whose first Changed is the baseline: it reports
// nothing.
func New(opts Options) *Watcher { return &Watcher{opts: opts} }

// scan is every matching file now.
func (w *Watcher) scan() map[string]stamp {
	now := make(map[string]stamp, len(w.seen))
	for _, root := range w.opts.Roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // vanished while walking: the next poll sees it
			}
			if d.IsDir() {
				if path != root && w.opts.SkipDir != nil && w.opts.SkipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || (w.opts.Match != nil && !w.opts.Match(path)) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			now[path] = stamp{info.ModTime(), info.Size()}
			return nil
		})
	}
	return now
}

// Changed returns the files created, written or removed since the last
// call, sorted; nil on the first call and when nothing changed.
func (w *Watcher) Changed() []string {
	now := w.scan()
	prev := w.seen
	w.seen = now
	if prev == nil {
		return nil
	}
	var out []string
	for p, s := range now {
		if old, ok := prev[p]; !ok || old != s {
			out = append(out, p)
		}
	}
	for p := range prev {
		if _, ok := now[p]; !ok {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// Watch calls fn with each batch of changes until ctx ends. Changes that
// keep coming across polls wait until one poll passes without more, so an
// editor saving in several steps is one batch.
func Watch(ctx context.Context, opts Options, fn func(changed []string)) {
	w := New(opts)
	w.Changed()
	t := time.NewTicker(Interval)
	defer t.Stop()
	var batch []string
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		more := w.Changed()
		if len(more) > 0 {
			batch = append(batch, more...)
			continue
		}
		if len(batch) > 0 {
			slices.Sort(batch)
			fn(slices.Compact(batch))
			batch = nil
		}
	}
}
