package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func main() {
	// 1. Define and parse command-line flags.
	var targetFunc string
	flag.StringVar(&targetFunc, "target", "", "Target function to trace calls to (e.g., example.com/mylib.MyFunction)")
	var logLevel = slog.LevelWarn
	flag.TextVar(&logLevel, "log-level", &logLevel, "Log level (debug, info, warn, error)")

	flag.Parse()

	if targetFunc == "" {
		fmt.Fprintln(os.Stderr, "Error: -target flag is required")
		flag.Usage()
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: &logLevel}))

	// The package patterns to scan are taken from the remaining arguments
	pkgPatterns := flag.Args()
	if len(pkgPatterns) == 0 {
		pkgPatterns = []string{"./..."} // Default to scanning the current module
	}

	if err := run(context.Background(), os.Stdout, logger, targetFunc, pkgPatterns); err != nil {
		log.Fatalf("Error: %+v", err)
	}
}

func getFuncTargetName(f *scanner.FunctionInfo) string {
	if f == nil {
		return ""
	}
	if f.Receiver == nil {
		return fmt.Sprintf("%s.%s", f.PkgPath, f.Name)
	}

	// Method
	recvType := f.Receiver.Type
	recvString := recvType.Name
	if recvType.IsPointer {
		// Ensure the name is wrapped in parens for pointer types, e.g., "(*MyType)"
		// The scanner's string representation might already do this, but we can be defensive.
		if !strings.HasPrefix(recvString, "(*") {
			recvString = fmt.Sprintf("(*%s)", recvString)
		}
	}
	return fmt.Sprintf("%s.%s.%s", f.PkgPath, recvString, f.Name)
}

func run(ctx context.Context, out io.Writer, logger *slog.Logger, targetFunc string, pkgPatterns []string) error {
	logger.Info("starting call-trace", "target", targetFunc, "packages", pkgPatterns)

	// a more robust way to extract the package path, handling methods like
	// "pkg.path.(*Type).Method"
	var pkgPath string
	lastDot := strings.LastIndex(targetFunc, ".")
	if lastDot == -1 {
		return fmt.Errorf("invalid target function format: %q. Expected format: <pkg>.<func>", targetFunc)
	}
	// Check if it's a method with a receiver like (*Type) or Type
	if endParen := strings.LastIndex(targetFunc[:lastDot], ")"); endParen != -1 && endParen > strings.LastIndex(targetFunc[:lastDot], "(") {
		// e.g. "pkg.path.(*Type).Method"
		// lastDot is at ".Method"
		// endParen is at ")" in "(*Type)"
		// we need to find the dot before the receiver type
		pkgPathEnd := strings.LastIndex(targetFunc[:endParen], ".")
		if pkgPathEnd != -1 {
			pkgPath = targetFunc[:pkgPathEnd]
		} else {
			// This could be a type in the "main" package of the module root
			// where there's no preceding dot.
			// e.g. "my-module.main.(*MyType).MyMethod"
			// In this case, we search for the package path up to the opening parenthesis.
			openParen := strings.LastIndex(targetFunc[:lastDot], "(")
			if openParen != -1 {
				pkgPath = targetFunc[:openParen]
				// trim the trailing dot if it exists
				pkgPath = strings.TrimSuffix(pkgPath, ".")

			} else {
				return fmt.Errorf("could not extract pkg path from method target %q", targetFunc)
			}
		}
	} else {
		// simple function, or method on non-pointer receiver without parens
		pkgPath = targetFunc[:lastDot]
	}

	// 2. Initialize the scanner.
	s, err := goscan.New(
		goscan.WithLogger(logger),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	// 3. Scan all specified packages to build a complete dependency graph.
	for _, pattern := range pkgPatterns {
		if _, err := s.Scan(ctx, pattern); err != nil {
			return fmt.Errorf("failed to scan packages for pattern %q: %w", pattern, err)
		}
	}

	// 4. Build the reverse dependency map.
	revDepMap, err := s.Walker.BuildReverseDependencyMap(ctx)
	if err != nil {
		return fmt.Errorf("could not build reverse dependency map: %w", err)
	}

	// 5. Find all packages that could possibly call the target function.
	analysisScope := make(map[string]bool)
	queue := []string{pkgPath}
	analysisScope[pkgPath] = true
	head := 0
	for head < len(queue) {
		currentPkg := queue[head]
		head++

		importers := revDepMap[currentPkg]
		for _, importer := range importers {
			if !analysisScope[importer] {
				analysisScope[importer] = true
				queue = append(queue, importer)
			}
		}
	}

	// 6. Initialize the symgo interpreter.
	interp, err := symgo.NewInterpreter(s,
		symgo.WithLogger(logger.WithGroup("symgo")),
		symgo.WithScanPolicy(func(importPath string) bool {
			return analysisScope[importPath]
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create interpreter: %w", err)
	}

	// 7. Register a default intrinsic to trace all function calls.
	var directHits [][]*object.CallFrame
	interp.RegisterDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
		calleeObj := args[0]
		var calleeFunc *scanner.FunctionInfo

		switch f := calleeObj.(type) {
		case *object.Function:
			calleeFunc = f.Def
		case *object.SymbolicPlaceholder:
			calleeFunc = f.UnderlyingFunc
		}

		if calleeFunc != nil {
			calleeName := getFuncTargetName(calleeFunc)
			if calleeName == targetFunc {
				stack := i.CallStack()
				directHits = append(directHits, stack)
			}
		}
		return nil
	})

	// 8. Find and analyze all main functions in the analysis scope.
	allScannedPkgs := s.AllSeenPackages()
	for pkgPath, p := range allScannedPkgs {
		if !analysisScope[pkgPath] {
			continue
		}

		var mainFunc *scanner.FunctionInfo
		for _, f := range p.Functions {
			if f.Name == "main" {
				mainFunc = f
				break
			}
		}

		if mainFunc != nil && p.Name == "main" {
			logger.Info("analyzing entry point", "package", p.ImportPath)
			eval := interp.EvaluatorForTest()
			pkgObj, err := eval.GetOrLoadPackageForTest(ctx, p.ImportPath)
			if err != nil {
				logger.Warn("failed to load package for analysis", "pkg", p.ImportPath, "error", err)
				continue
			}
			fnObj := eval.GetOrResolveFunctionForTest(ctx, pkgObj, mainFunc)
			interp.Apply(ctx, fnObj, nil, p)
		}
	}

	interp.Finalize(ctx)

	// 9. Print the results.
	if len(directHits) == 0 {
		fmt.Fprintf(out, "No calls to %s found.\n", targetFunc)
		return nil
	}

	fmt.Fprintf(out, "Found %d call stacks to %s:\n\n", len(directHits), targetFunc)
	fset := s.Fset()
	for i, stack := range directHits {
		fmt.Fprintf(out, "--- Stack %d ---\n", i+1)
		for _, frame := range stack {
			fmt.Fprintln(out, frame.Format(fset))
		}
		fmt.Fprintln(out)
	}

	return nil
}
