package main

import (
	"context"
	"flag"
	"fmt"
	"go/ast"
	"io"
	"log"
	"log/slog"
	"os"
	"sort"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

// CallInfo stores information about a single function call.
type CallInfo struct {
	Callee      *scanner.FunctionInfo
	IsRecursive bool
}

// callGraph is a data structure to store the call graph.
// The key is the caller function, and the value is a list of CallInfo structs.
type callGraph map[*scanner.FunctionInfo][]*CallInfo

// stringSlice is a custom type to handle multiple string flags.
type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	// 1. Define and parse command-line flags.
	var targets stringSlice
	flag.Var(&targets, "target", "Target function or method to inspect (e.g., mypkg.MyFunc, (*mypkg.MyType).MyMethod). Can be specified multiple times.")
	var pkgPatterns stringSlice
	flag.Var(&pkgPatterns, "pkg", "Go package pattern to inspect (e.g., ./...). Can be specified multiple times.")
	trimPrefix := flag.Bool("trim-prefix", false, "Trim module path prefix from output")
	includeUnexported := flag.Bool("include-unexported", false, "Include unexported functions as entry points")
	shortFormat := flag.Bool("short", false, "Use short format for output")
	expandFormat := flag.Bool("expand", false, "Use expand format for output with UIDs")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")

	flag.Parse()

	if len(pkgPatterns) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	var level slog.Level
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		log.Fatalf("Unknown log level: %s", *logLevel)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if err := run(os.Stdout, logger, pkgPatterns, targets, *trimPrefix, *includeUnexported, *shortFormat, *expandFormat); err != nil {
		log.Fatalf("Error: %+v", err)
	}
}

// getFuncTargetName generates a canonical name for a function or method for matching against the -target flag.
// For a function: "pkg/path.FuncName"
// For a method: "(*pkg/path.TypeName).MethodName"
func getFuncTargetName(f *scanner.FunctionInfo) string {
	if f == nil {
		return ""
	}
	if f.Receiver == nil {
		// It's a function
		return fmt.Sprintf("%s.%s", f.PkgPath, f.Name)
	}
	// It's a method
	// f.Receiver.Type.String() should give us the type name, including the package path for cross-package types.
	// e.g., "*github.com/podhmo/go-scan/examples/goinspect/testdata/src/myapp.Person"
	return fmt.Sprintf("(%s).%s", f.Receiver.Type.String(), f.Name)
}

