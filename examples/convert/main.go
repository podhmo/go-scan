package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/convert/generator"
	"github.com/podhmo/go-scan/examples/convert/parser"
	"github.com/podhmo/go-scan/scanner"
	"golang.org/x/tools/imports"
)

type FileWriter interface {
	WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error
}
type defaultFileWriter struct{}

func (w *defaultFileWriter) WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

type contextKey string

const FileWriterKey contextKey = "fileWriter"

func main() {
	var (
		pkgpath       = flag.String("pkg", "", "target package path (e.g. example.com/m/models)")
		workdir       = flag.String("cwd", ".", "current working directory")
		output        = flag.String("output", "generated.go", "output file name")
		pkgname       = flag.String("pkgname", "", "package name for the generated file (default: inferred from output dir)")
		outputPkgPath = flag.String("output-pkgpath", "", "full package import path for the generated file (e.g. example.com/m/generated)")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: convert -pkg <package_path> [-cwd <dir>] [-output <filename>] [-pkgname <name>] [-output-pkgpath <path>]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *pkgpath == "" {
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, FileWriterKey, &defaultFileWriter{})
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	if err := run(ctx, *pkgpath, *workdir, *output, *pkgname, *outputPkgPath); err != nil {
		slog.ErrorContext(ctx, "Error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context, pkgpath, workdir, output, pkgname, outputPkgPath string) error {
	// Define an external type override for time.Time to avoid scanning the stdlib time package,
	// which can cause issues in certain build contexts (like tests).
	overrides := scanner.ExternalTypeOverride{
		"time.Time": &scanner.TypeInfo{
			Name:    "Time",
			PkgPath: "time",
			Kind:    scanner.StructKind, // Treat it as a struct, not an interface
			Underlying: &scanner.FieldType{
				Name:               "Time",
				PkgName:            "time",
				IsResolvedByConfig: true,
			},
		},
	}

	// Create a scanner with the module resolver and the external type override.
	s, err := goscan.New(
		goscan.WithWorkDir(workdir),
		goscan.WithGoModuleResolver(),
		goscan.WithExternalTypeOverrides(overrides),
	)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	// Use ScanPackageByImport to leverage the scanner's configured locator.
	scannedPkg, err := s.ScanPackageByImport(ctx, pkgpath)
	if err != nil {
		return fmt.Errorf("failed to scan package %q: %w", pkgpath, err)
	}

	slog.DebugContext(ctx, "Parsing package", "path", scannedPkg.ImportPath)
	info, err := parser.Parse(ctx, s, scannedPkg)
	if err != nil {
		return fmt.Errorf("failed to parse package info: %w", err)
	}

	if len(info.ConversionPairs) == 0 {
		slog.InfoContext(ctx, "No @derivingconvert annotations found, nothing to generate.")
		return nil
	}
	slog.DebugContext(ctx, "Found conversion pairs", "count", len(info.ConversionPairs))

	if pkgname == "" {
		pkgname = info.PackageName
	}

	// Override package name and path if provided
	if pkgname != "" {
		info.PackageName = pkgname
	}
	if outputPkgPath != "" {
		info.PackagePath = outputPkgPath
	}

	slog.DebugContext(ctx, "Generating code", "package", info.PackageName, "pkgpath", info.PackagePath)
	generatedCode, err := generator.Generate(s, info)
	if err != nil {
		return fmt.Errorf("failed to generate code: %w", err)
	}

	writer, ok := ctx.Value(FileWriterKey).(FileWriter)
	if !ok {
		return fmt.Errorf("file writer not found in context")
	}

	slog.DebugContext(ctx, "Writing output", "file", output)

	formatted, err := formatCode(ctx, output, generatedCode)
	if err != nil {
		slog.WarnContext(ctx, "code formatting failed, using unformatted code", "error", err)
		// Even if formatting fails, write the unformatted code.
		if writeErr := writer.WriteFile(ctx, output, generatedCode, 0644); writeErr != nil {
			return fmt.Errorf("failed to write (unformatted) generated code to %s: %w", output, writeErr)
		}
		return nil // Do not treat as an error.
	}

	if err := writer.WriteFile(ctx, output, formatted, 0644); err != nil {
		return fmt.Errorf("failed to write formatted code to %s: %w", output, err)
	}

	slog.InfoContext(ctx, "Successfully generated conversion functions", "output", output)
	return nil
}

func formatCode(ctx context.Context, filename string, src []byte) ([]byte, error) {
	// The first argument to Process is a filename, which is used for context
	// when resolving imports. We can use the output file name here.
	// The third argument is options, which we can leave as nil for now.
	formatted, err := imports.Process(filename, src, nil)
	if err != nil {
		return nil, fmt.Errorf("goimports failed: %w", err)
	}
	return formatted, nil
}
