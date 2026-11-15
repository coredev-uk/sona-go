package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// Titles in the interface are in Title Case in Apple's style ("Made for
// You", "Top Picks for You"): every word starts with a capital except the
// small words below, which stay lower case unless first or last
// (docs/design.md, "Wording and case"). Buttons, menu items, body text,
// hints, toasts and errors stay in sentence case, so a title is recognised
// by where it is put, not by its text: titleSinks and titleFields list the
// calls and the struct fields that take one.

// smallWords are the words of a title that are not capitalised in the
// middle of it.
var smallWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "but": true, "or": true, "for": true,
	"of": true, "on": true, "in": true, "to": true, "by": true, "at": true,
}

// titleSinks are the functions and methods (by name) whose listed
// arguments are titles: page headers, shelf and section headings, sidebar
// sections, step and dialog headings.
var titleSinks = map[string][]int{
	"PageHeader": {1}, "PageHeaderCounted": {1}, "CoverShelf": {2}, "coverShelf": {2},
	"ItemShelf": {2}, "itemShelf": {2, 3}, "shelf": {2}, "section": {1, 2}, "band": {2},
	"bandHeading": {1}, "headingTitle": {1}, "sideHeading": {1}, "readOnlySection": {2},
	"queueHead": {3}, "ruler": {2}, "stepGroup": {2}, "stepHead": {2}, "groupBlock": {3},
	"itemBlock": {4}, "group": {2}, "heading": {1}, "picker": {2}, "dialogHeader": {1},
	"flowTitle": {1}, "masthead": {1},
}

// titleFields are the struct fields (by type name) that are titles. A
// field also takes a function returning the title (a route's Title).
var titleFields = map[string][]string{
	"ShelfOpts": {"Title"}, "ruledOpts": {"Title"}, "DialogOpts": {"Title"},
	"ConfirmOptions": {"Title"}, "PaletteSection": {"Title"}, "headOpts": {"title"},
	"Route": {"Title"}, "NavItem": {"Label"},
}

// isTitleCase reports whether s is in Title Case: its first and last words
// start with a capital and so does every other word that is not a small
// word. A small word may still be capitalised ("Finish Signing In in Your
// Browser"). Words with a digit, a placeholder or a capital inside (EPs,
// DJ, {name}) are left alone.
func isTitleCase(s string) bool {
	words := strings.Fields(s)
	for i, w := range words {
		if strings.ContainsAny(w, "{<0123456789") {
			continue
		}
		r := []rune(strings.TrimLeftFunc(w, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }))
		if len(r) == 0 || !unicode.IsLower(r[0]) {
			continue
		}
		lower := strings.ToLower(strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) }))
		if smallWords[lower] && i > 0 && i < len(words)-1 {
			continue
		}
		return false
	}
	return true
}

// titleLiteral returns the first message of an i18n.T or i18n.TC call in e,
// with its position, if any.
func titleLiteral(fset *token.FileSet, e ast.Expr) (string, token.Position, bool) {
	var msg string
	var pos token.Position
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "T" && sel.Sel.Name != "TC") {
			return true
		}
		if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "i18n" {
			return true
		}
		i := 0
		if sel.Sel.Name == "TC" {
			i = 1
		}
		if len(call.Args) <= i {
			return true
		}
		if lit, ok := call.Args[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if s, err := strconv.Unquote(lit.Value); err == nil {
				msg, pos, found = s, fset.Position(lit.Pos()), true
			}
		}
		return false
	})
	return msg, pos, found
}

// calleeName is the name a call is made by (f or x.f).
func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	case *ast.IndexExpr:
		return calleeName(f.X)
	}
	return ""
}

// typeName is the name of a composite literal's type (T, pkg.T).
func typeName(t ast.Expr) string {
	switch x := t.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}

// checkTitles reads the Go files under root's internal/ui (syntax only,
// tests skipped) and lists, one line each, every title that is not in
// Title Case.
func checkTitles(root string) ([]string, error) {
	var bad []string
	fset := token.NewFileSet()
	uiRoot := filepath.Join(root, filepath.FromSlash(uiDir))
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
		check := func(e ast.Expr) {
			msg, pos, ok := titleLiteral(fset, e)
			if ok && !isTitleCase(msg) {
				rel, _ := filepath.Rel(root, pos.Filename)
				bad = append(bad, fmt.Sprintf("%s:%d: %q", filepath.ToSlash(rel), pos.Line, msg))
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				for _, i := range titleSinks[calleeName(n.Fun)] {
					if i < len(n.Args) {
						check(n.Args[i])
					}
				}
			case *ast.CompositeLit:
				fields := titleFields[typeName(n.Type)]
				for _, el := range n.Elts {
					kv, ok := el.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if k, ok := kv.Key.(*ast.Ident); ok {
						for _, name := range fields {
							if k.Name == name {
								check(kv.Value)
							}
						}
					}
				}
			}
			return true
		})
		return nil
	})
	return bad, err
}
