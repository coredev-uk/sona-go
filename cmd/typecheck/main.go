// Command typecheck type-checks every Sona package, tests included, for
// another OS from source with cgo off, so a macOS or Windows break shows
// up on Linux (CI runs it; see CONTRIBUTING.md, "Platforms").
//
// `GOOS=windows CGO_ENABLED=0 go vet ./...` covers Windows; typecheck does
// the same for macOS, where go vet would stop at the first dependency that
// needs cgo there. MyGo is pure Go on macOS, so only cgo-bound third-party
// code can fail: typecheck reports the type errors in this module's
// packages and only names the third-party packages that failed.
//
//	go run ./cmd/typecheck -os darwin -arch arm64 ./...
//
// With -cgo it type-checks with cgo on, through the C compiler in $CC and
// $CXX, which must target that OS: this covers the code that builds only
// with cgo there (the macOS CDM host engine; CONTRIBUTING.md has the zig
// recipe). Third-party Objective-C that needs Apple's SDK still fails and is
// named as third-party.
package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

const module = "github.com/coredev-uk/sona-go"

func main() {
	goos := flag.String("os", "darwin", "target GOOS")
	goarch := flag.String("arch", "arm64", "target GOARCH")
	cgo := flag.Bool("cgo", false, "type-check with cgo on, compiling C with $CC/$CXX (a cross compiler for the target)")
	flag.Parse()
	patterns := flag.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	os.Exit(run(*goos, *goarch, *cgo, patterns))
}

func run(goos, goarch string, cgo bool, patterns []string) int {
	target := goos + "/" + goarch
	cgoEnabled := "CGO_ENABLED=0"
	if cgo {
		target += " (cgo)"
		cgoEnabled = "CGO_ENABLED=1"
	}
	cfg := &packages.Config{
		// Syntax and types for every dependency: type-check from source
		// rather than from export data, which needs a successful build.
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports | packages.NeedDeps |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		Env:   append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, cgoEnabled),
		Tests: true,
	}
	roots, err := packages.Load(cfg, patterns...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "typecheck %s: %v\n", target, err)
		return 2
	}
	var (
		ours, checked []string
		seen          = map[string]bool{}
		thirdParty    = map[string]int{}
	)
	packages.Visit(roots, nil, func(p *packages.Package) {
		mine := p.PkgPath == module || strings.HasPrefix(p.PkgPath, module+"/")
		if mine {
			checked = append(checked, p.ID)
		}
		for _, e := range p.Errors {
			switch {
			case !mine:
				thirdParty[p.PkgPath]++
			case !seen[e.Error()]:
				seen[e.Error()] = true
				ours = append(ours, e.Error())
			}
		}
	})
	if len(thirdParty) > 0 {
		paths := make([]string, 0, len(thirdParty))
		for p := range thirdParty {
			paths = append(paths, p)
		}
		slices.Sort(paths)
		why := "without cgo"
		if cgo {
			why = "without the target's SDK"
		}
		fmt.Printf("typecheck %s: third-party packages that don't type-check %s (not Sona's to fix):\n", target, why)
		for _, p := range paths {
			fmt.Printf("  %s (%d errors)\n", p, thirdParty[p])
		}
	}
	if len(ours) > 0 {
		slices.Sort(ours)
		for _, e := range ours {
			fmt.Println(e)
		}
		fmt.Printf("typecheck %s: %d errors in Sona packages\n", target, len(ours))
		return 1
	}
	fmt.Printf("typecheck %s: %d Sona packages (with test variants) type-check\n", target, len(checked))
	return 0
}
