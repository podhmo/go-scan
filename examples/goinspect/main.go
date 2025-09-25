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

	// 5. Print the call graph.
	p := &Printer{
		Graph:    graph,
		Short:    shortFormat,
		Expand:   expandFormat,
		Out:      out,
		visited:  make(map[*scanner.FunctionInfo]bool),
		assigned: make(map[*scanner.FunctionInfo]int),
	}
	p.Print(entryPoints)

	return nil
}

// Printer handles the output of the call graph.
type Printer struct {
	Graph    callGraph
	Short    bool
	Expand   bool
	Out      io.Writer
	visited  map[*scanner.FunctionInfo]bool // For preventing infinite recursion in printing
	assigned map[*scanner.FunctionInfo]int  // For assigning UIDs in expand mode
	nextID   int
}

// Print starts the printing process for the given entry points.
func (p *Printer) Print(entryPoints []*scanner.FunctionInfo) {
	for _, f := range entryPoints {
		// Only print functions that are actual entry points (not just called by other entry points).
		// A simple heuristic: if a function is a key in the graph, it's a caller.
		if _, isCaller := p.Graph[f]; isCaller {
			p.printRecursive(f, 0)
		}
	}
}

func (p *Printer) printRecursive(f *scanner.FunctionInfo, indent int) {
	if p.visited[f] {
		if p.Expand {
			fmt.Fprintf(p.Out, "%s%s #%d\n", strings.Repeat("  ", indent), formatFunc(f, p.Short), p.assigned[f])
		}
		return
	}

	// Assign UID if in expand mode and not yet assigned
	if p.Expand {
		if _, ok := p.assigned[f]; !ok {
			p.nextID++
			p.assigned[f] = p.nextID
		}
		fmt.Fprintf(p.Out, "%s%s #%d\n", strings.Repeat("  ", indent), formatFunc(f, p.Short), p.assigned[f])
	} else {
		fmt.Fprintf(p.Out, "%s%s\n", strings.Repeat("  ", indent), formatFunc(f, p.Short))
	}

	p.visited[f] = true

	if callees, ok := p.Graph[f]; ok {
		for _, callee := range callees {
			p.printRecursive(callee, indent+1)
		}
	}

	// Reset visited flag for this path to allow it to be printed in other branches
	p.visited[f] = false
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