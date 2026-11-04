// Package extract collects Go's catalogue messages (internal/i18n/l10n)
// from the module's source into the English catalogue, locales/en.po:
// `go run ./cmd/i18ntool extract` writes it, `extract -check` and this
// package's test fail when it is out of date. It reads the source as
// syntax only (go/parser, no type checking), so every file counts whatever
// its build tags or OS, and go/parser stays out of the app's binary.
//
// A message is marked as the l10n package doc says: a constant or
// variable declared with the type l10n.Message (Message inside package
// l10n), a conversion l10n.Message("…"), or a value in a composite literal
// of type map[K]l10n.Message, []l10n.Message or [N]l10n.Message. The
// marked value must be a string literal (interpreted or raw), literals
// joined with +, or the name of another marked constant or variable
// (`[]l10n.Message{msgPlay, msgPause}`); anything else is an error naming
// the file and line, since the catalogue could not hold its text.
package extract

import (
	"bufio"
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

	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
	"github.com/coredev-uk/sona-go/internal/i18n/po"
)

// PackageDir is the l10n package's directory relative to the module root.
const PackageDir = "internal/i18n/l10n"

// SourcePath is the English catalogue extraction writes, relative to the
// module root.
const SourcePath = PackageDir + "/" + l10n.SourceFile

// Command is how to bring SourcePath up to date, for error messages.
const Command = "go run ./cmd/i18ntool extract"

// header is the catalogue's header: Lingui's fields (the frontend's en.po)
// without the creation date, so extracting twice writes the same bytes.
var header = po.Header(
	[2]string{"MIME-Version", "1.0"},
	[2]string{"Content-Type", "text/plain; charset=utf-8"},
	[2]string{"Content-Transfer-Encoding", "8bit"},
	[2]string{"X-Generator", "sona i18ntool"},
	[2]string{"Language", l10n.SourceLocale},
)

// skipDirs are directories never walked: test fixtures, the frontend (its
// text is Lingui's), dependencies and the development environment.
var skipDirs = []string{"testdata", "frontend", "node_modules", "vendor"}

// message is one msgid's occurrences.
type message struct {
	refs  []string // files, slash paths relative to the root
	notes []string // translator notes from the declarations' comments
}

// ref is a name used as a message value; it must name a marked
// declaration.
type ref struct {
	dir, name string // the declaring package's directory and the name
	pos       string // file:line of the use
}

type extractor struct {
	root   string
	module string
	fset   *token.FileSet
	msgs   map[string]*message
	decls  map[string]map[string]bool // directory → names declared as Message
	refs   []ref
	errs   []error
}

// Extract reads every non-test, non-generated .go file under root (a
// module root) and returns the English catalogue of the messages they
// mark: sorted by msgid, with the files each comes from and its
// declaration's comment as a translator note, and msgstr empty.
func Extract(root string) (*po.File, error) {
	module, err := modulePath(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	x := &extractor{
		root: root, module: module, fset: token.NewFileSet(),
		msgs: map[string]*message{}, decls: map[string]map[string]bool{},
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || slices.Contains(skipDirs, name)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		return x.file(p)
	})
	if err != nil {
		return nil, err
	}
	x.checkRefs()
	if err := errors.Join(x.errs...); err != nil {
		return nil, err
	}
	return x.catalogue(), nil
}

// modulePath reads the module line of a go.mod.
func modulePath(gomod string) (string, error) {
	b, err := os.ReadFile(gomod)
	if err != nil {
		return "", err
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if m, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(m), `"`), nil
		}
	}
	return "", fmt.Errorf("%s: no module line", gomod)
}

// fileScope is what a file's marks are read against.
type fileScope struct {
	x       *extractor
	rel     string            // the file, a slash path relative to the root
	dir     string            // its directory, likewise
	inL10n  bool              // the file is package l10n itself
	l10n    string            // the name l10n is imported as ("" when not)
	dot     bool              // l10n is dot-imported
	imports map[string]string // import name → path, for references
}

