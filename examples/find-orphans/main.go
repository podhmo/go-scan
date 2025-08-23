package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"go/printer"
	"log"
	"sync"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func main() {
	var (
		all          = flag.Bool("all", false, "scan every package in the module")
		includeTests = flag.Bool("include-tests", false, "include usage within test files")
		workspace    = flag.String("workspace-root", "", "scan all Go modules found under a given directory")
		verbose      = flag.Bool("v", false, "enable verbose output")
	)
	flag.Parse()

	if err := run(context.Background(), *all, *includeTests, *workspace, *verbose); err != nil {
		log.Fatalf("!! %+v", err)
	}
}

func run(ctx context.Context, all bool, includeTests bool, workspace string, verbose bool) error {
	var scannerOpts []goscan.ScannerOption
	scannerOpts = append(scannerOpts, goscan.WithIncludeTests(includeTests))
	if verbose {
		log.SetFlags(log.Lshortfile)
	} else {
		log.SetFlags(0)
	}

	s, err := goscan.New(scannerOpts...)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	var startPatterns []string
	if flag.NArg() > 0 {
		startPatterns = flag.Args()
	} else if workspace != "" {
		startPatterns = []string{workspace}
	} else {
		startPatterns = []string{"."}
	}

	log.Printf("discovering packages from: %v", startPatterns)
	visitor := &collectorVisitor{
		s:        s,
		packages: make(map[string]*scanner.PackageInfo),
	}
	for _, pattern := range startPatterns {
		if err := s.Walker.Walk(ctx, pattern, visitor); err != nil {
			return fmt.Errorf("failed to walk packages from %q: %w", pattern, err)
		}
	}
	log.Printf("discovered %d packages", len(visitor.packages))

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		return fmt.Errorf("failed to create interpreter: %w", err)
	}

	usageMap := make(map[string]bool)
	interp.RegisterDefaultIntrinsic(func(i *symgo.Interpreter, args []object.Object) object.Object {
		if len(args) == 0 {
			return nil
		}
		fnObj := args[0]
		var fullName string

		switch fn := fnObj.(type) {
		case *object.Function:
			if fn.Package != nil && fn.Name != nil {
				if fn.Decl != nil && fn.Decl.Recv != nil && len(fn.Decl.Recv.List) > 0 {
					var buf bytes.Buffer
					printer.Fprint(&buf, s.Fset(), fn.Decl.Recv.List[0].Type)
					fullName = fmt.Sprintf("(%s.%s).%s", fn.Package.ImportPath, buf.String(), fn.Name.Name)
				} else {
					fullName = fmt.Sprintf("%s.%s", fn.Package.ImportPath, fn.Name.Name)
				}
			}
		case *object.SymbolicPlaceholder:
			if fn.UnderlyingFunc != nil && fn.Package != nil {
				fullName = getFullName(fn.Package, fn.UnderlyingFunc)
			}
		}

		if fullName != "" {
			usageMap[fullName] = true
		}
		return nil
	})

	log.Printf("running symbolic execution")
	for _, pkg := range visitor.packages {
		for _, decl := range pkg.Functions {
			if decl.AstDecl.Body == nil {
				continue
			}
			_, err := interp.Eval(ctx, decl.AstDecl, pkg)
			if err != nil {
				// log.Printf("error evaluating %s: %v", getFullName(pkg, decl), err)
			}
		}
	}
	log.Printf("symbolic execution complete")

	fmt.Println("\n-- Orphans --")
	count := 0
	for _, pkg := range visitor.packages {
		for _, decl := range pkg.Functions {
			name := getFullName(pkg, decl)
			if _, used := usageMap[name]; !used {
				if decl.Receiver != nil {
					continue
				}
				pos := s.Fset().Position(decl.AstDecl.Pos())
				fmt.Printf("%s\n  %s\n", name, pos)
				count++
			}
		}
	}

	if count == 0 {
		fmt.Println("No orphans found.")
	}

	return nil
}

type collectorVisitor struct {
	s        *goscan.Scanner
	packages map[string]*scanner.PackageInfo
	mu       sync.Mutex
}

func (v *collectorVisitor) Visit(pkg *goscan.PackageImports) ([]string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, exists := v.packages[pkg.ImportPath]; exists {
		return nil, nil
	}
	fullPkg, err := v.s.ScanPackageByImport(context.Background(), pkg.ImportPath)
	if err != nil {
		log.Printf("warning: could not scan package %s: %v", pkg.ImportPath, err)
		return nil, nil
	}
	v.packages[pkg.ImportPath] = fullPkg
	return pkg.Imports, nil
}

func getFullName(pkg *scanner.PackageInfo, fn *scanner.FunctionInfo) string {
	if fn.Receiver != nil {
		// Use the String() method on FieldType which is designed for this.
		recvTypeStr := fn.Receiver.Type.String()
		return fmt.Sprintf("(%s.%s).%s", pkg.ImportPath, recvTypeStr, fn.Name)
	}
	return fmt.Sprintf("%s.%s", pkg.ImportPath, fn.Name)
}
