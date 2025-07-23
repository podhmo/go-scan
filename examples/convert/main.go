package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"

	"example.com/convert/generator"
	"example.com/convert/parser"
)

func main() {
	fmt.Println("\n--- Running go-scan converter generator ---")
	if err := runGenerate(context.Background()); err != nil {
		log.Fatalf("Error running generator: %v", err)
	}
	fmt.Println("Generator finished successfully.")
}

func runGenerate(ctx context.Context) error {
	sourcePath := "./models/source"
	s, err := goscan.New(goscan.WithWorkDir(filepath.Dir(sourcePath)))
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	pkg, err := s.ScanPackage(ctx, sourcePath)
	if err != nil {
		return fmt.Errorf("failed to scan package %s: %w", sourcePath, err)
	}

	return Generate(ctx, s, pkg)
}

// Generate produces converter code for the given package.
func Generate(ctx context.Context, s *goscan.Scanner, pkgInfo *scanner.PackageInfo) error {
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

	// This path assumes the 'converter' package is a sibling of the 'models' package.
	converterPkgDir := goscan.NewPackageDirectory(filepath.Join(pkgInfo.Path, "..", "..", "converter"), "converter")

	gf := goscan.GoFile{
		PackageName: "converter",
		CodeSet:     string(generatedCode),
	}

	// The generated file will be named 'generated_converters.go' to distinguish it from manual files.
	return converterPkgDir.SaveGoFile(ctx, gf, "generated_converters.go")
}
