package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/coredev-uk/sona-go/internal/i18n/l10n/extract"
	"github.com/coredev-uk/sona-go/internal/i18n/po"
)

// uiI18nImport is the package whose T and TC calls hold the interface's
// messages (docs/ui-architecture.md, "Strings and locales").
const uiI18nImport = "github.com/coredev-uk/sona-go/internal/ui/i18n"

// uiMessage is one message the interface's Go code asks for.
type uiMessage struct {
	po.Key
	HasContext bool
	refs       []string // files, slash paths relative to the root
	notes      []string // translators' notes, sorted and without repeats
}

// noteMarker starts a translator's note in a comment: the comment group
// directly above a call, or one that ends on the line the call ends on,
// holds a note for each line that starts with it (the lines after it, up to
// the next marker, continue the note).
const noteMarker = "translators:"

// noteGroup is one comment group's notes and the lines it covers.
type noteGroup struct {
	first, last int
	notes       []string
}

// noteGroups reads the translators' notes of a file's comments.
func noteGroups(fset *token.FileSet, f *ast.File) []noteGroup {
	var out []noteGroup
	for _, g := range f.Comments {
		var notes []string
		for _, line := range strings.Split(g.Text(), "\n") {
			line = strings.TrimSpace(line)
			if rest, ok := strings.CutPrefix(line, noteMarker); ok {
				notes = append(notes, strings.TrimSpace(rest))
			} else if line != "" && len(notes) > 0 {
				notes[len(notes)-1] += " " + line
			}
		}
		if len(notes) > 0 {
			out = append(out, noteGroup{fset.Position(g.Pos()).Line, fset.Position(g.End()).Line, notes})
		}
	}
	return out
}

// noteFor is the notes that belong to a call spanning lines first to last.
func noteFor(groups []noteGroup, first, last int) []string {
	var out []string
	for _, g := range groups {
		if g.last == first-1 || g.first == last {
			out = append(out, g.notes...)
		}
	}
	return out
}

// scanUI reads the Go files under internal/ui as syntax only (go/parser,
// no type checking, build tags ignored) and collects the first arguments
// of i18n.T(msg, …) and i18n.TC(context, msg, …). Test files are skipped.
// Each must be a string literal (or literals joined with +): the message
// is its text. Inside package i18n itself T and TC are called bare.
// The returned error joins one line per call that is not.
func scanUI(root string) ([]*uiMessage, error) {
	var (
		msgs   []*uiMessage
		byKey  = map[po.Key]*uiMessage{}
		errs   []error
		fset   = token.NewFileSet()
		uiRoot = filepath.Join(root, filepath.FromSlash(uiDir))
	)
	err := filepath.WalkDir(uiRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution|parser.ParseComments)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		groups := noteGroups(fset, f)
		add := func(key po.Key, hasCtx bool, notes []string) {
			m := byKey[key]
			if m == nil {
				m = &uiMessage{Key: key, HasContext: hasCtx}
				byKey[key] = m
				msgs = append(msgs, m)
			}
			if !slices.Contains(m.refs, rel) {
				m.refs = append(m.refs, rel)
			}
			for _, n := range notes {
				if !slices.Contains(m.notes, n) {
					m.notes = append(m.notes, n)
				}
			}
		}
		pkg := i18nName(f)
		// Package i18n's own files call T and TC without a qualifier; their
		// definitions pass their arguments on, which are no messages.
		own := path.Dir(rel) == uiI18nDir
		inspect := func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := calledName(call.Fun, pkg, own)
			if !ok {
				return true
			}
			want := 1
			if name == "TC" {
				want = 2
			}
			at := fset.Position(call.Pos())
			where := fmt.Sprintf("%s:%d", rel, at.Line)
			if len(call.Args) < want {
				errs = append(errs, fmt.Errorf("%s: i18n.%s needs %d string literal argument(s) first", where, name, want))
				return true
			}
			var lits []string
			for _, a := range call.Args[:want] {
				s, ok := literalString(a)
				if !ok {
					errs = append(errs, fmt.Errorf("%s: the %s of i18n.%s must be a string literal, as the catalogue cannot hold its text otherwise", where, argName(name, len(lits)), name))
					return true
				}
				lits = append(lits, s)
			}
			notes := noteFor(groups, at.Line, fset.Position(call.End()).Line)
			if name == "TC" {
				add(po.Key{ID: lits[1], Context: lits[0]}, true, notes)
			} else {
				add(po.Key{ID: lits[0]}, false, notes)
			}
			return true
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && own && fn.Recv == nil && (fn.Name.Name == "T" || fn.Name.Name == "TC") {
				continue
			}
			ast.Inspect(d, inspect)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, err)
	}
	for _, m := range msgs {
		slices.Sort(m.refs)
		slices.Sort(m.notes)
	}
	slices.SortFunc(msgs, func(a, b *uiMessage) int {
		if c := strings.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		return strings.Compare(a.Context, b.Context)
	})
	return msgs, errors.Join(errs...)
}

