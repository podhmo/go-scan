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
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

// callGraph is a data structure to store the call graph.
// The key is the caller function, and the value is a list of callee functions.
type callGraph map[*scanner.FunctionInfo][]*scanner.FunctionInfo

func main() {
	// 1. Define and parse command-line flags.
	pkgPattern := flag.String("pkg", "", "Go package pattern to inspect (e.g., ./...)")
	includeUnexported := flag.Bool("include-unexported", false, "Include unexported functions as entry points")
	shortFormat := flag.Bool("short", false, "Use short format for output")
	expandFormat := flag.Bool("expand", false, "Use expand format for output with UIDs")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")

	flag.Parse()

	if *pkgPattern == "" {
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

	if err := run(os.Stdout, logger, *pkgPattern, *includeUnexported, *shortFormat, *expandFormat); err != nil {
		log.Fatalf("Error: %+v", err)
	}
}

func run(out io.Writer, logger *slog.Logger, pkgPattern string, includeUnexported, shortFormat, expandFormat bool) error {
	ctx := context.Background()

	// 2. Scan packages using goscan.
	s, err := goscan.New(
		goscan.WithLogger(logger),
		// goscan.WithLoadMode(goscan.LoadModeNeedsAll), // TODO: find the correct option
	)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	pkgs, err := s.Scan(ctx, pkgPattern)
	if err != nil {
		return fmt.Errorf("failed to scan package pattern %q: %w", pkgPattern, err)
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

		switch f := calleeObj.(type) {
		case *object.Function:
			calleeFunc = f.Def
		case *object.SymbolicPlaceholder:
			calleeFunc = f.UnderlyingFunc
		}

		if callerFrame.Fn != nil && callerFrame.Fn.Def != nil && calleeFunc != nil {
			callerFunc := callerFrame.Fn.Def
			if callerFunc == nil {
				return nil
			}
			graph[callerFunc] = append(graph[callerFunc], calleeFunc)
		}
		return nil
	})

	// 4. Build the call graph by evaluating entry point functions.
	var entryPoints []*scanner.FunctionInfo
	for _, pkg := range pkgs {
		for _, f := range pkg.Functions {
			if includeUnexported || ast.IsExported(f.Name) {
				entryPoints = append(entryPoints, f)
			}
		}
	}

	logger.Info("starting analysis", "entrypoints", len(entryPoints))
	for _, pkg := range pkgs {
		for _, f := range pkg.Functions {
			if !includeUnexported && !ast.IsExported(f.Name) {
				continue
			}
			if _, ok := graph[f]; ok {
				continue // Already visited as part of another call
			}
			eval := interp.EvaluatorForTest()
			pkgObj, err := eval.GetOrLoadPackageForTest(ctx, pkg.ImportPath)
			if err != nil {
				logger.Warn("failed to load package for entrypoint, skipping", "func", f.Name, "pkg", pkg.ImportPath, "error", err)
				continue
			}
			fnObj := eval.GetOrResolveFunctionForTest(ctx, pkgObj, f)
			interp.Apply(ctx, fnObj, nil, pkg)
		}
	}

	// 5. Filter for true top-level functions (not called by any other entry point).
	callees := make(map[string]bool)
	for _, calledFuncs := range graph {
		for _, f := range calledFuncs {
			callees[getFuncID(f)] = true
		}
	}

	var topLevelFunctions []*scanner.FunctionInfo
	for _, f := range entryPoints {
		if !callees[getFuncID(f)] {
			topLevelFunctions = append(topLevelFunctions, f)
		}
	}

	// 6. Print the call graph starting from the true top-level functions.
	p := &Printer{
		Graph:  graph,
		Short:  shortFormat,
		Expand: expandFormat,
		Out:    out,
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
	Graph  callGraph
	Short  bool
	Expand bool
	Out    io.Writer

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
		p.printRecursive(f, 0)
	}
}

