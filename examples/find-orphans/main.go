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
	"github.com/podhmo/go-scan/locator"
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
		mode         = flag.String("mode", "auto", "analysis mode: auto, app, or lib")
		excludeDirs  stringSliceFlag
	)
	flag.Var(&excludeDirs, "exclude-dirs", "comma-separated list of directories to exclude (e.g. testdata,vendor)")
	flag.Parse()

	// Validate mode
	switch *mode {
	case "auto", "app", "lib":
		// valid
	default:
		slog.Error("invalid mode specified", "mode", *mode)
		os.Exit(1)
	}

	// Set default exclude directories
	if len(excludeDirs) == 0 {
		excludeDirs = []string{"testdata", "vendor"}
	}

	startPatterns := flag.Args()
	if len(startPatterns) == 0 {
		startPatterns = []string{"./..."}
	}

	ctx := context.Background()
	if err := run(ctx, *all, *includeTests, *workspace, *verbose, *asJSON, *mode, startPatterns, excludeDirs); err != nil {
		slog.ErrorContext(ctx, "toplevel", "error", err)
		os.Exit(1)
	}
}

// stringSliceFlag is a custom flag type for handling comma-separated strings
type stringSliceFlag []string

func (f *stringSliceFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *stringSliceFlag) Set(value string) error {
	*f = strings.Split(value, ",")
	return nil
}

// discoverModules finds all Go modules under the given root directory.
// It prioritizes a go.work file if it exists, otherwise it scans for go.mod files.
func discoverModules(ctx context.Context, root string, excludeDirs []string) ([]string, error) {
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
		slog.DebugContext(ctx, "discovered modules from go.work", "modules", modules)
		return modules, nil
	} else if !os.IsNotExist(err) {
		// Another error occurred with os.Stat, which is unexpected.
		return nil, fmt.Errorf("failed to stat go.work file at %s: %w", workFilePath, err)
	}

	// go.work does not exist, fall back to scanning for go.mod files.
	slog.DebugContext(ctx, "no go.work file found, falling back to go.mod scan", "root", root)

	excludeMap := make(map[string]bool)
	for _, dir := range excludeDirs {
		excludeMap[dir] = true
	}
	// Also add default exclusions
	excludeMap["vendor"] = true

	var modules []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if excludeMap[d.Name()] || (d.Name() != "." && strings.HasPrefix(d.Name(), ".")) {
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

func run(ctx context.Context, all bool, includeTests bool, workspace string, verbose bool, asJSON bool, mode string, startPatterns []string, excludeDirs []string) error {
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

	// Create locators first, as they are needed to resolve target packages.
	var locators []*locator.Locator
	var moduleDirs []string
	var resolutionDir string

	locatorOpts := []locator.Option{locator.WithGoModuleResolver()}
	if workspace != "" {
		var err error
		absWorkspace, err := filepath.Abs(workspace)
		if err != nil {
			return fmt.Errorf("could not get absolute path for workspace root %q: %w", workspace, err)
		}
		workspace = absWorkspace
		resolutionDir = workspace

		moduleDirs, err = discoverModules(ctx, workspace, excludeDirs)
		if err != nil {
			return err
		}
		if len(moduleDirs) == 0 {
			return fmt.Errorf("no go.mod files found in workspace root %s", workspace)
		}
		slog.DebugContext(ctx, "creating locators for workspace", "count", len(moduleDirs), "modules", moduleDirs)
		for _, dir := range moduleDirs {
			loc, err := locator.New(dir, locatorOpts...)
			if err != nil {
				return fmt.Errorf("workspace mode: failed to create locator for module %q: %w", dir, err)
			}
			locators = append(locators, loc)
		}
	} else {
		var err error
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current working directory: %w", err)
		}
		resolutionDir = cwd
		loc, err := locator.New(resolutionDir, locatorOpts...)
		if err != nil {
			return fmt.Errorf("single module mode: failed to create locator for %q: %w", resolutionDir, err)
		}
		locators = append(locators, loc)
	}
	for _, loc := range locators {
		slog.InfoContext(ctx, "* scan module", "module", loc.ModulePath())
	}

	// Resolve the target packages for reporting.
	targetPackages, err := resolveTargetPackages(ctx, locators, startPatterns, excludeDirs, resolutionDir)
	if err != nil {
		return fmt.Errorf("could not resolve target packages: %w", err)
	}
	slog.DebugContext(ctx, "resolved target packages for reporting", "count", len(targetPackages), "packages", keys(targetPackages))

	// Resolve all packages in the workspace for scanning.
	scanPatterns := []string{"./..."}
	scanPackages, err := resolveTargetPackages(ctx, locators, scanPatterns, excludeDirs, resolutionDir)
	if err != nil {
		return fmt.Errorf("could not resolve scan packages: %w", err)
	}
	slog.DebugContext(ctx, "resolved scan packages for analysis", "count", len(scanPackages), "packages", keys(scanPackages))

	// Now create the main scanner
	var scannerOpts []goscan.ScannerOption
	scannerOpts = append(scannerOpts, goscan.WithIncludeTests(includeTests))
	scannerOpts = append(scannerOpts, goscan.WithGoModuleResolver())

	if workspace != "" {
		scannerOpts = append(scannerOpts, goscan.WithModuleDirs(moduleDirs))
	} else {
		// In single-module mode, the resolutionDir is the workDir.
		scannerOpts = append(scannerOpts, goscan.WithWorkDir(resolutionDir))
	}

	s, err := goscan.New(scannerOpts...)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	a := &analyzer{
		s:              s,
		packages:       make(map[string]*scanner.PackageInfo),
		targetPackages: targetPackages,
		mode:           mode,
		scanPackages:   scanPackages,
	}
	return a.analyze(ctx, asJSON)
}

