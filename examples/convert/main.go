package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"

	"example.com/convert/generator"
	"example.com/convert/parser"
)

func main() {
	// Part 1: Run existing examples
	// runConversionExamples() // Commented out during refactoring

	// Part 2: Generator Prototype
	fmt.Println("\n--- Generator Prototype ---")
	if err := runGenerate(context.Background()); err != nil {
		log.Fatalf("Error running generator: %v", err)
	}
}

func runGenerate(ctx context.Context) error {
	sourcePath := "./models/source"
	s, err := goscan.New(goscan.WithWorkDir(filepath.Dir(sourcePath))) // Use parent dir as workdir
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	// Scan the package containing the source models.
	// The scanner will automatically resolve dependencies, including the destination package.
	pkg, err := s.ScanPackage(ctx, sourcePath)
	if err != nil {
		return fmt.Errorf("failed to scan package %s: %w", sourcePath, err)
	}

	return Generate(ctx, s, pkg)
}

// Generate produces converter code for the given package.
func Generate(ctx context.Context, s *goscan.Scanner, pkgInfo *scanner.PackageInfo) error {
	// The scanner itself can act as the resolver.
	pairs, err := parser.Parse(ctx, pkgInfo, s)
	if err != nil {
		return fmt.Errorf("failed to parse conversion pairs: %w", err)
	}

	if len(pairs) == 0 {
		fmt.Println("No @derivingconvert annotations found.")
		return nil
	}

	generatedCode, err := generator.Generate("converter", pairs, pkgInfo)
	if err != nil {
		return fmt.Errorf("failed to generate converter code: %w", err)
	}

	// Use goscan.SaveGoFile to allow interception by scantest
	converterPkgDir := goscan.NewPackageDirectory(filepath.Join(pkgInfo.Path, "..", "converter"), "converter")

	gf := goscan.GoFile{
		PackageName: "converter",
		CodeSet:     string(generatedCode),
	}

	return converterPkgDir.SaveGoFile(ctx, gf, "generated_converters.go")
}

// printJSON is a helper to pretty-print structs as JSON.
// This is currently unused but might be useful for debugging later.
func printJSON(data interface{}) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("Error marshalling to JSON: %v\n", err)
		return
	}
	fmt.Println(string(jsonData))
}