func run(out io.Writer, logger *slog.Logger, pkgPatterns []string, targets []string, trimPrefix, includeUnexported, shortFormat, expandFormat bool) error {
	ctx := context.Background()

	// 2. Scan packages using goscan.
	s, err := goscan.New(
		goscan.WithLogger(logger),
		goscan.WithGoModuleResolver(), // Enable resolving stdlib and external modules
		// goscan.WithLoadMode(goscan.LoadModeNeedsAll), // TODO: find the correct option
	)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	var pkgs []*scanner.PackageInfo
	for _, pkgPattern := range pkgPatterns {
		scannedPkgs, err := s.Scan(ctx, pkgPattern)
		if err != nil {
			return fmt.Errorf("failed to scan package pattern %q: %w", pkgPattern, err)
		}
		pkgs = append(pkgs, scannedPkgs...)
	}

	// DEBUG: Check if function bodies are loaded for the scanned packages.
	for _, pkg := range pkgs {
		logger.Debug("checking package", "pkg", pkg.ID, "importPath", pkg.ImportPath)
		for _, f := range pkg.Functions {
			hasBody := f.AstDecl != nil && f.AstDecl.Body != nil
			isExported := ast.IsExported(f.Name)
			if isExported { // Log only exported functions for brevity
				logger.Debug("  - func", "name", f.Name, "hasBody", hasBody, "exported", isExported)
			}
		}
	}

	// Define the analysis scope.
	primaryScope := make(map[string]bool)
	for _, pkg := range pkgs {
		primaryScope[pkg.ImportPath] = true
	}
	scanPolicy := func(importPath string) bool {
		return primaryScope[importPath]
	}

	// 3. Initialize symgo.Evaluator with a custom intrinsic.
	graph := make(callGraph)
	interp, err := symgo.NewInterpreter(s,
		symgo.WithLogger(logger.WithGroup("symgo")),
		symgo.WithScanPolicy(scanPolicy),
	)
	if err != nil {
		return fmt.Errorf("failed to create interpreter: %w", err)
	}

	interp.RegisterDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
		callerFrame, ok := evaluator.FrameFromContext(ctx)
		if !ok {
			return nil // Should not happen if called from evalCallExpr
		}

		calleeObj := args[0]
		var calleeFunc *scanner.FunctionInfo
		isRecursive := false

		switch f := calleeObj.(type) {
		case *object.Function:
			calleeFunc = f.Def
		case *object.SymbolicPlaceholder:
			calleeFunc = f.UnderlyingFunc
			// Check if the placeholder reason indicates a recursive call.
			if strings.HasPrefix(f.Reason, "bounded recursion depth exceeded") {
				isRecursive = true
			}
		}

		if callerFrame.Fn != nil && callerFrame.Fn.Def != nil && calleeFunc != nil {
			callerFunc := callerFrame.Fn.Def
			if callerFunc == nil {
				return nil
			}
			// Also check for direct recursion (caller is the same as callee).
			if !isRecursive && callerFunc == calleeFunc {
				isRecursive = true
			}
			graph[callerFunc] = append(graph[callerFunc], &CallInfo{
				Callee:      calleeFunc,
				IsRecursive: isRecursive,
			})
		}
		return nil
	})

	// 4. Determine entry point functions for analysis.
	var entryPoints []*scanner.FunctionInfo
	if len(targets) > 0 {
		// If specific targets are provided, find them.
		targetSet := make(map[string]bool)
		for _, target := range targets {
			targetSet[target] = true
		}

		for _, pkg := range pkgs {
			for _, f := range pkg.Functions {
				// Check for function: "pkg/path.FuncName"
				// Check for method: "(*pkg/path.TypeName).MethodName"
				targetName := getFuncTargetName(f)
				if targetSet[targetName] {
					entryPoints = append(entryPoints, f)
				}
			}
		}
		if len(entryPoints) != len(targets) {
			logger.Warn("could not find all specified targets", "found", len(entryPoints), "wanted", len(targets))
		}

	} else {
		// If no targets are specified, use all functions in the scanned packages as potential entry points.
		for _, pkg := range pkgs {
			for _, f := range pkg.Functions {
				if includeUnexported || ast.IsExported(f.Name) {
					entryPoints = append(entryPoints, f)
				}
			}
		}
	}

	logger.Info("starting analysis", "entrypoints", len(entryPoints))
	for _, f := range entryPoints {
		if _, ok := graph[f]; ok {
			continue // Already visited as part of another call
		}

		// The package for the entry point function should be in the scanner's cache.
		pkg, ok := interp.Scanner().AllSeenPackages()[f.Pkg.ID]
		if !ok {
			logger.Warn("package not found in cache for entrypoint, this is unexpected", "func", f.Name, "pkgID", f.Pkg.ID)
			continue
		}

		eval := interp.EvaluatorForTest()
		pkgObj, err := eval.GetOrLoadPackageForTest(ctx, f.Pkg.ImportPath)
		if err != nil {
			logger.Warn("failed to load package for entrypoint, skipping", "func", f.Name, "pkg", f.Pkg.ImportPath, "error", err)
			continue
		}

		fnObj := eval.GetOrResolveFunctionForTest(ctx, pkgObj, f)
		if fnObj == nil {
			logger.Warn("could not resolve function object for entrypoint", "func", f.Name, "pkg", f.Pkg.ImportPath)
			continue
		}

		// Prepare symbolic arguments for the function/method call.
		var args []object.Object
		if f.Receiver != nil {
			// Create a symbolic placeholder for the receiver.
			args = append(args, &object.SymbolicPlaceholder{Reason: f.Receiver.Type.String()})
		}
		for _, p := range f.Parameters {
			// Create a symbolic placeholder for each parameter.
			args = append(args, &object.SymbolicPlaceholder{Reason: p.Type.String()})
		}

		interp.Apply(ctx, fnObj, args, pkg)
	}

	// 5. Filter for true top-level functions (not called by any other entry point).
	callees := make(map[string]bool)
	for _, calledInfos := range graph {
		for _, info := range calledInfos {
			callees[getFuncID(info.Callee)] = true
		}
	}

	var topLevelFunctions []*scanner.FunctionInfo
	for _, f := range entryPoints {
		if !callees[getFuncID(f)] {
			topLevelFunctions = append(topLevelFunctions, f)
		}
	}

	// 6. Print the call graph starting from the true top-level functions.
	var modulePrefix string
	if trimPrefix {
		l, err := locator.New(".")
		if err != nil {
			logger.Warn("could not find module root, --trim-prefix will be ignored", "error", err)
		} else {
			modulePrefix = l.ModulePath()
		}
	}

	p := &Printer{
		Graph:      graph,
		Short:      shortFormat,
		Expand:     expandFormat,
		Out:        out,
		TrimPrefix: modulePrefix,
		// visited and assigned are initialized in Print()
	}
	p.Print(topLevelFunctions)

	return nil
}

