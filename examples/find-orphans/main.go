package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/printer"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"golang.org/x/mod/modfile"
)

func main() {
	var (
		all          = flag.Bool("all", false, "scan every package in the module")
		includeTests = flag.Bool("include-tests", false, "include usage within test files")
		workspace    = flag.String("workspace-root", "", "scan all Go modules found under a given directory")
		verbose      = flag.Bool("v", false, "enable verbose output")
		asJSON       = flag.Bool("json", false, "output orphans in JSON format")
	)
	flag.Parse()

	startPatterns := flag.Args()
	if len(startPatterns) == 0 {
		startPatterns = []string{"./..."}
	}

	if err := run(context.Background(), *all, *includeTests, *workspace, *verbose, *asJSON, startPatterns); err != nil {
		log.Fatalf("!! %+v", err)
	}
}

// discoverModules finds all Go modules under the given root directory.
// It prioritizes a go.work file if it exists, otherwise it scans for go.mod files.
func discoverModules(root string) ([]string, error) {
	workFilePath := filepath.Join(root, "go.work")

	// Check if go.work exists
	if _, err := os.Stat(workFilePath); err == nil {
		// go.work exists, so parse it
		data, err := os.ReadFile(workFilePath)
		if err != nil {
			return nil, fmt.Errorf("could not read go.work file at %s: %w", workFilePath, err)
		}
		wf, err := modfile.ParseWork(workFilePath, data, nil)
		if err != nil {
			return nil, fmt.Errorf("could not parse go.work file: %w", err)
		}

		var modules []string
		for _, use := range wf.Use {
			// The path in go.work is relative to the root of the go.work file.
			modulePath := filepath.Join(root, use.Path)
			modules = append(modules, modulePath)
		}
		log.Printf("discovered modules from go.work: %v", modules)
		return modules, nil
	} else if !os.IsNotExist(err) {
		// Another error occurred with os.Stat, which is unexpected.
		return nil, fmt.Errorf("failed to stat go.work file at %s: %w", workFilePath, err)
	}

	// go.work does not exist, fall back to scanning for go.mod files.
	log.Printf("no go.work file found, falling back to go.mod scan in %s", root)
	var modules []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "vendor" || (len(d.Name()) > 1 && d.Name()[0] == '.')) {
			return filepath.SkipDir
		}
		if d.Name() == "go.mod" {
			modules = append(modules, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk workspace root %s: %w", root, err)
	}
	return modules, nil
}

func run(ctx context.Context, all bool, includeTests bool, workspace string, verbose bool, asJSON bool, startPatterns []string) error {
	var scannerOpts []goscan.ScannerOption
	scannerOpts = append(scannerOpts, goscan.WithIncludeTests(includeTests))
	scannerOpts = append(scannerOpts, goscan.WithGoModuleResolver()) // Important for resolving modules
	if verbose {
		log.SetFlags(log.Lshortfile)
	} else {
		log.SetFlags(0)
	}

	var s *goscan.Scanner
	var err error

	if workspace != "" {
		moduleDirs, err := discoverModules(workspace)
		if err != nil {
			return err
		}
		if len(moduleDirs) == 0 {
			return fmt.Errorf("no go.mod files found in workspace root %s", workspace)
		}
		log.Printf("found %d modules in workspace: %v", len(moduleDirs), moduleDirs)
		scannerOpts = append(scannerOpts, goscan.WithModuleDirs(moduleDirs))
		s, err = goscan.New(scannerOpts...)
	} else {
		// For single module mode, we need to specify the workdir.
		// The startPatterns are often relative (e.g., ./...), so we need a base.
		// If no patterns are given, it defaults to "./...", so CWD is a safe bet.
		scannerOpts = append(scannerOpts, goscan.WithWorkDir("."))
		s, err = goscan.New(scannerOpts...)
	}

	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	a := &analyzer{
		s:        s,
		packages: make(map[string]*scanner.PackageInfo),
	}
	return a.analyze(ctx, asJSON, startPatterns)
}

