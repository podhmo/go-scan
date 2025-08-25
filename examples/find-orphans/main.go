package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/printer"
	"log/slog"
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
		excludeDirs  = flag.String("exclude-dirs", "vendor,testdata", "comma-separated list of directory names to exclude from scans")
		verbose      = flag.Bool("v", false, "enable verbose output")
		asJSON       = flag.Bool("json", false, "output orphans in JSON format")
	)
	flag.Parse()

	startPatterns := flag.Args()
	if len(startPatterns) == 0 {
		startPatterns = []string{"./..."}
	}

	ctx := context.Background()
	exclude := strings.Split(*excludeDirs, ",")
	if err := run(ctx, *all, *includeTests, *workspace, *verbose, *asJSON, startPatterns, exclude); err != nil {
		slog.ErrorContext(ctx, "toplevel", "error", err)
		os.Exit(1)
	}
}

// discoverModules finds all Go modules under the given root directory.
// It prioritizes a go.work file if it exists, otherwise it scans for go.mod files.
func discoverModules(ctx context.Context, root string, exclude []string) ([]string, error) {
	workFilePath := filepath.Join(root, "go.work")
	excludeMap := make(map[string]struct{}, len(exclude))
	for _, dir := range exclude {
		excludeMap[dir] = struct{}{}
	}

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
		slog.DebugContext(ctx, "discovered modules from go.work", "modules", modules)
		return modules, nil
	} else if !os.IsNotExist(err) {
		// Another error occurred with os.Stat, which is unexpected.
		return nil, fmt.Errorf("failed to stat go.work file at %s: %w", workFilePath, err)
	}

	// go.work does not exist, fall back to scanning for go.mod files.
	slog.DebugContext(ctx, "no go.work file found, falling back to go.mod scan", "root", root)
	var modules []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			dirName := d.Name()
			if _, ok := excludeMap[dirName]; ok {
				return filepath.SkipDir
			}
			if len(dirName) > 1 && dirName[0] == '.' {
				return filepath.SkipDir
			}
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

func run(ctx context.Context, all bool, includeTests bool, workspace string, verbose bool, asJSON bool, startPatterns []string, exclude []string) error {
	logLevel := new(slog.LevelVar)
	if !verbose {
		logLevel.Set(slog.LevelInfo)
	}
	opts := &slog.HandlerOptions{
		AddSource: verbose,
		Level:     logLevel,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	var scannerOpts []goscan.ScannerOption
	scannerOpts = append(scannerOpts, goscan.WithIncludeTests(includeTests))
	scannerOpts = append(scannerOpts, goscan.WithGoModuleResolver()) // Important for resolving modules
	scannerOpts = append(scannerOpts, goscan.WithWalkerExcludeDirs(exclude))

	var s *goscan.Scanner
	var err error

	if workspace != "" {
		moduleDirs, err := discoverModules(ctx, workspace, exclude)
		if err != nil {
			return err
		}
		if len(moduleDirs) == 0 {
			return fmt.Errorf("no go.mod files found in workspace root %s", workspace)
		}
		slog.DebugContext(ctx, "found modules in workspace", "count", len(moduleDirs), "modules", moduleDirs)
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
	s              *goscan.Scanner
	packages       map[string]*scanner.PackageInfo
	targetPackages map[string]struct{} // The set of packages to report orphans from
	mu             sync.Mutex
	ctx            context.Context
}

func (a *analyzer) analyze(ctx context.Context, asJSON bool, startPatterns []string) error {
	a.ctx = ctx

	// 1. Resolve user-provided patterns to a set of initial package import paths.
	// This will be our "target" set for reporting orphans.
	targetPackagePaths, err := a.s.Walker.ResolvePatternsToImportPaths(ctx, startPatterns)
	if err != nil {
		return fmt.Errorf("failed to resolve start patterns: %w", err)
	}
	a.targetPackages = make(map[string]struct{}, len(targetPackagePaths))
	for _, path := range targetPackagePaths {
		a.targetPackages[path] = struct{}{}
	}
	slog.DebugContext(ctx, "resolved target packages", "patterns", startPatterns, "resolved", targetPackagePaths)

	// 2. Walk the dependency graph starting from the same patterns.
	// The walker will correctly handle resolving these patterns to find all transitive dependencies.
	slog.DebugContext(ctx, "walking dependency graph", "patterns", startPatterns)
	if err := a.s.Walker.Walk(ctx, a, startPatterns...); err != nil {
		return fmt.Errorf("failed to walk packages: %w", err)
	}
	slog.InfoContext(ctx, "analysis phase", "packages", len(a.packages), "targets", len(a.targetPackages))

	interfaceMap := buildInterfaceMap(a.packages)
	slog.DebugContext(ctx, "built interface map", "interfaces", len(interfaceMap))

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

	slog.DebugContext(ctx, "running symbolic execution from entry points")
	// First, find all potential entry points.
	var mainEntryPoint *object.Function
	var libraryEntryPoints []*object.Function

	for _, pkg := range a.packages {
		// Load all files in the package to define all symbols in the interpreter's env
		for _, fileAst := range pkg.AstFiles {
			if _, err := interp.Eval(ctx, fileAst, pkg); err != nil {
				slog.WarnContext(ctx, "could not load package", "package", pkg.ImportPath, "error", err)
				break // if one file fails, probably best to skip the whole pkg
			}
		}

		for _, fnInfo := range pkg.Functions {
			funcObj, ok := interp.FindObject(fnInfo.Name)
			if !ok {
				slog.DebugContext(ctx, "could not find function object in interpreter", "function", fnInfo.Name, "package", pkg.ImportPath)
				continue
			}
			fn, ok := funcObj.(*object.Function)
			if !ok {
				slog.DebugContext(ctx, "object is not a function", "name", fnInfo.Name, "package", pkg.ImportPath)
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
		slog.InfoContext(ctx, "found main entry point, running in application mode")
	} else {
		entryPoints = libraryEntryPoints
		slog.InfoContext(ctx, "no main entry point found, running in library mode", "entrypoints", len(entryPoints))
	}

	for _, ep := range entryPoints {
		epName := getFullName(a.s, ep.Package, &scanner.FunctionInfo{Name: ep.Name.Name, AstDecl: ep.Decl})
		// In application mode, main is the only entry point we mark as used by default.
		// In library mode, we don't mark any entry points as used by default.
		// They are only "used" if called by another entry point.
		if mainEntryPoint != nil {
			usageMap[epName] = true
		}
		slog.DebugContext(ctx, "analyzing from entry point", "entrypoint", epName)
		interp.Apply(ctx, ep, []object.Object{}, ep.Package)
	}
	slog.InfoContext(ctx, "symbolic execution complete")

	type Orphan struct {
		Name     string `json:"name"`
		Position string `json:"position"`
		Package  string `json:"package"`
	}
	var orphans []Orphan

	for _, pkg := range a.packages {
		// Only report orphans for the packages the user explicitly asked for.
		if _, ok := a.targetPackages[pkg.ImportPath]; !ok {
			continue
		}

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
	fullPkg, err := a.s.ScanPackageByImport(a.ctx, pkg.ImportPath)
	if err != nil {
		slog.WarnContext(a.ctx, "could not scan package", "package", pkg.ImportPath, "error", err)
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