// resolveTargetPackages converts user-provided patterns (including file paths and import paths)
// into a definitive set of Go import paths. It resolves file path patterns relative to rootDir.
func resolveTargetPackages(ctx context.Context, locators []*locator.Locator, patterns []string, excludeDirs []string, rootDir string) (map[string]bool, error) {
	targetPackages := make(map[string]bool)
	excludeMap := make(map[string]bool)
	for _, dir := range excludeDirs {
		excludeMap[dir] = true
	}

	for _, pattern := range patterns {
		isRecursive := strings.HasSuffix(pattern, "/...")
		cleanPattern := strings.TrimSuffix(pattern, "/...")

		// Determine if it's a file path or import path pattern
		isFilePathPattern := strings.HasPrefix(pattern, ".") || filepath.IsAbs(pattern)

		if isFilePathPattern {
			// It's a file path pattern, e.g., '.', './...', '../..'.
			root := filepath.Clean(filepath.Join(rootDir, cleanPattern))

			err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if d.IsDir() {
					if excludeMap[d.Name()] || (d.Name() != "." && strings.HasPrefix(d.Name(), ".")) {
						return filepath.SkipDir
					}
					if !isRecursive && path != root {
						return filepath.SkipDir
					}
				}

				if !d.IsDir() && !strings.HasSuffix(d.Name(), ".go") {
					return nil
				}
				if d.IsDir() && !isRecursive && path != root {
					return filepath.SkipDir
				}

				dirPath := path
				if !d.IsDir() {
					dirPath = filepath.Dir(path)
				}

				// Check if the directory contains any .go files at all.
				// This avoids adding non-package directories to the target set.
				goFiles, err := os.ReadDir(dirPath)
				if err != nil {
					return nil // Ignore dirs we can't read
				}
				hasGo := false
				for _, f := range goFiles {
					if !f.IsDir() && strings.HasSuffix(f.Name(), ".go") {
						hasGo = true
						break
					}
				}
				if !hasGo {
					return nil
				}

				importPath, err := pathToImport(locators, dirPath)
				if err != nil {
					slog.DebugContext(ctx, "could not convert path to import, skipping", "path", dirPath, "error", err)
					return nil
				}

				if _, exists := targetPackages[importPath]; !exists {
					targetPackages[importPath] = true
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("failed to walk directory for file path pattern %s: %w", pattern, err)
			}
		} else {
			// It's an import path pattern, e.g., 'example.com/foo/...'
			if !isRecursive {
				targetPackages[cleanPattern] = true
				continue
			}

			// This is a recursive import path. We need to find its directory and walk it.
			var rootDir string
			var found bool
			for _, loc := range locators {
				dir, err := loc.FindPackageDir(cleanPattern)
				if err == nil {
					rootDir = dir
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("could not find package directory for import path pattern: %s", pattern)
			}

			err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					if excludeMap[d.Name()] || (d.Name() != "." && strings.HasPrefix(d.Name(), ".")) {
						return filepath.SkipDir
					}
				}
				if !d.IsDir() {
					return nil
				}
				importPath, err := pathToImport(locators, path)
				if err != nil {
					return nil // Skip dirs that can't be resolved
				}
				targetPackages[importPath] = true
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("failed to walk directory for import path pattern %s: %w", pattern, err)
			}
		}
	}
	return targetPackages, nil
}