type analyzer struct {
	s        *goscan.Scanner
	packages map[string]*scanner.PackageInfo
	mu       sync.Mutex
}

func (a *analyzer) analyze(ctx context.Context, asJSON bool, startPatterns []string) error {
	var patternsToWalk []string
	if a.s.IsWorkspace() {
		roots := a.s.ModuleRoots()
		log.Printf("discovering packages from workspace roots: %v", roots)
		for _, root := range roots {
			for _, pattern := range startPatterns {
				// a bit of a hack to check for relative patterns like './...'
				if strings.HasPrefix(pattern, ".") {
					patternsToWalk = append(patternsToWalk, filepath.Join(root, pattern))
				} else {
					// assume it's a full import path pattern
					patternsToWalk = append(patternsToWalk, pattern)
				}
			}
		}
	} else {
		patternsToWalk = startPatterns
	}

	log.Printf("walking with patterns: %v", patternsToWalk)
	if err := a.s.Walker.Walk(ctx, a, patternsToWalk...); err != nil {
		return fmt.Errorf("failed to walk packages: %w", err)
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
			// Handle interface method calls
			if fn.UnderlyingMethod != nil {
				methodName := fn.UnderlyingMethod.Name
				var implementerTypes []*scanner.FieldType

				// Always use the interface map for a conservative analysis.
				// This ensures that if an interface method is used, all possible implementations are considered "used".
				if fn.Receiver != nil {
					receiverTypeInfo := fn.Receiver.TypeInfo()
					if receiverTypeInfo != nil && receiverTypeInfo.Kind == scanner.InterfaceKind {
						ifaceName := fmt.Sprintf("%s.%s", receiverTypeInfo.PkgPath, receiverTypeInfo.Name)
						if allImplementers, ok := interfaceMap[ifaceName]; ok {
							// Convert TypeInfo to FieldType for consistency
							for _, ti := range allImplementers {
								implementerTypes = append(implementerTypes, &scanner.FieldType{Definition: ti})
							}
						}
					}
				}

				for _, implFt := range implementerTypes {
					a.markMethodAsUsed(ctx, usageMap, implFt, methodName)
				}
			}

			// Handle regular function calls (non-methods)
			if fn.UnderlyingFunc != nil && fn.Package != nil && fn.UnderlyingFunc.Receiver == nil {
				fullName := fmt.Sprintf("%s.%s", fn.Package.ImportPath, fn.UnderlyingFunc.Name)
				usageMap[fullName] = true
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
		epName := getFullName(a.s, ep.Package, &scanner.FunctionInfo{Name: ep.Name.Name, AstDecl: ep.Decl})
		// In application mode, main is the only entry point we mark as used by default.
		// In library mode, we don't mark any entry points as used by default.
		// They are only "used" if called by another entry point.
		if mainEntryPoint != nil {
			usageMap[epName] = true
		}
		log.Printf("analyzing from entry point: %s", epName)
		interp.Apply(ctx, ep, []object.Object{}, ep.Package)
	}
	log.Printf("symbolic execution complete")

	type Orphan struct {
		Name     string `json:"name"`
		Position string `json:"position"`
		Package  string `json:"package"`
	}
	var orphans []Orphan

	for _, pkg := range a.packages {
		for _, decl := range pkg.Functions {
			name := getFullName(a.s, pkg, decl)
			if _, used := usageMap[name]; !used {
				// If a method is on a pointer receiver, a call to it might have been marked
				// against the value receiver type. Let's check for that possibility.
				if decl.Receiver != nil {
					var buf bytes.Buffer
					printer.Fprint(&buf, a.s.Fset(), decl.AstDecl.Recv.List[0].Type)
					if recvTypeStr := buf.String(); len(recvTypeStr) > 0 && recvTypeStr[0] == '*' {
						valueRecvName := fmt.Sprintf("(%s.%s).%s", pkg.ImportPath, recvTypeStr[1:], decl.Name)
						if _, usedValue := usageMap[valueRecvName]; usedValue {
							continue // It was used, just via the value type.
						}
					}
				}

				if decl.AstDecl.Doc != nil {
					for _, comment := range decl.AstDecl.Doc.List {
						if strings.Contains(comment.Text, "//go:scan:ignore") {
							goto nextDecl
						}
					}
				}
				pos := a.s.Fset().Position(decl.AstDecl.Pos())
				orphans = append(orphans, Orphan{
					Name:     name,
					Position: pos.String(),
					Package:  pkg.ImportPath,
				})
			}
		nextDecl:
		}
	}

	if asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(orphans); err != nil {
			return fmt.Errorf("failed to encode orphans to JSON: %w", err)
		}
	} else {
		if len(orphans) == 0 {
			fmt.Println("No orphans found.")
			return nil
		}
		fmt.Println("\n-- Orphans --")
		for _, o := range orphans {
			fmt.Printf("%s\n  %s\n", o.Name, o.Position)
		}
	}

	return nil
}

