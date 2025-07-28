package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"example.com/convert/generator"
	"example.com/convert/parser"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

// ContextKey is a private type for context keys to avoid collisions.
type ContextKey string

const (
	// FileWriterKey is the context key for the file writer interceptor.
	FileWriterKey = ContextKey("fileWriter")
)

// FileWriter is an interface for writing files, allowing for interception during tests.
type FileWriter interface {
	WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error
}

// WriteFile is a context-aware file writing function.
func WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	if writer, ok := ctx.Value(FileWriterKey).(FileWriter); ok {
		return writer.WriteFile(ctx, path, data, perm)
	}
	return os.WriteFile(path, data, perm)
}

func main() {
	var (
		input   = flag.String("input", "", "input package path (e.g., example.com/convert/sampledata/source)")
		output  = flag.String("output", "generated.go", "output file name")
		pkgname = flag.String("pkgname", "main", "package name for the generated file")
	)
	flag.Parse()

	if *input == "" {
		log.Fatal("-input is required")
	}

	if err := run(context.Background(), *input, *output, *pkgname); err != nil {
		log.Fatalf("!! %+v", err)
	}
}

func run(ctx context.Context, input, output, pkgname string) error {
	s, err := goscan.New()
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	pkg, err := s.ScanPackageByImport(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to scan package %s: %w", input, err)
	}

	return Generate(ctx, s, pkg, output, pkgname)
}

// Generate produces converter code for the given package.
func Generate(ctx context.Context, s *goscan.Scanner, pkgInfo *scanner.PackageInfo, output, pkgname string) error {
	info, err := parser.Parse(ctx, pkgInfo.ImportPath, ".")
	if err != nil {
		return fmt.Errorf("failed to parse conversion pairs: %w", err)
	}

	if len(info.ConversionPairs) == 0 {
		fmt.Println("No @derivingconvert annotations found.")
		return nil
	}

	generatedCode, err := generator.Generate(s, pkgname, info.ConversionPairs, pkgInfo)
	if err != nil {
		return fmt.Errorf("failed to generate converter code: %w", err)
	}

	return WriteFile(ctx, output, generatedCode, 0644)
}