// pathToImport tries to convert a file path to an import path using a list of locators.
// This is necessary in workspace mode where a path could belong to any of the modules.
func pathToImport(locators []*locator.Locator, path string) (string, error) {
	for _, loc := range locators {
		importPath, err := loc.PathToImport(path)
		if err == nil {
			return importPath, nil
		}
	}
	return "", fmt.Errorf("path %q does not belong to any known module", path)
}

func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

type analyzer struct {
	s              *goscan.Scanner
	packages       map[string]*scanner.PackageInfo
	targetPackages map[string]bool
	mode           string
	scanPackages   map[string]bool
	mu             sync.Mutex
	ctx            context.Context
}

func (a *analyzer) analyze(ctx context.Context, asJSON bool) error {
	a.ctx = ctx

	// Walk all dependencies, starting from the scan packages to find all potential usages.
	patternsToWalk := keys(a.scanPackages)

	slog.DebugContext(ctx, "walking with patterns", "patterns", patternsToWalk)
	if err := a.s.Walker.Walk(ctx, a, patternsToWalk...); err != nil {
		return fmt.Errorf("failed to walk packages: %w", err)
	}
	slog.InfoContext(ctx, "analysis phase", "packages", len(a.packages))

	interfaceMap := buildInterfaceMap(a.packages)
	slog.DebugContext(ctx, "built interface map", "interfaces", len(interfaceMap))

	interp, err := symgo.NewInterpreter(a.s, symgo.WithLogger(slog.Default()))
	if err != nil {
		return fmt.Errorf("failed to create interpreter: %w", err)
	}

	usageMap := make(map[string]bool)

	// markUsage is a helper function to mark a function/method as used.
	// It's designed to be called on any object, and it will figure out if it's a function.
	markUsage := func(obj object.Object) {
		var fullName string
		switch fn := obj.(type) {
		case *object.Function:
			if fn.Package != nil && fn.Name != nil {
				if _, isScannable := a.scanPackages[fn.Package.ImportPath]; !isScannable {
					return // Don't track usage for functions outside the scan scope.
				}

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
			// For symbolic placeholders, we also check if they belong to a scanned package.
			if fn.Package != nil {
				if _, isScannable := a.scanPackages[fn.Package.ImportPath]; !isScannable {
					return
				}
			}

			if fn.UnderlyingMethod != nil {
				methodName := fn.UnderlyingMethod.Name
				var implementerTypes []*scanner.FieldType
				if fn.Receiver != nil {
					receiverTypeInfo := fn.Receiver.TypeInfo()
					if receiverTypeInfo != nil && receiverTypeInfo.Kind == scanner.InterfaceKind {
						ifaceName := fmt.Sprintf("%s.%s", receiverTypeInfo.PkgPath, receiverTypeInfo.Name)
						if allImplementers, ok := interfaceMap[ifaceName]; ok {
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
			if fn.UnderlyingFunc != nil && fn.Package != nil && fn.UnderlyingFunc.Receiver == nil {
				fullName := fmt.Sprintf("%s.%s", fn.Package.ImportPath, fn.UnderlyingFunc.Name)
				usageMap[fullName] = true
			}
		}
	}

	interp.RegisterDefaultIntrinsic(func(i *symgo.Interpreter, args []object.Object) object.Object {
		// The intrinsic is triggered for every function call.
		// We need to mark the function being called (args[0]) as used.
		// We also need to check if any of the arguments themselves are function
		// values being passed along, and mark them as used too.
		for _, arg := range args {
			markUsage(arg)
		}
		return nil
	})

	slog.DebugContext(ctx, "running symbolic execution from entry points")
	// First, find all potential entry points.
	var mainEntryPoint *object.Function
	var libraryEntryPoints []*object.Function

	for _, pkg := range a.packages {
		slog.InfoContext(ctx, "** scan package", "package", pkg.ImportPath)

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
			} else if fnInfo.AstDecl.Name.IsExported() {
				libraryEntryPoints = append(libraryEntryPoints, fn)
			}
		}
	}

	// Decide which entry points to use based on the selected mode, and apply
	// initial usage marks.
	var analysisFns []*object.Function
	isAppMode := false

	switch a.mode {
	case "app":
		if mainEntryPoint == nil {
			return fmt.Errorf("application mode specified, but no main entry point was found")
		}
		analysisFns = []*object.Function{mainEntryPoint}
		isAppMode = true
		slog.InfoContext(ctx, "running in forced application mode")
	case "lib":
		analysisFns = libraryEntryPoints
		isAppMode = false // Explicitly false for library mode
		slog.InfoContext(ctx, "running in forced library mode", "analysis_functions", len(analysisFns))
	case "auto":
		fallthrough
	default: // auto
		if mainEntryPoint != nil {
			analysisFns = []*object.Function{mainEntryPoint}
			isAppMode = true
			slog.InfoContext(ctx, "found main entry point, running in application mode")
		} else {
			analysisFns = libraryEntryPoints
			isAppMode = false
			slog.InfoContext(ctx, "no main entry point found, running in library mode", "analysis_functions", len(analysisFns))
		}
	}

	// In application mode, the entry point is always considered used.
	// In library mode, we don't mark anything initially. A function is only "used"
	// if it's actually called by another function in the analysis set.
	if isAppMode {
		for _, ep := range analysisFns {
			epName := getFullName(a.s, ep.Package, &scanner.FunctionInfo{Name: ep.Name.Name, AstDecl: ep.Decl})
			usageMap[epName] = true
		}
	}

	// Run symbolic execution from each analysis function to find what they use.
	for _, ep := range analysisFns {
		epName := getFullName(a.s, ep.Package, &scanner.FunctionInfo{Name: ep.Name.Name, AstDecl: ep.Decl})
		slog.DebugContext(ctx, "analyzing from function", "function", epName)
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
		// Only report orphans from the packages the user explicitly asked to scan.
		if _, isTarget := a.targetPackages[pkg.ImportPath]; !isTarget {
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

				// Always exclude init functions and main.main from the orphan list.
				// These are entry points by definition.
				if decl.Receiver == nil {
					if decl.Name == "init" {
						continue
					}
					if pkg.Name == "main" && decl.Name == "main" {
						continue
					}
				}

				// Exclude actual test functions, which are entry points for the test runner.
				// A function is considered a test entry point if it has a test-like name
				// AND resides in a _test.go file. A function with a test-like name in a
				// regular .go file is just a regular function.
				pos := a.s.Fset().Position(decl.AstDecl.Pos())
				isTestFile := strings.HasSuffix(pos.Filename, "_test.go")
				isTestFunc := strings.HasPrefix(decl.Name, "Test") ||
					strings.HasPrefix(decl.Name, "Benchmark") ||
					strings.HasPrefix(decl.Name, "Example") ||
					strings.HasPrefix(decl.Name, "Fuzz")

				// If `a.s.Config.IncludeTests` is false, `isTestFile` will always be false
				// because no _test.go files are scanned, so this check works correctly
				// in both cases.
				if isTestFunc && isTestFile {
					continue
				}

				if decl.AstDecl.Doc != nil {
					for _, comment := range decl.AstDecl.Doc.List {
						if strings.Contains(comment.Text, "//go:scan:ignore") {
							goto nextDecl
						}
					}
				}
				orphans = append(orphans, Orphan{
					Name:     name,
					Position: pos.String(), // pos is already defined above
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

	// Only follow imports that are part of the original scan scope.
	// This prevents the walker from traversing into third-party dependencies.
	var importsToFollow []string
	for _, imp := range pkg.Imports {
		if _, ok := a.scanPackages[imp]; ok {
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
