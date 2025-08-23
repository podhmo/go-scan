package main

import (
	"context"
	"flag"
	"fmt"
	"go/token"
	"log"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/find-orphans/id"
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
		debug        = flag.Bool("debug", false, "enable debug logging")
	)
	flag.Parse()

	if err := run(context.Background(), *all, *includeTests, *workspace, *verbose, *debug); err != nil {
		log.Fatalf("!! %+v", err)
	}
}

func run(ctx context.Context, all bool, includeTests bool, workspace string, verbose bool, debug bool) error {
	var scannerOpts []goscan.ScannerOption
	scannerOpts = append(scannerOpts, goscan.WithIncludeTests(includeTests))

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

	var startPatterns []string
	if flag.NArg() > 0 {
		startPatterns = flag.Args()
	} else if workspace != "" {
		startPatterns = []string{workspace}
	} else {
		startPatterns = []string{"."}
	}

	analyzer, err := NewAnalyzer(s, logger)
	if err != nil {
		return fmt.Errorf("failed to create analyzer: %w", err)
	}

	pkgs, err := analyzer.DiscoverPackages(ctx, startPatterns)
	if err != nil {
		return err
	}

	results, err := analyzer.Analyze(ctx, pkgs)
	if err != nil {
		return err
	}

	results.Report(verbose)

	return nil
}

// Analyzer holds the state for the orphan analysis.
type Analyzer struct {
	Scanner     *goscan.Scanner
	Interpreter *symgo.Interpreter
	Logger      *slog.Logger
}

// NewAnalyzer creates a new analyzer.
func NewAnalyzer(s *goscan.Scanner, logger *slog.Logger) (*Analyzer, error) {
	interp, err := symgo.NewInterpreter(s, symgo.WithLogger(logger))
	if err != nil {
		return nil, fmt.Errorf("failed to create interpreter: %w", err)
	}
	return &Analyzer{
		Scanner:     s,
		Interpreter: interp,
		Logger:      logger,
	}, nil
}

// DiscoverPackages finds all packages to be analyzed based on the initial patterns.
func (a *Analyzer) DiscoverPackages(ctx context.Context, patterns []string) (map[string]*scanner.PackageInfo, error) {
	a.Logger.Info("discovering packages", "patterns", patterns)
	visitor := &collectorVisitor{
		s:        a.Scanner,
		packages: make(map[string]*scanner.PackageInfo),
		logger:   a.Logger,
	}
	for _, pattern := range patterns {
		if err := a.Scanner.Walker.Walk(ctx, pattern, visitor); err != nil {
			return nil, fmt.Errorf("failed to walk packages from %q: %w", pattern, err)
		}
	}
	a.Logger.Info("discovered packages", "count", len(visitor.packages))
	return visitor.packages, nil
}

// AnalysisResult holds the outcome of the analysis.
type AnalysisResult struct {
	AllDecls map[string]*scanner.FunctionInfo
	UsageMap map[string][]string
	Logger   *slog.Logger
	Fset     *token.FileSet
}

type InterfaceImplMap map[string][]*scanner.TypeInfo

func getMethods(t *scanner.TypeInfo, pkg *scanner.PackageInfo) []*scanner.FunctionInfo {
	var methods []*scanner.FunctionInfo
	for _, fn := range pkg.Functions {
		if fn.Receiver != nil && fn.Receiver.Type.TypeName == t.Name {
			methods = append(methods, fn)
		}
	}
	return methods
}

func implements(concrete *scanner.TypeInfo, iface *scanner.TypeInfo, concretePkg *scanner.PackageInfo) bool {
	if concrete.Kind != scanner.StructKind || iface.Kind != scanner.InterfaceKind {
		return false
	}

	ifaceMethods := make(map[string]bool)
	if iface.Interface != nil {
		for _, m := range iface.Interface.Methods {
			ifaceMethods[m.Name] = true
		}
	}

	concreteMethods := getMethods(concrete, concretePkg)
	for _, m := range concreteMethods {
		delete(ifaceMethods, m.Name)
	}

	return len(ifaceMethods) == 0
}