// uiI18nDir is the i18n package's directory relative to the root.
const uiI18nDir = "internal/ui/i18n"

func argName(fn string, i int) string {
	if fn == "TC" && i == 0 {
		return "context"
	}
	return "message"
}

// i18nName is the name the file imports the UI's i18n package under ("" if
// it doesn't).
func i18nName(f *ast.File) string {
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != uiI18nImport {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "i18n"
	}
	return ""
}

// calledName reports whether fun is i18n.T or i18n.TC (or, inside package
// i18n, T or TC) and which.
func calledName(fun ast.Expr, pkg string, own bool) (string, bool) {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		x, ok := f.X.(*ast.Ident)
		if ok && pkg != "" && x.Name == pkg && (f.Sel.Name == "T" || f.Sel.Name == "TC") {
			return f.Sel.Name, true
		}
	case *ast.Ident:
		if own && (f.Name == "T" || f.Name == "TC") {
			return f.Name, true
		}
	}
	return "", false
}

// literalString is the value of a string literal or of literals joined
// with +.
func literalString(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		l, ok1 := literalString(e.X)
		r, ok2 := literalString(e.Y)
		return l + r, ok1 && ok2
	case *ast.ParenExpr:
		return literalString(e.X)
	}
	return "", false
}

// uiHeader is the header of the UI's catalogues: no creation date, so
// extracting twice writes the same bytes. language is the file's tag.
func uiHeader(language string) string {
	return po.Header(
		[2]string{"MIME-Version", "1.0"},
		[2]string{"Content-Type", "text/plain; charset=utf-8"},
		[2]string{"Content-Transfer-Encoding", "8bit"},
		[2]string{"X-Generator", "sona i18ntool"},
		[2]string{"Language", language},
	)
}

// uiCatalogue is one of the UI's catalogue files, as extraction writes it.
type uiCatalogue struct {
	path     string // repository-relative
	language string
	str      func(*uiMessage) string // an entry's msgstr
}

// uiCatalogues are the files extraction writes: the English source, whose
// msgstr is its msgid, and the pseudo locale's, whose msgstr is empty (MyGo
// transforms the English).
var uiCatalogues = []uiCatalogue{
	{sourcePOPath, "en", func(m *uiMessage) string { return m.ID }},
	{pseudoPOPath, "en-XA", func(*uiMessage) string { return "" }},
}

// file is the catalogue extraction writes for msgs: every message once,
// sorted, with the Go files that ask for it.
func (c *uiCatalogue) file(msgs []*uiMessage) *po.File {
	f := &po.File{Header: uiHeader(c.language)}
	for _, m := range msgs {
		f.Entries = append(f.Entries, &po.Entry{
			Extracted:  m.notes,
			References: m.refs,
			Context:    m.Context,
			HasContext: m.HasContext,
			ID:         m.ID,
			Str:        c.str(m),
		})
	}
	return f
}

// runExtractUI writes the UI's catalogues from the calls in internal/ui:
// each holds exactly the messages the code asks for. With check it writes
// nothing and fails when a file is out of date or a call's text is not a
// literal.
func runExtractUI(r repo, check bool) error {
	msgs, scanErr := scanUI(string(r))
	if scanErr != nil {
		return scanErr
	}
	var problems []string
	for _, c := range uiCatalogues {
		want := po.Format(c.file(msgs))
		got, err := os.ReadFile(r.path(c.path))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if bytes.Equal(got, want) {
			if check {
				fmt.Printf("%s is up to date: %d messages\n", c.path, len(msgs))
			}
			continue
		}
		if check {
			diff := []string{"~ formatting"}
			if old, err := po.Parse(got); err == nil {
				if d := extract.Diff(old, c.file(msgs)); len(d) > 0 {
					diff = d
				}
			}
			problems = append(problems, fmt.Sprintf("%s is out of date:\n  %s", c.path, strings.Join(diff, "\n  ")))
			continue
		}
		if err := os.MkdirAll(filepath.Dir(r.path(c.path)), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(r.path(c.path), want, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s: %d messages\n", c.path, len(msgs))
	}
	if len(problems) > 0 {
		return fmt.Errorf("the UI's catalogue is out of date (run `go run ./cmd/i18ntool extract`):\n%s", strings.Join(problems, "\n"))
	}
	// The Apple mapping names English messages: one the UI stopped asking
	// for is a mapping to remove (or the message to bring back).
	sona, err := r.loadSona()
	if err != nil {
		return err
	}
	_, err = r.loadMappings(sona)
	return err
}
