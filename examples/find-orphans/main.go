package main

import (
	"context"
	"flag"
	"fmt"
	"go/ast"
	"log"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

func main() {
	var (
		includeTests = flag.Bool("include-tests", false, "include usage within test files")
		workspace    = flag.String("workspace-root", "", "scan all Go modules found under a given directory")
		verbose      = flag.Bool("v", false, "enable verbose output")
		debug        = flag.Bool("debug", false, "enable debug logging")
	)
	flag.Parse()

	var startPatterns []string
	if flag.NArg() > 0 {
		startPatterns = flag.Args()
	} else {
		startPatterns = []string{"./..."}
	}

	if err := run(context.Background(), startPatterns, *workspace, *includeTests, *verbose, *debug); err != nil {
		log.Fatalf("!! %+v", err)
	}
}

// idFromFuncDecl generates a unique identifier for a function declaration.
func idFromFuncDecl(pkg *goscan.Package, decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return fmt.Sprintf("%s.%s", pkg.ImportPath, decl.Name.Name)
	}
	var recvType string
	switch t := decl.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			recvType = fmt.Sprintf("*%s", ident.Name)
		}
	case *ast.Ident:
		recvType = t.Name
	}
	if recvType == "" {
		return "" // Could not determine receiver type
	}
	return fmt.Sprintf("(%s.%s).%s", pkg.ImportPath, recvType, decl.Name.Name)
}

// resolveCallExpr attempts to find the fully qualified name of a function being called.
// This is a simplified implementation and has limitations.
func resolveCallExpr(pkg *goscan.Package, file *ast.File, call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		// Simple call in the same package, e.g., `myFunc()`
		return fmt.Sprintf("%s.%s", pkg.ImportPath, fun.Name)
	case *ast.SelectorExpr:
		// e.g., `fmt.Println()` or `myVar.myMethod()`
		if pkgIdent, ok := fun.X.(*ast.Ident); ok {
			// Find the import path for the package identifier `pkgIdent.Name`
			for _, imp := range file.Imports {
				var importName string
				if imp.Name != nil {
					importName = imp.Name.Name
				} else {
					path := strings.Trim(imp.Path.Value, `"`)
					importName = path[strings.LastIndex(path, "/")+1:]
				}
				if pkgIdent.Name == importName {
					importPath := strings.Trim(imp.Path.Value, `"`)
					return fmt.Sprintf("%s.%s", importPath, fun.Sel.Name)
				}
			}
		}
	}
	return ""
}

func run(ctx context.Context, patterns []string, workspace string, includeTests bool, verbose bool, debug bool) error {
	var scannerOpts []goscan.ScannerOption
	scannerOpts = append(scannerOpts, goscan.WithIncludeTests(includeTests))
	if workspace != "" {
		scannerOpts = append(scannerOpts, goscan.WithWorkDir(workspace))
	}

	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
	scannerOpts = append(scannerOpts, goscan.WithLogger(logger))

	s, err := goscan.New(scannerOpts...)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	pkgs, err := discoverPackages(ctx, s, patterns)
	if err != nil {
		return err
	}

	declarations := make(map[string]*ast.FuncDecl)
	usages := make(map[string]bool)

	for _, pkg := range pkgs {
		for _, file := range pkg.AstFiles {
			ast.Inspect(file, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.FuncDecl:
					id := idFromFuncDecl(pkg, node)
					if id != "" {
						declarations[id] = node
					}
				case *ast.CallExpr:
					id := resolveCallExpr(pkg, file, node)
					if id != "" {
						usages[id] = true
					}
				}
				return true
			})
		}
	}

	logger.Info("Analysis complete", "declarations", len(declarations), "usages", len(usages))

	// Main analysis and reporting
	var orphanNames []string
	for id, decl := range declarations {
		if decl.Name.Name == "main" && pkgNameFromID(id) == "main" {
			continue
		}

		if _, used := usages[id]; !used {
			if decl.Doc != nil {
				for _, comment := range decl.Doc.List {
					if strings.Contains(comment.Text, "//go:scan:ignore") {
						goto nextDecl
					}
				}
			}
			orphanNames = append(orphanNames, id)
		}
	nextDecl:
	}
	sort.Strings(orphanNames)

	if verbose {
		fmt.Println("\n-- Used Functions --")
		var usedNames []string
		for id := range usages {
			usedNames = append(usedNames, id)
		}
		sort.Strings(usedNames)
		for _, name := range usedNames {
			fmt.Println(name)
		}
	}

	fmt.Println("\n-- Orphans --")
	if len(orphanNames) == 0 {
		fmt.Println("No orphans found.")
	} else {
		for _, name := range orphanNames {
			decl := declarations[name]
			pos := s.Fset().Position(decl.Pos())
			fmt.Printf("%s\n  %s\n", name, pos)
		}
	}

	return nil
}

type collectorVisitor struct {
	s        *goscan.Scanner
	pkgs     map[string]*goscan.Package
	mu       sync.Mutex
	ctx      context.Context
	logger   *slog.Logger
}

func (v *collectorVisitor) Visit(pkgImports *scanner.PackageImports) ([]string, error) {
	fullPkg, err := v.s.ScanPackageByImport(v.ctx, pkgImports.ImportPath)
	if err != nil {
		v.logger.Warn("could not scan package", "path", pkgImports.ImportPath, "error", err)
		return nil, nil
	}
	v.mu.Lock()
	v.pkgs[fullPkg.ImportPath] = fullPkg
	v.mu.Unlock()
	return pkgImports.Imports, nil
}

func discoverPackages(ctx context.Context, s *goscan.Scanner, patterns []string) (map[string]*goscan.Package, error) {
	visitor := &collectorVisitor{
		s:      s,
		pkgs:   make(map[string]*goscan.Package),
		ctx:    ctx,
		logger: s.Logger,
	}

	for _, pattern := range patterns {
		if err := s.Walker.Walk(ctx, pattern, visitor); err != nil {
			return nil, err
		}
	}
	return visitor.pkgs, nil
}

func pkgNameFromID(id string) string {
	parts := strings.Split(id, ".")
	if len(parts) < 2 {
		return ""
	}
	pkgPathParts := strings.Split(parts[0], "/")
	return pkgPathParts[len(pkgPathParts)-1]
}
