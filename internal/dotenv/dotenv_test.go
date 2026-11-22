package dotenv

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestReadAndSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if v, err := Read(path, "A"); v != "" || err != nil {
		t.Fatalf("missing file: %q, %v", v, err)
	}
	if err := os.WriteFile(path, []byte("# c\nexport A=\"one\"\nB = 'two'\n\nC=x=y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"A": "one", "B": "two", "C": "x=y", "D": ""} {
		if v, err := Read(path, key); v != want || err != nil {
			t.Errorf("Read(%s) = %q, %v; want %q", key, v, err, want)
		}
	}
	if err := Set(path, "A", "new"); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, "D", "d"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if want := "# c\nB = 'two'\nC=x=y\nA=new\nD=d\n"; string(b) != want {
		t.Errorf("file = %q, want %q", b, want)
	}
}

func TestSetWritesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "main.env")
	link := filepath.Join(dir, ".env")
	if err := os.WriteFile(real, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks:", err)
	}
	if err := Set(link, "B", "2"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the symlink was replaced: %v, %v", fi, err)
	}
	if b, _ := os.ReadFile(real); string(b) != "A=1\nB=2\n" {
		t.Errorf("target = %q", b)
	}
}

func TestParse(t *testing.T) {
	in := "# a comment\n  \t\n" + `export A=1
export	B = two words   # a comment
C="dq \"x\" \n\t\\ \$HOME #not" # after
D='sq \n "x" #not'   # after
E=
F=""
G=a#b
exported=x
H = ' padded '
I=x=y
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []Var{
		{"A", "1"},
		{"B", "two words"},
		{"C", "dq \"x\" \n\t\\ $HOME #not"},
		{"D", `sq \n "x" #not`},
		{"E", ""},
		{"F", ""},
		{"G", "a#b"},
		{"exported", "x"},
		{"H", " padded "},
		{"I", "x=y"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestParseErrorsNameTheLineNotTheValue(t *testing.T) {
	for _, in := range []string{
		"A=1\nnot an assignment secret-value\n",
		"A=1\nB=\"secret-value\n",
		"A=1\nB='secret-value\n",
		"A=1\nB='secret-value' trailing\n",
		"A=1\n1B=secret-value\n",
		"A=1\nB C=secret-value\n",
	} {
		_, err := Parse(strings.NewReader(in))
		if err == nil {
			t.Errorf("%q parsed", in)
			continue
		}
		if !strings.HasPrefix(err.Error(), "line 2") || strings.Contains(err.Error(), "secret-value") {
			t.Errorf("%q: error %q, want line 2 and no value", in, err)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if vars, err := Load(filepath.Join(dir, ".env")); vars != nil || err != nil {
		t.Errorf("missing file: %v, %v", vars, err)
	}
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("A=secret-value\nB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "secret-value") {
		t.Errorf("error %v, want the path and no value", err)
	}
}

func TestMerge(t *testing.T) {
	env := []string{"=C:=C:\\src", "PATH=/bin", "SET=mine", "EMPTY=", "Lower=l"}
	vars := []Var{{"SET", "env-file"}, {"EMPTY", "env-file"}, {"NEW", "1"}, {"NEW", "2"}, {"LOWER", "file"}, {"C:", "x"}}

	got, added := merge(env, vars, false)
	wantEnv := append(slices.Clone(env), "NEW=1", "LOWER=file", "C:=x")
	if !slices.Equal(got, wantEnv) || !slices.Equal(added, []string{"NEW", "LOWER", "C:"}) {
		t.Errorf("got %q, added %q", got, added)
	}
	if len(env) != 5 || env[4] != "Lower=l" {
		t.Errorf("the caller's slice changed: %q", env)
	}

	// Windows: names compare regardless of case.
	got, added = merge(env, vars, true)
	if !slices.Equal(got, append(slices.Clone(env), "NEW=1", "C:=x")) || !slices.Equal(added, []string{"NEW", "C:"}) {
		t.Errorf("folded: got %q, added %q", got, added)
	}
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "sona-go")
	wt := filepath.Join(root, "sona-go-x")
	gitdir := filepath.Join(main, ".git", "worktrees", "sona-go-x")
	for _, d := range []string{gitdir, wt} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// What `git worktree add` writes (forward slashes on Windows too).
	write(filepath.Join(wt, ".git"), "gitdir: "+filepath.ToSlash(gitdir)+"\n")
	write(filepath.Join(gitdir, "commondir"), "../..\n")

	if got := Find(wt); got != "" {
		t.Errorf("no .env anywhere: %q", got)
	}
	write(filepath.Join(main, ".env"), "A=1\n")
	if got, want := Find(wt), filepath.Join(main, ".env"); got != want {
		t.Errorf("worktree without .env: %q, want the main checkout's %q", got, want)
	}
	if got, want := Find(main), filepath.Join(main, ".env"); got != want {
		t.Errorf("main checkout: %q, want %q", got, want)
	}
	write(filepath.Join(wt, ".env"), "A=2\n")
	if got, want := Find(wt), filepath.Join(wt, ".env"); got != want {
		t.Errorf("worktree with its own .env: %q, want %q", got, want)
	}

	// A relative gitdir, as `git worktree add --relative-paths` writes.
	other := filepath.Join(root, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(other, ".git"), "gitdir: ../sona-go/.git/worktrees/sona-go-x\n")
	if got, want := Find(other), filepath.Join(main, ".env"); got != want {
		t.Errorf("relative gitdir: %q, want %q", got, want)
	}

	// A submodule's .git points into the superproject's modules: no
	// commondir, no fallback.
	sub := filepath.Join(root, "sub")
	mod := filepath.Join(main, ".git", "modules", "sub")
	for _, d := range []string{sub, mod} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(sub, ".git"), "gitdir: "+mod+"\n")
	if got := Find(sub); got != "" {
		t.Errorf("submodule: %q", got)
	}
}