func (x *extractor) file(p string) error {
	src, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	f, err := parser.ParseFile(x.fset, p, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	if ast.IsGenerated(f) {
		return nil
	}
	rel, err := filepath.Rel(x.root, p)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	s := &fileScope{x: x, rel: rel, dir: path.Dir(rel), imports: map[string]string{}}
	s.inL10n = s.dir == PackageDir
	importPath := x.module + "/" + PackageDir
	for _, im := range f.Imports {
		ip, _ := strconv.Unquote(im.Path.Value)
		name := path.Base(ip)
		if im.Name != nil {
			name = im.Name.Name
		}
		s.imports[name] = ip
		if ip == importPath {
			switch name {
			case ".":
				s.dot = true
			case "_":
			default:
				s.l10n = name
			}
		}
	}
	if s.l10n == "" && !s.dot && !s.inL10n {
		return nil
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.GenDecl:
			s.genDecl(n)
		case *ast.CallExpr:
			s.call(n)
		case *ast.CompositeLit:
			s.composite(n)
		}
		return true
	})
	return nil
}

// isMessage reports whether e is the type l10n.Message.
func (s *fileScope) isMessage(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.ParenExpr:
		return s.isMessage(e.X)
	case *ast.SelectorExpr:
		id, ok := e.X.(*ast.Ident)
		return ok && s.l10n != "" && id.Name == s.l10n && e.Sel.Name == "Message"
	case *ast.Ident:
		return (s.inL10n || s.dot) && e.Name == "Message"
	}
	return false
}

// genDecl marks the values of constants and variables declared as
// Message. A grouped declaration's specs are each their own: one without
// a type is an untyped string, not a message.
func (s *fileScope) genDecl(d *ast.GenDecl) {
	if d.Tok != token.CONST && d.Tok != token.VAR {
		return
	}
	for _, sp := range d.Specs {
		vs := sp.(*ast.ValueSpec)
		if vs.Type == nil || !s.isMessage(vs.Type) {
			continue
		}
		if s.x.decls[s.dir] == nil {
			s.x.decls[s.dir] = map[string]bool{}
		}
		for _, id := range vs.Names {
			s.x.decls[s.dir][id.Name] = true
		}
		doc := vs.Doc
		if doc == nil && !d.Lparen.IsValid() {
			doc = d.Doc
		}
		note := joinNotes(doc, vs.Comment)
		for _, v := range vs.Values {
			s.value(v, note, false)
		}
	}
}

// call marks the argument of a conversion to Message.
func (s *fileScope) call(c *ast.CallExpr) {
	if len(c.Args) != 1 || !s.isMessage(c.Fun) {
		return
	}
	// The runtime itself converts catalogue text it reads (a msgid);
	// only its constant conversions are messages.
	s.value(c.Args[0], "", s.inL10n)
}

// composite marks the values of a map, slice or array of Message.
func (s *fileScope) composite(c *ast.CompositeLit) {
	var keyed bool
	switch t := c.Type.(type) {
	case *ast.MapType:
		if !s.isMessage(t.Value) {
			return
		}
		keyed = true
	case *ast.ArrayType:
		if !s.isMessage(t.Elt) {
			return
		}
	default:
		return
	}
	for _, el := range c.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			el = kv.Value
		} else if keyed {
			continue
		}
		s.value(el, "", false)
	}
}

// value marks a message value: a constant string, or the name of a
// marked declaration. lenient skips anything else instead of failing.
func (s *fileScope) value(e ast.Expr, note string, lenient bool) {
	if text, ok := constString(e); ok {
		s.x.add(text, s.rel, note)
		return
	}
	if c, ok := e.(*ast.CallExpr); ok && s.isMessage(c.Fun) {
		return // a conversion, marked on its own (call)
	}
	switch e := e.(type) {
	case *ast.Ident:
		if !lenient {
			s.x.refs = append(s.x.refs, ref{dir: s.dir, name: e.Name, pos: s.pos(e)})
		}
		return
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			if ip, ok := s.imports[id.Name]; ok {
				if dir, ok := strings.CutPrefix(ip, s.x.module+"/"); ok {
					s.x.refs = append(s.x.refs, ref{dir: dir, name: e.Sel.Name, pos: s.pos(e)})
					return
				}
			}
		}
	}
	if !lenient {
		s.x.errs = append(s.x.errs, fmt.Errorf("%s: a message must be a string literal or a marked message's name, not %s",
			s.pos(e), exprKind(e)))
	}
}

