package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"example.com/convert/generator"
	"example.com/convert/parser"
	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/locator"
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
		pkgpath = flag.String("pkg", "", "target package path (e.g. example.com/m/models)")
		workdir = flag.String("cwd", ".", "current working directory")
		output  = flag.String("output", "generated.go", "output file name")
		pkgname = flag.String("pkgname", "", "package name for the generated file (default: inferred from output dir)")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: convert -pkg <package_path> [-cwd <dir>] [-output <filename>] [-pkgname <name>]\n")
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

	if err := run(ctx, *pkgpath, *workdir, *output, *pkgname); err != nil {
		slog.ErrorContext(ctx, "Error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context, pkgpath, workdir, output, pkgname string) error {
	s, err := goscan.New(goscan.WithWorkDir(workdir))
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	l, err := locator.New(workdir, nil)
	if err != nil {
		return fmt.Errorf("failed to create locator: %w", err)
	}
	pkgDir, err := l.FindPackageDir(pkgpath)
	if err != nil {
		return fmt.Errorf("could not find package dir for %q: %w", pkgpath, err)
	}

	scannedPkg, err := s.ScanPackage(ctx, pkgDir)
	if err != nil {
		return fmt.Errorf("failed to scan package: %w", err)
	}

	slog.DebugContext(ctx, "Parsing package", "path", scannedPkg.ImportPath)
	info, err := parser.Parse(ctx, scannedPkg)
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

	slog.DebugContext(ctx, "Generating code", "package", pkgname)
	generatedCode, err := generator.Generate(s, info)
	if err != nil {
		return fmt.Errorf("failed to generate code: %w", err)
	}

	writer, ok := ctx.Value(FileWriterKey).(FileWriter)
	if !ok {
		return fmt.Errorf("file writer not found in context")
	}

	slog.DebugContext(ctx, "Writing output", "file", output)

	cmd := exec.CommandContext(ctx, "goimports")
	cmd.Stdin = bytes.NewReader(generatedCode)
	var out bytes.Buffer
	cmd.Stdout = &out
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		// fallback to original generated code if goimports fails
		slog.WarnContext(ctx, "goimports failed, using unformatted code", "error", err, "stderr", errBuf.String())
		if err := writer.WriteFile(ctx, output, generatedCode, 0644); err != nil {
			return fmt.Errorf("failed to write (unformatted) generated code to %s: %w", output, err)
		}
	} else {
		if err := writer.WriteFile(ctx, output, out.Bytes(), 0644); err != nil {
			return fmt.Errorf("failed to write formatted code to %s: %w", output, err)
		}
	}

	slog.InfoContext(ctx, "Successfully generated conversion functions", "output", output)
	return nil
}
