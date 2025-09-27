package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"

	"github.com/podhmo/go-scan/goscan"
	"github.com/podhmo/go-scan/symgo"
)

func main() {
	// 1. Setup test environment
	dir := "." // We are running inside temp_runner

	// 2. Create scanner and interpreter
	s, err := goscan.New(
		goscan.WithWorkDir(dir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		log.Fatalf("failed to create scanner: %v", err)
	}

	interp, err := symgo.NewInterpreter(s,
		symgo.WithLogger(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))),
	)
	if err != nil {
		log.Fatalf("failed to create interpreter: %v", err)
	}

	// 3. Scan the package to get all functions
	// The package path is now relative to the temp_runner/src directory
	pkgName := "example.com/recursion"
	pkgs, err := s.Scan(context.Background(), "src/"+pkgName)
	if err != nil {
		log.Fatalf("failed to scan package %q: %v", pkgName, err)
	}
	if len(pkgs) == 0 {
		log.Fatalf("package %q not found", pkgName)
	}
	pkg := pkgs[0]
	fmt.Printf("Successfully scanned package: %s\n", pkg.Name)
	fmt.Printf("Found %d functions to analyze\n", len(pkg.Functions))

	// 4. This is the core of the test: iterate and analyze each function.
	// This loop should panic if the bug is present.
	for _, fn := range pkg.Functions {
		fmt.Printf("Analyzing function: %s\n", fn.Name)
		ctx := context.Background()

		// Get the function object
		fnObj, ok := interp.FindObjectInPackage(ctx, pkg.ImportPath, fn.Name)
		if !ok {
			log.Printf("could not find function object for %s", fn.Name)
			continue
		}

		// Run the interpreter on this function.
		// We don't care about the result, just that it doesn't panic.
		interp.Apply(ctx, fnObj, nil, pkg)
	}

	fmt.Println("Analysis completed without panic.")
}