func (s *fileScope) pos(n ast.Node) string {
	return fmt.Sprintf("%s:%d", s.rel, s.x.fset.Position(n.Pos()).Line)
}

// constString evaluates a string literal or literals joined with +.
func constString(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.ParenExpr:
		return constString(e.X)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		l, ok := constString(e.X)
		if !ok {
			return "", false
		}
		r, ok := constString(e.Y)
		return l + r, ok
	}
	return "", false
}

// exprKind names an expression for an error.
func exprKind(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.CallExpr:
		return "a call"
	case *ast.BinaryExpr:
		return "an expression (only literals may be joined with +)"
	case *ast.SelectorExpr:
		return "the selector " + e.Sel.Name
	case *ast.BasicLit:
		return "a " + strings.ToLower(e.Kind.String()) + " literal"
	}
	return fmt.Sprintf("%T", e)
}

// joinNotes is the comments' text on one line.
func joinNotes(groups ...*ast.CommentGroup) string {
	var parts []string
	for _, g := range groups {
		if g != nil {
			if t := strings.Join(strings.Fields(g.Text()), " "); t != "" {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, " ")
}

func (x *extractor) add(id, file, note string) {
	if id == "" {
		x.errs = append(x.errs, fmt.Errorf("%s: an empty message", file))
		return
	}
	m := x.msgs[id]
	if m == nil {
		m = &message{}
		x.msgs[id] = m
	}
	if !slices.Contains(m.refs, file) {
		m.refs = append(m.refs, file)
	}
	if note != "" && !slices.Contains(m.notes, note) {
		m.notes = append(m.notes, note)
	}
}

// checkRefs fails names used as messages that no marked declaration
// declares: an untyped constant there would compile but never reach the
// catalogue.
func (x *extractor) checkRefs() {
	for _, r := range x.refs {
		if !x.decls[r.dir][r.name] {
			x.errs = append(x.errs, fmt.Errorf("%s: %s is not declared as an l10n.Message, so its text is not in the catalogue", r.pos, r.name))
		}
	}
}

// catalogue is the collected messages as a PO file.
func (x *extractor) catalogue() *po.File {
	f := &po.File{Header: header}
	for id, m := range x.msgs {
		slices.Sort(m.refs)
		slices.Sort(m.notes)
		e := &po.Entry{ID: id, References: m.refs, Extracted: m.notes}
		// The verbs are fmt's: translators and their tools must keep them
		// (Printer.Sprintf fills them in, and the runtime drops a
		// translation whose verbs differ from the English).
		if len(l10n.Verbs(id)) > 0 {
			e.Flags = []string{"go-format"}
		}
		f.Entries = append(f.Entries, e)
	}
	slices.SortFunc(f.Entries, func(a, b *po.Entry) int { return strings.Compare(a.ID, b.ID) })
	return f
}

// Diff summarises how catalogue got differs from want, one line per
// message: "+" added, "-" removed, "~" changed (files, notes or flags).
// A message is its msgid and context. It is empty when they are the same.
func Diff(got, want *po.File) []string {
	have := map[po.Key]*po.Entry{}
	for _, e := range got.Entries {
		if !e.Obsolete {
			have[e.Key()] = e
		}
	}
	name := func(k po.Key) string {
		if k.Context != "" {
			return strconv.Quote(k.Context) + "/" + strconv.Quote(k.ID)
		}
		return strconv.Quote(k.ID)
	}
	var out []string
	seen := map[po.Key]bool{}
	for _, e := range want.Entries {
		seen[e.Key()] = true
		g, ok := have[e.Key()]
		switch {
		case !ok:
			out = append(out, "+ "+name(e.Key()))
		case !slices.Equal(g.References, e.References) || !slices.Equal(g.Extracted, e.Extracted) ||
			!slices.Equal(g.Translator, e.Translator) || !slices.Equal(g.Flags, e.Flags) || g.Str != e.Str:
			out = append(out, "~ "+name(e.Key()))
		}
	}
	for _, e := range got.Entries {
		if !e.Obsolete && !seen[e.Key()] {
			out = append(out, "- "+name(e.Key()))
		}
	}
	if len(out) == 0 && got.Header != want.Header {
		out = append(out, "~ the header")
	}
	return out
}