// buildInterfaceImplMap creates a map from interface type IDs to the concrete types that implement them.
func (a *Analyzer) buildInterfaceImplMap(pkgs map[string]*scanner.PackageInfo) (InterfaceImplMap, error) {
	implMap := make(InterfaceImplMap)
	var interfaces []*scanner.TypeInfo
	var concretes []*scanner.TypeInfo

	for _, pkg := range pkgs {
		for _, t := range pkg.Types {
			if t.Kind == scanner.InterfaceKind {
				interfaces = append(interfaces, t)
			} else if t.Kind == scanner.StructKind {
				concretes = append(concretes, t)
			}
		}
	}

	for _, iface := range interfaces {
		ifaceID := id.FromType(iface)
		for _, concrete := range concretes {
			concretePkg, ok := pkgs[concrete.PkgPath]
			if !ok {
				continue
			}
			if implements(concrete, iface, concretePkg) {
				implMap[ifaceID] = append(implMap[ifaceID], concrete)
			}
		}
	}

	return implMap, nil
}

// Analyze performs the symbolic execution to find orphans.
func (a *Analyzer) Analyze(ctx context.Context, pkgs map[string]*scanner.PackageInfo) (*AnalysisResult, error) {
	implMap, err := a.buildInterfaceImplMap(pkgs)
	if err != nil {
		return nil, fmt.Errorf("failed to build interface implementation map: %w", err)
	}
	for iface, impls := range implMap {
		var implNames []string
		for _, impl := range impls {
			implNames = append(implNames, id.FromType(impl))
		}
		a.Logger.Debug("interface implementation", "interface", iface, "implementations", implNames)
	}

	usageMap := make(map[string][]string)

	a.Interpreter.RegisterDefaultIntrinsic(func(i *symgo.Interpreter, args []object.Object) object.Object {
		if len(args) == 0 {
			return nil
		}
		caller := i.CurrentFunc()
		var callerID string
		if caller != nil && caller.Package != nil {
			var callerInfo *scanner.FunctionInfo
			for _, f := range caller.Package.Functions {
				if f.AstDecl == caller.Decl {
					callerInfo = f
					break
				}
			}
			if callerInfo != nil {
				callerID = id.FromFunc(caller.Package, callerInfo)
			}
		}

		addUsage := func(usedID string) {
			if usedID != "" {
				a.Logger.Debug("usage", "caller", callerID, "used", usedID)
				usageMap[usedID] = append(usageMap[usedID], callerID)
			}
		}

		fnObj := args[0]
		fnID := ""

		switch fn := fnObj.(type) {
		case *object.Function:
			if fn.Package != nil && fn.Name != nil {
				var funcInfo *scanner.FunctionInfo
				for _, f := range fn.Package.Functions {
					if f.AstDecl == fn.Decl {
						funcInfo = f
						break
					}
				}
				if funcInfo != nil {
					fnID = id.FromFunc(fn.Package, funcInfo)
				}
			}
		case *object.SymbolicPlaceholder:
			if fn.UnderlyingFunc != nil && fn.Package != nil {
				fnID = id.FromFunc(fn.Package, fn.UnderlyingFunc)
			}
		}
		addUsage(fnID)

		if fn, ok := fnObj.(*object.SymbolicPlaceholder); ok {
			if fn.UnderlyingFunc != nil && fn.UnderlyingFunc.Receiver != nil {
				recvType, err := fn.UnderlyingFunc.Receiver.Type.Resolve(ctx)
				if err != nil {
					a.Logger.Warn("could not resolve receiver type", "error", err)
					return nil
				}

				if recvType.Kind == scanner.InterfaceKind {
					ifaceID := id.FromType(recvType)
					if concreteTypes, ok := implMap[ifaceID]; ok {
						for _, concreteType := range concreteTypes {
							concretePkg, ok := pkgs[concreteType.PkgPath]
							if !ok {
								a.Logger.Warn("package not found for concrete type", "pkg_path", concreteType.PkgPath)
								continue
							}

							methods := getMethods(concreteType, concretePkg)
							for _, method := range methods {
								if method.Name == fn.UnderlyingFunc.Name {
									methodID := id.FromFunc(concretePkg, method)
									addUsage(methodID)
									break
								}
							}
						}
					}
				}
			}
		}

		return nil
	})

	a.Logger.Info("running symbolic execution")
	// First, evaluate all package files to populate the interpreter's environment
	// with all function declarations.
	for _, pkg := range pkgs {
		for _, file := range pkg.AstFiles {
			_, err := a.Interpreter.Eval(ctx, file, pkg)
			if err != nil {
				a.Logger.Warn("error evaluating file", "file", file.Name, "error", err)
			}
		}
	}

	// Then, find and apply only the main functions as entry points.
	for _, pkg := range pkgs {
		if pkg.Name == "main" {
			var mainFuncInfo *scanner.FunctionInfo
			for _, f := range pkg.Functions {
				if f.Name == "main" {
					mainFuncInfo = f
					break
				}
			}
			if mainFuncInfo == nil {
				continue
			}

			mainFuncObj, ok := a.Interpreter.FindObject("main")
			if !ok {
				a.Logger.Warn("main function object not found in interpreter", "pkg", pkg.ImportPath)
				continue
			}

			a.Logger.Debug("found entrypoint", "function", id.FromFunc(pkg, mainFuncInfo))
			usageMap[id.FromFunc(pkg, mainFuncInfo)] = []string{"<entrypoint>"}
			_, err := a.Interpreter.Apply(ctx, mainFuncObj, []object.Object{}, pkg)
			if err != nil {
				a.Logger.Warn("error applying main function", "pkg", pkg.ImportPath, "error", err)
			}
		}
	}
	a.Logger.Info("symbolic execution complete")

	allDecls := make(map[string]*scanner.FunctionInfo)
	for _, pkg := range pkgs {
		for _, decl := range pkg.Functions {
			allDecls[id.FromFunc(pkg, decl)] = decl
		}
	}

	return &AnalysisResult{
		AllDecls: allDecls,
		UsageMap: usageMap,
		Logger:   a.Logger,
		Fset:     a.Scanner.Fset(),
	}, nil
}

