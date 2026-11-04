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
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		add := func(key po.Key, hasCtx bool) {
			m := byKey[key]
			if m == nil {
				m = &uiMessage{Key: key, HasContext: hasCtx}
				byKey[key] = m
				msgs = append(msgs, m)
			}
			if !slices.Contains(m.refs, rel) {
				m.refs = append(m.refs, rel)
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
			if name == "TC" {
				add(po.Key{ID: lits[1], Context: lits[0]}, true)
			} else {
				add(po.Key{ID: lits[0]}, false)
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

// uiCatalogue is one of the UI's catalogue files on disk.
type uiCatalogue struct {
	path string // repository-relative
	raw  []byte
	file *po.File
	str  func(*uiMessage) string // the new entry's msgstr
}

// missing is the messages of msgs the catalogue lacks, in msgs' order.
func (c *uiCatalogue) missing(msgs []*uiMessage) []*uiMessage {
	have := map[po.Key]bool{}
	for _, e := range c.file.Entries {
		have[e.Key()] = true
	}
	var out []*uiMessage
	for _, m := range msgs {
		if !have[m.Key] {
			out = append(out, m)
		}
	}
	return out
}

// appended is the catalogue's bytes with the messages added at its end:
// what is there stays byte for byte, as Lingui wrote it.
func (c *uiCatalogue) appended(add []*uiMessage) []byte {
	f := &po.File{}
	for _, m := range add {
		f.Entries = append(f.Entries, &po.Entry{
			References: m.refs,
			Context:    m.Context,
			HasContext: m.HasContext,
			ID:         m.ID,
			Str:        c.str(m),
		})
	}
	// po.Format starts with the empty header's two lines; the entries
	// follow, each after a blank line.
	entries := bytes.TrimPrefix(po.Format(f), []byte("msgid \"\"\nmsgstr \"\"\n"))
	out := bytes.Clone(c.raw)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return append(out, entries...)
}

func (r repo) loadUICatalogue(rel string, str func(*uiMessage) string) (*uiCatalogue, error) {
	raw, err := os.ReadFile(r.path(rel))
	if err != nil {
		return nil, err
	}
	f, err := po.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	return &uiCatalogue{path: rel, raw: raw, file: f, str: str}, nil
}

// runExtractUI keeps the UI's catalogues in step with the calls in
// internal/ui: it adds the messages the code asks for and the catalogue
// lacks, never removing one (the Lingui messages stay until the catalogue
// is made exact). With check it writes nothing and fails when a message is
// missing or a call's text is not a literal.
func runExtractUI(r repo, check bool) error {
	msgs, scanErr := scanUI(string(r))
	cats := make([]*uiCatalogue, 0, 2)
	for _, c := range []struct {
		path string
		str  func(*uiMessage) string
	}{
		{sourcePOPath, func(m *uiMessage) string { return m.ID }}, // English is its own translation
		{pseudoPOPath, func(*uiMessage) string { return "" }},     // MyGo's pseudo locale transforms the English
	} {
		cat, err := r.loadUICatalogue(c.path, c.str)
		if err != nil {
			return err
		}
		cats = append(cats, cat)
	}
	var problems []string
	for _, cat := range cats {
		add := cat.missing(msgs)
		if len(add) == 0 {
			continue
		}
		if check {
			for _, m := range add {
				problems = append(problems, fmt.Sprintf("%s lacks %s (%s)", cat.path, describe(m), strings.Join(m.refs, ", ")))
			}
			continue
		}
		if err := os.WriteFile(r.path(cat.path), cat.appended(add), 0o644); err != nil {
			return err
		}
		fmt.Printf("%s: added %d messages\n", cat.path, len(add))
	}
	switch {
	case scanErr != nil:
		return scanErr
	case len(problems) > 0:
		return fmt.Errorf("the UI's catalogue is out of date (run `%s`):\n  %s", "go run ./cmd/i18ntool extract", strings.Join(problems, "\n  "))
	case check:
		fmt.Printf("%s is up to date: %d messages in use\n", sourcePOPath, len(msgs))
	}
	return nil
}

func describe(m *uiMessage) string {
	if m.HasContext {
		return strconv.Quote(m.Context) + "/" + strconv.Quote(m.ID)
	}
	return strconv.Quote(m.ID)
}