func (a *analyzer) markMethodAsUsed(ctx context.Context, usageMap map[string]bool, implFt *scanner.FieldType, methodName string) {
	typeInfo, err := implFt.Resolve(ctx)
	if err != nil || typeInfo == nil {
		return // Cannot resolve the type, so cannot mark its methods.
	}

	implPkg, ok := a.packages[typeInfo.PkgPath]
	if !ok {
		return
	}

	// Find the concrete method on the implementing type
	for _, m := range implPkg.Functions { // m is *scanner.FunctionInfo
		if m.Name == methodName && m.Receiver != nil {
			// Check if the receiver of the method `m` matches the type `typeInfo`.
			if m.Receiver.Type.Name == typeInfo.Name || (m.Receiver.Type.IsPointer && m.Receiver.Type.Elem.Name == typeInfo.Name) {
				// Found the method. Mark it as used.
				var buf bytes.Buffer
				printer.Fprint(&buf, a.s.Fset(), m.AstDecl.Recv.List[0].Type)
				methodFullName := fmt.Sprintf("(%s.%s).%s", implPkg.ImportPath, buf.String(), m.Name)
				usageMap[methodFullName] = true

				// Also mark the value-receiver form if the concrete method has a pointer receiver
				if recvTypeStr := buf.String(); len(recvTypeStr) > 0 && recvTypeStr[0] == '*' {
					valueRecvName := fmt.Sprintf("(%s.%s).%s", implPkg.ImportPath, recvTypeStr[1:], m.Name)
					usageMap[valueRecvName] = true
				}
				break
			}
		}
	}
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

func getFullName(s *goscan.Scanner, pkg *scanner.PackageInfo, fn *scanner.FunctionInfo) string {
	if fn.Receiver != nil {
		var buf bytes.Buffer
		// fn.AstDecl can be nil for functions without bodies (like in interfaces)
		// or for entry points where we create a synthetic FunctionInfo.
		if fn.AstDecl == nil || fn.AstDecl.Recv == nil || len(fn.AstDecl.Recv.List) == 0 {
			// Fallback for safety, using the less precise type string.
			// This is important for analyzing entry points that are just names.
			return fmt.Sprintf("(%s.%s).%s", pkg.ImportPath, fn.Receiver.Type.String(), fn.Name)
		}
		printer.Fprint(&buf, s.Fset(), fn.AstDecl.Recv.List[0].Type)
		return fmt.Sprintf("(%s.%s).%s", pkg.ImportPath, buf.String(), fn.Name)
	}
	// Handle special case for main entry point where fn is synthetic
	if fn.Name == "main" && fn.AstDecl == nil {
		return fmt.Sprintf("%s.main", pkg.ImportPath)
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
