// Package dotenv reads and updates KEY=value lines in a .env file: the
// manual spike tools and the live tests read a Music User Token from it, and
// cmd/devloop hands its variables to the dev build where devenv, which loads
// it on Linux, does not run (Windows).
package dotenv

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/coredev-uk/sona-go/internal/fsutil"
)

// Var is one assignment of an env file.
type Var struct{ Key, Value string }

// Parse reads the assignments of an env file, in order. Each line is blank,
// a comment (#…), or KEY=value with an optional "export " prefix, KEY a
// shell name and spaces around the '=' allowed. The value is "double quoted"
// (\n, \r, \t, \", \\ and \$ escapes), 'single quoted' (literal), or bare:
// trimmed, and ended by a # after a space. Either may be followed by a
// comment. There is no variable expansion and no value spans lines. An
// error names the line, never its text: the file holds secrets.
func Parse(r io.Reader) ([]Var, error) {
	var vars []Var
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export"); ok && rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
			line = strings.TrimSpace(rest)
		}
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || !isName(k) {
			return nil, fmt.Errorf("line %d: not a KEY=value assignment", n)
		}
		v, err := value(strings.TrimSpace(v))
		if err != nil {
			return nil, fmt.Errorf("line %d (%s): %w", n, k, err)
		}
		vars = append(vars, Var{Key: k, Value: v})
	}
	return vars, sc.Err()
}

// value unquotes the text after a line's '='.
func value(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	var v strings.Builder
	var rest string
	switch q := s[0]; q {
	case '\'':
		end := strings.IndexByte(s[1:], '\'')
		if end < 0 {
			return "", errors.New("no closing quote")
		}
		v.WriteString(s[1 : 1+end])
		rest = s[2+end:]
	case '"':
		i := 1
		for ; i < len(s) && s[i] != '"'; i++ {
			if s[i] != '\\' || i+1 == len(s) {
				v.WriteByte(s[i])
				continue
			}
			i++
			switch c := s[i]; c {
			case 'n':
				v.WriteByte('\n')
			case 'r':
				v.WriteByte('\r')
			case 't':
				v.WriteByte('\t')
			case '"', '\\', '$':
				v.WriteByte(c)
			default:
				v.WriteByte('\\')
				v.WriteByte(c)
			}
		}
		if i == len(s) {
			return "", errors.New("no closing quote")
		}
		rest = s[i+1:]
	default:
		for i := 1; i < len(s); i++ {
			if s[i] == '#' && (s[i-1] == ' ' || s[i-1] == '\t') {
				s = s[:i]
				break
			}
		}
		return strings.TrimSpace(s), nil
	}
	if rest = strings.TrimSpace(rest); rest != "" && rest[0] != '#' {
		return "", errors.New("text after the closing quote")
	}
	return v.String(), nil
}

func isName(k string) bool {
	if k == "" || k[0] >= '0' && k[0] <= '9' {
		return false
	}
	for _, c := range k {
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// Load parses the env file at path; nil when it does not exist. An error
// names the file and the line, never a value.
func Load(path string) ([]Var, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	vars, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return vars, nil
}

// Read returns the value of key in the env file at path (its first
// assignment, as Parse reads it); "" when the file or the key is missing.
func Read(path, key string) (string, error) {
	vars, err := Load(path)
	for _, v := range vars {
		if v.Key == key {
			return v.Value, nil
		}
	}
	return "", err
}

// Merge returns env (KEY=value entries, as os.Environ gives them) with the
// vars it does not set appended, and the keys it added. The environment
// wins, as with devenv's dotenv: a key it sets, even to "", is kept, and so
// is the first of a key the file repeats. Keys compare as the system does:
// regardless of case on Windows.
func Merge(env []string, vars []Var) (merged, added []string) {
	return merge(env, vars, runtime.GOOS == "windows")
}

func merge(env []string, vars []Var, foldCase bool) (merged, added []string) {
	norm := func(k string) string {
		if foldCase {
			return strings.ToUpper(k)
		}
		return k
	}
	set := make(map[string]bool, len(env)+len(vars))
	for _, kv := range env {
		// The name ends at the first '=' after its first byte: Windows
		// keeps per-drive working directories as "=C:=C:\dir".
		if kv == "" {
			continue
		}
		if i := strings.IndexByte(kv[1:], '='); i >= 0 {
			set[norm(kv[:1+i])] = true
		}
	}
	merged = env[:len(env):len(env)] // appending copies: the caller's slice stays as it was
	for _, v := range vars {
		if k := norm(v.Key); !set[k] {
			set[k] = true
			merged = append(merged, v.Key+"="+v.Value)
			added = append(added, v.Key)
		}
	}
	return merged, added
}

// Find returns the env file of the checkout at dir: its own .env or, in a
// git worktree without one, the main checkout's, so a worktree needs no
// copy or symlink (Windows makes a symlink only in Developer Mode or an
// elevated shell). "" when neither exists.
func Find(dir string) string {
	if p := filepath.Join(dir, ".env"); isFile(p) {
		return p
	}
	if main := mainCheckout(dir); main != "" {
		if p := filepath.Join(main, ".env"); isFile(p) {
			return p
		}
	}
	return ""
}

// mainCheckout is the main working tree of the git worktree at dir, read
// from the files git keeps (dir/.git is "gitdir: <dir>", whose commondir
// names the repository's .git); "" when dir is not a linked worktree (a main
// checkout, whose .git is a directory; a submodule; a bare repository's
// worktree).
func mainCheckout(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		return ""
	}
	gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
	if !ok {
		return ""
	}
	gitdir = resolve(dir, strings.TrimSpace(gitdir))
	c, err := os.ReadFile(filepath.Join(gitdir, "commondir"))
	if err != nil {
		return ""
	}
	common := resolve(gitdir, strings.TrimSpace(string(c)))
	if filepath.Base(common) != ".git" {
		return ""
	}
	return filepath.Dir(common)
}

func resolve(base, p string) string {
	if p = filepath.FromSlash(p); filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(base, p)
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// Set writes key=value to the env file at path (mode 0600), replacing the
// key's earlier lines and keeping the others.
func Set(path, key, value string) error {
	// A symlinked .env (worktrees link the main checkout's) is written
	// through, not replaced by a regular file.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	var lines []string
	if b, err := os.ReadFile(path); err == nil {
		for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			if k, _, ok := strings.Cut(strings.TrimPrefix(l, "export "), "="); ok && strings.TrimSpace(k) == key {
				continue
			}
			if l != "" {
				lines = append(lines, l)
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	lines = append(lines, key+"="+value)
	return fsutil.WriteFileAtomic(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}