// getFuncID generates a unique and stable identifier for a function.
// It uses the package's unique ID and the function's syntax position.
func getFuncID(f *scanner.FunctionInfo) string {
	if f == nil {
		return ""
	}
	pkgID := f.PkgPath
	if f.Pkg != nil {
		pkgID = f.Pkg.ID
	}
	var pos int
	if f.AstDecl != nil {
		pos = int(f.AstDecl.Pos())
	}
	return fmt.Sprintf("%s:%d", pkgID, pos)
}

// Printer handles the output of the call graph.
type Printer struct {
	Graph      callGraph
	Short      bool
	Expand     bool
	Out        io.Writer
	TrimPrefix string

	// State for printing
	visited  map[string]bool // Key: func ID. For preventing infinite recursion in printing.
	assigned map[string]int  // Key: func ID. For assigning numeric UIDs in expand mode.
	nextID   int
}

// Print starts the printing process for the given entry points.
func (p *Printer) Print(entryPoints []*scanner.FunctionInfo) {
	p.visited = make(map[string]bool)
	p.assigned = make(map[string]int)
	p.nextID = 0

	// Create a stable sort order for the entry points to ensure deterministic output.
	sort.Slice(entryPoints, func(i, j int) bool {
		return getFuncID(entryPoints[i]) < getFuncID(entryPoints[j])
	})

	for _, f := range entryPoints {
		p.printRecursive(f, 0, false)
	}
}

func (p *Printer) printRecursive(f *scanner.FunctionInfo, indent int, isRecursive bool) {
	id := getFuncID(f)

	var prefixes []string
	if isAccessor(f) {
		prefixes = append(prefixes, "[accessor]")
	}
	if isRecursive {
		prefixes = append(prefixes, "[recursive]")
	}

	prefixStr := ""
	if len(prefixes) > 0 {
		prefixStr = strings.Join(prefixes, " ") + " "
	}

	formatted := p.formatFunc(f)

	// Default mode (expand=false): Use IDs to show a function's tree only once.
	if !p.Expand {
		if num, ok := p.assigned[id]; ok {
			// This function has been printed before, just show its reference.
			fmt.Fprintf(p.Out, "%s%s%s #%d\n", strings.Repeat("  ", indent), prefixStr, formatted, num)
			return
		}
	}

	// Cycle detection (for both modes).
	if p.visited[id] {
		cycleRef := ""
		if !p.Expand {
			if num, ok := p.assigned[id]; ok {
				cycleRef = fmt.Sprintf(" #%d", num)
			}
		}
		fmt.Fprintf(p.Out, "%s%s%s ... (cycle detected%s)\n", strings.Repeat("  ", indent), prefixStr, formatted, cycleRef)
		return
	}

	p.visited[id] = true
	defer func() { p.visited[id] = false }()

	if !p.Expand {
		p.nextID++
		p.assigned[id] = p.nextID
		fmt.Fprintf(p.Out, "%s%s%s #%d\n", strings.Repeat("  ", indent), prefixStr, formatted, p.nextID)
	} else {
		fmt.Fprintf(p.Out, "%s%s%s\n", strings.Repeat("  ", indent), prefixStr, formatted)
	}

	if callInfos, ok := p.Graph[f]; ok {
		uniqueCallees := make([]*CallInfo, 0, len(callInfos))
		seen := make(map[string]bool)
		for _, info := range callInfos {
			calleeID := getFuncID(info.Callee)
			if !seen[calleeID] {
				uniqueCallees = append(uniqueCallees, info)
				seen[calleeID] = true
			}
		}

		sort.Slice(uniqueCallees, func(i, j int) bool {
			return getFuncID(uniqueCallees[i].Callee) < getFuncID(uniqueCallees[j].Callee)
		})

		for _, info := range uniqueCallees {
			p.printRecursive(info.Callee, indent+1, info.IsRecursive)
		}
	}
}

