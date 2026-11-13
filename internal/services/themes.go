package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// themesPoll is how often the themes folder is checked for changes while
// Sona runs. A poll of one directory's names, sizes and times costs next to
// nothing and needs no file-watching dependency.
const themesPoll = 2 * time.Second

// ThemesService reads the user's theme files, `<config dir>/themes/*.json`
// (docs/themes.md). It only lists and reads them: the UI's theme registry
// (internal/ui/theme) parses, checks and applies them. After the first paint
// it polls the folder every two seconds and emits "themes:changed" with the
// files when one is added, edited or removed, so an edit shows without a
// restart. The folder is created when the user asks to open it, never at
// start-up; until then it simply has no files.
type ThemesService struct {
	e        *Env
	dir      string
	interval time.Duration
	quit     chan struct{}
	done     chan struct{}

	mu      sync.Mutex
	started bool
	// seen is the folder's signature the last time its files were listed or
	// announced: the poll reports a change against it.
	seen string
}

func newThemesService(e *Env) *ThemesService {
	s := &ThemesService{
		e: e, dir: e.Paths.ThemesDir(), interval: themesPoll,
		quit: make(chan struct{}), done: make(chan struct{}),
	}
	if e.Gate != nil {
		e.OnNetwork(s.start)
	}
	e.OnClose(s.close)
	return s
}

// Folder is where the user's theme files go.
func (s *ThemesService) Folder() string { return s.dir }

// Files lists the folder's *.json files by name, with their bytes: the
// regular files (a link to one counts), at most maxThemeFiles of them, none
// read beyond MaxThemeFileBytes. A folder that does not exist has none. It
// reads files, so call it off the main thread.
func (s *ThemesService) Files(ctx context.Context) ([]ThemeFile, error) {
	files, sig, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.seen = sig
	s.mu.Unlock()
	return files, nil
}

// OpenFolder creates the themes folder if it is not there yet and opens it
// in the file manager (the way Settings opens the log folder).
func (s *ThemesService) OpenFolder() error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("themes: create %s: %w", s.dir, err)
	}
	if s.e.OpenURL == nil {
		return fmt.Errorf("themes: open folder: %w", errors.ErrUnsupported)
	}
	return s.e.OpenURL("file://" + s.dir)
}

// entries are the folder's candidate files, by name: directories,
// other file types and names that are not *.json (any case) left out.
func (s *ThemesService) entries() ([]os.FileInfo, error) {
	list, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("themes: list %s: %w", s.dir, err)
	}
	var out []os.FileInfo
	for _, d := range list {
		name := d.Name()
		if strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), ".json") {
			continue
		}
		fi, err := os.Stat(filepath.Join(s.dir, name)) // follows a link
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		out = append(out, namedInfo{fi, name})
		if len(out) == maxThemeFiles {
			break
		}
	}
	slices.SortFunc(out, func(a, b os.FileInfo) int { return strings.Compare(a.Name(), b.Name()) })
	return out, nil
}

// namedInfo is a FileInfo under the name it has in the folder (a link's
// target has another).
type namedInfo struct {
	os.FileInfo
	name string
}

func (n namedInfo) Name() string { return n.name }

// signature is what changes when a file is added, edited or removed.
func signature(list []os.FileInfo) string {
	var b strings.Builder
	for _, fi := range list {
		fmt.Fprintf(&b, "%s|%d|%d\n", fi.Name(), fi.Size(), fi.ModTime().UnixNano())
	}
	return b.String()
}

// read lists the files with their bytes and the folder's signature.
func (s *ThemesService) read(ctx context.Context) ([]ThemeFile, string, error) {
	list, err := s.entries()
	if err != nil {
		return nil, "", err
	}
	out := make([]ThemeFile, 0, len(list))
	for _, fi := range list {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		f := ThemeFile{Name: fi.Name(), Size: fi.Size(), ModTime: fi.ModTime().UnixMilli()}
		if fi.Size() > MaxThemeFileBytes {
			f.TooLarge = true
		} else if data, err := readCapped(filepath.Join(s.dir, fi.Name())); err != nil {
			f.Unreadable = true
		} else if int64(len(data)) > MaxThemeFileBytes { // grew while being read
			f.TooLarge = true
		} else {
			f.Data = data
		}
		out = append(out, f)
	}
	return out, signature(list), nil
}

// readCapped reads at most MaxThemeFileBytes+1 bytes of the file.
func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, MaxThemeFileBytes+1))
}

// start begins polling (once the first paint is done).
func (s *ThemesService) start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.quit:
		return
	default:
	}
	if s.started {
		return
	}
	s.started = true
	s.e.Go("themes folder", s.run)
}

func (s *ThemesService) run() {
	defer close(s.done)
	tick := time.NewTicker(s.interval)
	defer tick.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-tick.C:
			s.poll()
		}
	}
}

// poll announces the folder when it differs from what was last listed.
func (s *ThemesService) poll() {
	list, err := s.entries()
	if err != nil {
		s.e.Logger.Warn("themes folder", "err", err)
		return
	}
	sig := signature(list)
	s.mu.Lock()
	same := sig == s.seen
	s.mu.Unlock()
	if same {
		return
	}
	files, sig, err := s.read(context.Background())
	if err != nil {
		s.e.Logger.Warn("themes folder", "err", err)
		return
	}
	s.mu.Lock()
	s.seen = sig
	s.mu.Unlock()
	if s.e.Emit != nil {
		s.e.Emit.Emit(EventThemesChanged, files)
	}
}

func (s *ThemesService) close() {
	s.mu.Lock()
	select {
	case <-s.quit:
		s.mu.Unlock()
		return
	default:
	}
	close(s.quit)
	started := s.started
	s.mu.Unlock()
	if started {
		<-s.done
	}
}
