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

	interfaceMap := buildInterfaceMap(visitor.packages)
	log.Printf("built interface map with %d interfaces", len(interfaceMap))

	innerScanner, err := s.ScannerForSymgo()
	if err != nil {
		return fmt.Errorf("failed to get inner scanner: %w", err)
	}
	interp, err := symgo.NewInterpreter(innerScanner)
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
					printer.Fprint(&buf, s.Fset(), fn.Decl.Recv.List[0].Type)
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
								implPkg := visitor.packages[impl.PkgPath]
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
	var entryPoints []*object.Function
	for _, pkg := range visitor.packages {
		if pkg.Name == "main" {
			for _, fnInfo := range pkg.Functions {
				if fnInfo.Name == "main" && fnInfo.Receiver == nil {
					fileAst, ok := pkg.AstFiles[fnInfo.FilePath]
					if !ok {
						log.Printf("could not find ast file for %s", fnInfo.FilePath)
						continue
					}
					if _, err := interp.Eval(ctx, fileAst, pkg); err != nil {
						log.Printf("could not load package %s: %v", pkg.ImportPath, err)
						continue
					}

					mainFuncObj, ok := interp.FindObject(fnInfo.Name)
					if !ok {
						log.Printf("could not find main function in package %s", pkg.ImportPath)
						continue
					}
					mainFunc, ok := mainFuncObj.(*object.Function)
					if !ok {
						log.Printf("main is not a function in package %s", pkg.ImportPath)
						continue
					}
					entryPoints = append(entryPoints, mainFunc)
				}
			}
		}
	}

	if len(entryPoints) == 0 {
		log.Printf("no main entry point found, analysis may be incomplete")
	}

	for _, ep := range entryPoints {
		log.Printf("analyzing from entry point: %s.%s", ep.Package.ImportPath, ep.Name.Name)
		interp.Apply(ctx, ep, []object.Object{}, ep.Package)
	}
	log.Printf("symbolic execution complete")

	fmt.Println("\n-- Orphans --")
	count := 0
	for _, pkg := range visitor.packages {
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

				pos := s.Fset().Position(decl.AstDecl.Pos())
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
		recvTypeStr := fn.Receiver.Type.String()
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
