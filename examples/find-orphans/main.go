package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"go/printer"
	"log"
	"strings"
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

	startPatterns := flag.Args()
	if len(startPatterns) == 0 {
		startPatterns = []string{"./..."}
	}

	if err := run(context.Background(), *all, *includeTests, *workspace, *verbose, startPatterns); err != nil {
		log.Fatalf("!! %+v", err)
	}
}

func run(ctx context.Context, all bool, includeTests bool, workspace string, verbose bool, startPatterns []string) error {
	var scannerOpts []goscan.ScannerOption
	scannerOpts = append(scannerOpts, goscan.WithIncludeTests(includeTests))
	scannerOpts = append(scannerOpts, goscan.WithGoModuleResolver()) // Important for resolving modules
	if verbose {
		log.SetFlags(log.Lshortfile)
	} else {
		log.SetFlags(0)
	}

	if workspace != "" {
		scannerOpts = append(scannerOpts, goscan.WithWorkDir(workspace))
	}
	s, err := goscan.New(scannerOpts...)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	a := &analyzer{
		s:        s,
		packages: make(map[string]*scanner.PackageInfo),
	}
	return a.analyze(ctx, startPatterns)
}

type analyzer struct {
	s        *goscan.Scanner
	packages map[string]*scanner.PackageInfo
	mu       sync.Mutex
}

func (a *analyzer) analyze(ctx context.Context, startPatterns []string) error {
	log.Printf("discovering packages from: %v", startPatterns)
	for _, pattern := range startPatterns {
		if err := a.s.Walker.Walk(ctx, pattern, a); err != nil {
			return fmt.Errorf("failed to walk packages from %q: %w", pattern, err)
		}
	}
	log.Printf("analyzing %d packages", len(a.packages))

	interfaceMap := buildInterfaceMap(a.packages)
	log.Printf("built interface map with %d interfaces", len(interfaceMap))

	interp, err := symgo.NewInterpreter(a.s)
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
				if fn.Decl.Recv != nil && len(fn.Decl.Recv.List) > 0 {
					var buf bytes.Buffer
					printer.Fprint(&buf, a.s.Fset(), fn.Decl.Recv.List[0].Type)
					fullName = fmt.Sprintf("(%s.%s).%s", fn.Package.ImportPath, buf.String(), fn.Name.Name)
					usageMap[fullName] = true

					if recvTypeStr := buf.String(); len(recvTypeStr) > 0 && recvTypeStr[0] == '*' {
						valueRecvName := fmt.Sprintf("(%s.%s).%s", fn.Package.ImportPath, recvTypeStr[1:], fn.Name.Name)
						usageMap[valueRecvName] = true
					}

				} else {
					fullName = fmt.Sprintf("%s.%s", fn.Package.ImportPath, fn.Name.Name)
					usageMap[fullName] = true
				}
			}
		case *object.SymbolicPlaceholder:
			if fn.UnderlyingFunc != nil && fn.Package != nil {
				fullName = getFullName(fn.Package, fn.UnderlyingFunc)
				usageMap[fullName] = true

				if fn.UnderlyingFunc.Receiver != nil {
					receiverTypeInfo := fn.UnderlyingFunc.Receiver.Type.Definition
					if receiverTypeInfo != nil && receiverTypeInfo.Kind == scanner.InterfaceKind {
						ifaceName := fmt.Sprintf("%s.%s", receiverTypeInfo.PkgPath, receiverTypeInfo.Name)
						if implementers, ok := interfaceMap[ifaceName]; ok {
							for _, impl := range implementers {
								methodName := fn.UnderlyingFunc.Name
								implPkg := a.packages[impl.PkgPath]
								if implPkg != nil {
									for _, m := range implPkg.Functions {
										if m.Name == methodName && m.Receiver != nil {
											implMethodName := getFullName(implPkg, m)
											usageMap[implMethodName] = true
										}
									}
								}
							}
						}
					}
				}
			}
		}
		return nil
	})

	log.Printf("running symbolic execution from entry points")
	// First, find all potential entry points.
	var mainEntryPoint *object.Function
	var libraryEntryPoints []*object.Function

	for _, pkg := range a.packages {
		// Load all files in the package to define all symbols in the interpreter's env
		for _, fileAst := range pkg.AstFiles {
			if _, err := interp.Eval(ctx, fileAst, pkg); err != nil {
				log.Printf("could not load package %s: %v", pkg.ImportPath, err)
				break // if one file fails, probably best to skip the whole pkg
			}
		}

		for _, fnInfo := range pkg.Functions {
			funcObj, ok := interp.FindObject(fnInfo.Name)
			if !ok {
				log.Printf("could not find function object for %s in package %s", fnInfo.Name, pkg.ImportPath)
				continue
			}
			fn, ok := funcObj.(*object.Function)
			if !ok {
				log.Printf("%s is not a function in package %s", fnInfo.Name, pkg.ImportPath)
				continue
			}

			if pkg.Name == "main" && fnInfo.Name == "main" && fnInfo.Receiver == nil {
				mainEntryPoint = fn
			} else if fnInfo.AstDecl.Name.IsExported() && fnInfo.Receiver == nil {
				libraryEntryPoints = append(libraryEntryPoints, fn)
			}
		}
	}

	// Decide which entry points to use.
	var entryPoints []*object.Function
	if mainEntryPoint != nil {
		entryPoints = []*object.Function{mainEntryPoint}
		log.Printf("found main entry point, running in application mode")
	} else {
		entryPoints = libraryEntryPoints
		log.Printf("no main entry point found, running in library mode with %d exported functions as entry points", len(entryPoints))
	}

	for _, ep := range entryPoints {
		epName := getFullName(ep.Package, &scanner.FunctionInfo{Name: ep.Name.Name})
		usageMap[epName] = true
		log.Printf("analyzing from entry point: %s", epName)
		interp.Apply(ctx, ep, []object.Object{}, ep.Package)
	}
	log.Printf("symbolic execution complete")

	fmt.Println("\n-- Orphans --")
	count := 0
	for _, pkg := range a.packages {
		for _, decl := range pkg.Functions {
			name := getFullName(pkg, decl)
			if _, used := usageMap[name]; !used {
				if decl.AstDecl.Doc != nil {
					for _, comment := range decl.AstDecl.Doc.List {
						if strings.Contains(comment.Text, "//go:scan:ignore") {
							goto nextDecl
						}
					}
				}

				pos := a.s.Fset().Position(decl.AstDecl.Pos())
				fmt.Printf("%s\n  %s\n", name, pos)
				count++
			}
		nextDecl:
		}
	}

	if count == 0 {
		fmt.Println("No orphans found.")
	}

	return nil
}