func (p *Printer) printRecursive(f *scanner.FunctionInfo, indent int) {
	id := getFuncID(f)

	accessorPrefix := ""
	if isAccessor(f) {
		accessorPrefix = "[accessor] "
	}
	formatted := formatFunc(f, p.Short)

	// Default mode (expand=false): Use IDs to show a function's tree only once.
	if !p.Expand {
		if num, ok := p.assigned[id]; ok {
			// This function has been printed before, just show its reference.
			fmt.Fprintf(p.Out, "%s%s%s #%d\n", strings.Repeat("  ", indent), accessorPrefix, formatted, num)
			return
		}
	}

	// Cycle detection (for both modes). If we are currently visiting this node in this path, it's a cycle.
	if p.visited[id] {
		cycleRef := ""
		// In default mode, a cycle to a node that's already been fully printed
		// would have been caught by the `p.assigned` check above. This `p.visited`
		// check is for cycles within a *single* new call tree that we are in the
		// process of printing for the first time.
		if !p.Expand {
			if num, ok := p.assigned[id]; ok {
				cycleRef = fmt.Sprintf(" #%d", num)
			}
		}
		fmt.Fprintf(p.Out, "%s%s%s ... (cycle detected%s)\n", strings.Repeat("  ", indent), accessorPrefix, formatted, cycleRef)
		return
	}

	// Mark as visited for the current path traversal.
	p.visited[id] = true

	// --- Printing Logic (Corrected) ---
	// - expand=false (default): show ID on first appearance, reference on subsequent.
	// - expand=true: show full tree every time, no IDs.
	if !p.Expand {
		// First time seeing this function in default mode. Assign an ID and print it.
		p.nextID++
		p.assigned[id] = p.nextID
		fmt.Fprintf(p.Out, "%s%s%s #%d\n", strings.Repeat("  ", indent), accessorPrefix, formatted, p.nextID)
	} else {
		// Expand mode: just print the formatted function, no ID.
		fmt.Fprintf(p.Out, "%s%s%s\n", strings.Repeat("  ", indent), accessorPrefix, formatted)
	}

	// Recursively print callees.
	if callees, ok := p.Graph[f]; ok {
		uniqueCallees := make([]*scanner.FunctionInfo, 0, len(callees))
		seen := make(map[string]bool)
		for _, callee := range callees {
			calleeID := getFuncID(callee)
			if !seen[calleeID] {
				uniqueCallees = append(uniqueCallees, callee)
				seen[calleeID] = true
			}
		}

		// Sort callees for deterministic output.
		sort.Slice(uniqueCallees, func(i, j int) bool {
			return getFuncID(uniqueCallees[i]) < getFuncID(uniqueCallees[j])
		})

		for _, callee := range uniqueCallees {
			p.printRecursive(callee, indent+1)
		}
	}

	// Reset visited flag for this path. This allows the function to be printed again
	// if it appears in a different call branch. In expand mode, this won't happen
	// because the `p.assigned` check at the top will catch it.
	p.visited[id] = false

	// In non-expand mode, we prevent a function from being printed more than once
	// at the top level by a different mechanism (in the Print method), but this
	// reset is crucial for correct cycle detection.
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
func formatFunc(f *scanner.FunctionInfo, short bool) string {
	if f == nil {
		return "<nil>"
	}
	var b strings.Builder
	b.WriteString("func ")
	if f.Receiver != nil {
		b.WriteString("(")
		b.WriteString(f.Receiver.Type.String())
		b.WriteString(")")
		b.WriteString(".")
	} else {
		b.WriteString(f.PkgPath)
		b.WriteString(".")
	}
	b.WriteString(f.Name)

	if short {
		b.WriteString("(...)")
	} else {
		b.WriteString("(")
		params := []string{}
		for _, p := range f.Parameters {
			params = append(params, p.Type.String())
		}
		b.WriteString(strings.Join(params, ", "))
		b.WriteString(")")
	}
	// TODO: Add results
	return b.String()
}