// isAccessor checks if a function is a simple getter or setter.
func isAccessor(f *scanner.FunctionInfo) bool {
	if f == nil || f.AstDecl == nil || f.AstDecl.Recv == nil || f.AstDecl.Body == nil || len(f.AstDecl.Body.List) != 1 {
		return false
	}

	stmt := f.AstDecl.Body.List[0]

	// Getter heuristic
	// e.g., func (d *Data) GetID() string { return d.id }
	if len(f.Parameters) == 0 && len(f.Results) == 1 {
		if ret, ok := stmt.(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
			if sel, ok := ret.Results[0].(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					// Check if the receiver name matches
					if len(f.AstDecl.Recv.List) > 0 && len(f.AstDecl.Recv.List[0].Names) > 0 {
						return ident.Name == f.AstDecl.Recv.List[0].Names[0].Name
					}
				}
			}
		}
	}

	// Setter heuristic
	// e.g., func (d *Data) SetName(name string) { d.name = name }
	if len(f.Parameters) == 1 && len(f.Results) == 0 {
		if assign, ok := stmt.(*ast.AssignStmt); ok && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
			if sel, ok := assign.Lhs[0].(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					if len(f.AstDecl.Recv.List) > 0 && len(f.AstDecl.Recv.List[0].Names) > 0 {
						return ident.Name == f.AstDecl.Recv.List[0].Names[0].Name
					}
				}
			}
		}
	}

	return false
}

// formatFunc formats the function info into a string.
func (p *Printer) formatFunc(f *scanner.FunctionInfo) string {
	if f == nil {
		return "<nil>"
	}

	// trim is a helper function that removes the module prefix from a given string.
	// It correctly handles package paths and fully qualified type names for
	// both root packages and sub-packages.
	trim := func(s string) string {
		if p.TrimPrefix == "" {
			return s
		}
		// First, replace the prefix for sub-packages (e.g., "my/module/pkg" -> "pkg").
		// This also handles nested types like "(*my/module/pkg.Type)".
		res := strings.ReplaceAll(s, p.TrimPrefix+"/", "")
		// Second, replace the prefix for root package types (e.g., "my/module.Type" -> "Type").
		res = strings.ReplaceAll(res, p.TrimPrefix+".", "")
		// Finally, if the string was the module path itself, it will not have been
		// modified by the replacements. In this case, return an empty string.
		if res == p.TrimPrefix {
			return ""
		}
		return res
	}

	var b strings.Builder
	b.WriteString("func ")

	if f.Receiver != nil {
		// Method: func (receiver) MethodName(...)
		b.WriteString("(")
		b.WriteString(trim(f.Receiver.Type.String()))
		b.WriteString(")")
		b.WriteString(".")
	} else {
		// Function: func pkg.FuncName(...)
		trimmedPkgPath := trim(f.PkgPath)
		if trimmedPkgPath != "" {
			b.WriteString(trimmedPkgPath)
			b.WriteString(".")
		} else if f.PkgPath == "" {
			// This handles interface methods where PkgPath is empty.
			// The golden files expect a leading dot in this case (e.g., ".Greet").
			b.WriteString(".")
		}
	}

	b.WriteString(f.Name)

	if p.Short {
		b.WriteString("(...)")
	} else {
		b.WriteString("(")
		params := make([]string, len(f.Parameters))
		for i, param := range f.Parameters {
			params[i] = trim(param.Type.String())
		}
		b.WriteString(strings.Join(params, ", "))
		b.WriteString(")")
	}
	// TODO: Add results
	return b.String()
}