// Report prints the final analysis results.
func (r *AnalysisResult) Report(verbose bool) {
	var orphanNames []string
	orphans := make(map[string]*scanner.FunctionInfo)

	for name, decl := range r.AllDecls {
		if _, used := r.UsageMap[name]; !used {
			if strings.Contains(decl.Doc, "//go:scan:ignore") {
				r.Logger.Debug("ignoring orphan", "name", name)
				continue
			}
			orphanNames = append(orphanNames, name)
			orphans[name] = decl
		}
	}
	sort.Strings(orphanNames)

	if verbose {
		fmt.Println("\n-- Used Functions --")
		var usedNames []string
		for name := range r.UsageMap {
			usedNames = append(usedNames, name)
		}
		sort.Strings(usedNames)

		for _, name := range usedNames {
			if r.AllDecls[name] == nil {
				continue
			}
			pos := r.Fset.Position(r.AllDecls[name].AstDecl.Pos())
			fmt.Printf("%s\n  %s\n", name, pos)
			callers := r.UsageMap[name]
			callerMap := make(map[string]bool)
			for _, c := range callers {
				if c != "" {
					callerMap[c] = true
				}
			}
			var sortedCallers []string
			for c := range callerMap {
				sortedCallers = append(sortedCallers, c)
			}
			sort.Strings(sortedCallers)

			for _, c := range sortedCallers {
				fmt.Printf("  - used by: %s\n", c)
			}
		}
	}

	fmt.Println("\n-- Orphans --")
	if len(orphanNames) == 0 {
		fmt.Println("No orphans found.")
		return
	}

	for _, name := range orphanNames {
		decl := orphans[name]
		pos := r.Fset.Position(decl.AstDecl.Pos())
		fmt.Printf("%s\n  %s\n", name, pos)
	}
}

type collectorVisitor struct {
	s        *goscan.Scanner
	packages map[string]*scanner.PackageInfo
	mu       sync.Mutex
	logger   *slog.Logger
}

func (v *collectorVisitor) Visit(pkg *goscan.PackageImports) ([]string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, exists := v.packages[pkg.ImportPath]; exists {
		return nil, nil
	}
	fullPkg, err := v.s.ScanPackageByImport(context.Background(), pkg.ImportPath)
	if err != nil {
		v.logger.Warn("could not scan package", "path", pkg.ImportPath, "error", err)
		return nil, nil
	}
	v.packages[pkg.ImportPath] = fullPkg
	return pkg.Imports, nil
}
