// Package dotenv reads and updates KEY=value lines in a .env file, for the
// manual spike tools and the live tests that keep a Music User Token there.
package dotenv

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/coredev-uk/sona-go/internal/fsutil"
)

// Read returns the value of key in the env file at path: the text after the
// first '=' of the line, trimmed and unquoted, with an optional "export "
// prefix on the line allowed. "" when the file or the key is missing.
func Read(path, key string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimPrefix(strings.TrimSpace(sc.Text()), "export ")
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`), nil
		}
	}
	return "", sc.Err()
}

// Set writes key=value to the env file at path (mode 0600), replacing the
// key's earlier lines and keeping the others.
func Set(path, key, value string) error {
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