func (a *analyzer) Visit(pkg *goscan.PackageImports) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.packages[pkg.ImportPath]; exists {
		return nil, nil
	}
	if pkg.ImportPath == "C" {
		return nil, nil
	}
	fullPkg, err := a.s.ScanPackageByImport(context.Background(), pkg.ImportPath)
	if err != nil {
		log.Printf("warning: could not scan package %s: %v", pkg.ImportPath, err)
		return nil, nil // Continue even if a package fails to scan
	}
	a.packages[pkg.ImportPath] = fullPkg

	// Filter out stdlib and C pseudo-packages to avoid trying to scan them.
	var importsToFollow []string
	for _, imp := range pkg.Imports {
		if strings.Contains(imp, ".") {
			importsToFollow = append(importsToFollow, imp)
		}
	}
	return importsToFollow, nil
}

func getFullName(pkg *scanner.PackageInfo, fn *scanner.FunctionInfo) string {
	if fn.Receiver != nil {
		recvTypeStr := fn.Receiver.Type.String()
		recvTypeStr = strings.TrimPrefix(recvTypeStr, "*") // remove pointer
		return fmt.Sprintf("(%s.%s).%s", pkg.ImportPath, recvTypeStr, fn.Name)
	}
	return fmt.Sprintf("%s.%s", pkg.ImportPath, fn.Name)
}

func buildInterfaceMap(packages map[string]*scanner.PackageInfo) map[string][]*scanner.TypeInfo {
	interfaceMap := make(map[string][]*scanner.TypeInfo)
	var allInterfaces []*scanner.TypeInfo
	var allStructs []*scanner.TypeInfo
	packageOfStruct := make(map[*scanner.TypeInfo]*scanner.PackageInfo)

	for _, pkg := range packages {
		for _, t := range pkg.Types {
			if t.Kind == scanner.InterfaceKind {
				allInterfaces = append(allInterfaces, t)
			} else if t.Kind == scanner.StructKind {
				allStructs = append(allStructs, t)
				packageOfStruct[t] = pkg
			}
		}
	}

	for _, iface := range allInterfaces {
		ifaceName := fmt.Sprintf("%s.%s", iface.PkgPath, iface.Name)
		var implementers []*scanner.TypeInfo

		for _, strct := range allStructs {
			pkgInfo := packageOfStruct[strct]
			if goscan.Implements(strct, iface, pkgInfo) {
				implementers = append(implementers, strct)
			}
		}
		if len(implementers) > 0 {
			interfaceMap[ifaceName] = implementers
		}
	}

	return interfaceMap
}
