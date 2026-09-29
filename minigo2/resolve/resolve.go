// Package resolve maps import paths to package directories and file lists,
// without expanding the import graph. The default backend adapts go-scan's
// lazy locator; a `go list -find` backend may be added as an opt-in.
package resolve

import (
	"context"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BuildConfig selects which files belong to a package build.
type BuildConfig struct {
	GOOS   string
	GOARCH string
	Tags   []string
}

// PackageMeta is the cheap metadata level of a package — enough to know its
// real name and which files belong to it, before any AST is built.
type PackageMeta struct {
	ImportPath string
	Name       string // the package clause name (may differ from path basename)
	Dir        string
	GoFiles    []string // absolute paths, filtered by build constraints
	Standard   bool     // inside GOROOT
	ModulePath string   // owning module path, "" if unknown
}

// Resolver locates a package. It must NOT recursively resolve the package's
// own imports — that is the point of the lazy design.
type Resolver interface {
	// Locate resolves an import path (e.g. "fmt", "github.com/x/y").
	Locate(ctx context.Context, fromDir, importPath string) (*PackageMeta, error)
	// LocateDir resolves a filesystem directory (e.g. "./app") — used for
	// the entry package, which need not have an import path.
	LocateDir(ctx context.Context, dir string) (*PackageMeta, error)
}

// ReadPackageFiles reads a directory into a PackageMeta: it filters files
// with go/build's match rules (//go:build constraints, _GOOS/_GOARCH
// suffixes, _test.go) and reads the package clause of the first match.
func ReadPackageFiles(dir, importPath string, cfg BuildConfig) (*PackageMeta, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading package dir %s: %w", dir, err)
	}
	ctx := build.Default
	if cfg.GOOS != "" {
		ctx.GOOS = cfg.GOOS
	}
	if cfg.GOARCH != "" {
		ctx.GOARCH = cfg.GOARCH
	}
	ctx.BuildTags = append(ctx.BuildTags, cfg.Tags...)

	var files []string
	var name string
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		match, err := ctx.MatchFile(dir, e.Name())
		if err != nil || !match {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
		if name == "" {
			f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.PackageClauseOnly)
			if err == nil && f != nil {
				name = f.Name.Name
			}
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no buildable Go source files in %s", dir)
	}
	sort.Strings(files)
	return &PackageMeta{
		ImportPath: importPath,
		Name:       name,
		Dir:        dir,
		GoFiles:    files,
		Standard:   strings.HasPrefix(dir, build.Default.GOROOT),
	}, nil
}

// CheckImportPath sanity-checks a path that claims to be an import path.
func LooksLikeDir(path string) bool {
	return strings.HasPrefix(path, ".") || filepath.IsAbs(path)
}

var _ = ast.IsExported // keep go/ast imported for future TypeRef work
