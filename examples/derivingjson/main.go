package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	goscan "github.com/podhmo/go-scan"
)

//go:embed unmarshal.tmpl
var templateFile embed.FS

func main() {
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelDebug)
	opts := slog.HandlerOptions{Level: logLevel}
	handler := slog.NewTextHandler(os.Stderr, &opts)
	slog.SetDefault(slog.New(handler))

	var cwd string
	flag.StringVar(&cwd, "cwd", ".", "current working directory")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: derivingjson [options] <file_or_dir_path_1> [file_or_dir_path_2 ...]\n")
		fmt.Fprintf(os.Stderr, "Example (file): derivingjson examples/derivingjson/testdata/simple/models.go\n")
		fmt.Fprintf(os.Stderr, "Example (dir):  derivingjson examples/derivingjson/testdata/simple/\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	ctx := context.Background()
	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	if err := run(ctx, cwd, flag.Args()); err != nil {
		slog.ErrorContext(ctx, "toplevel error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context, cwd string, args []string) error {
	gscn, err := goscan.New(goscan.WithWorkDir(cwd))
	if err != nil {
		return fmt.Errorf("failed to create go-scan scanner: %w", err)
	}

	filesByPackage := make(map[string][]string)
	dirsToScan := []string{}

	for _, path := range args {
		stat, err := os.Stat(path)
		if err != nil {
			slog.ErrorContext(ctx, "Error accessing path", slog.String("path", path), slog.Any("error", err))
			continue
		}
		if stat.IsDir() {
			dirsToScan = append(dirsToScan, path)
		} else if strings.HasSuffix(path, ".go") {
			pkgDir := filepath.Dir(path)
			filesByPackage[pkgDir] = append(filesByPackage[pkgDir], path)
		} else {
			slog.WarnContext(ctx, "Argument is not a .go file or directory, skipping", slog.String("path", path))
		}
	}

	var successCount, errorCount int

	// Process directories
	for _, dirPath := range dirsToScan {
		slog.InfoContext(ctx, "Scanning directory", "path", dirPath)
		pkgInfo, err := gscn.ScanPackage(ctx, dirPath)
		if err != nil {
			slog.ErrorContext(ctx, "Error scanning package", "path", dirPath, slog.Any("error", err))
			errorCount++
			continue
		}
		if err := Generate(ctx, gscn, pkgInfo); err != nil {
			slog.ErrorContext(ctx, "Error generating code for package", "path", dirPath, slog.Any("error", err))
			errorCount++
		} else {
			slog.InfoContext(ctx, "Successfully generated UnmarshalJSON for package", "path", dirPath)
			successCount++
		}
	}

	// Process file groups
	for pkgDir, filePaths := range filesByPackage {
		slog.InfoContext(ctx, "Scanning files in package", "package", pkgDir, "files", filePaths)
		// Note: ScanFiles requires the package directory to be passed explicitly.
		pkgInfo, err := gscn.ScanFiles(ctx, filePaths)
		if err != nil {
			slog.ErrorContext(ctx, "Error scanning files", "package", pkgDir, slog.Any("error", err))
			errorCount++
			continue
		}
		if err := Generate(ctx, gscn, pkgInfo); err != nil {
			slog.ErrorContext(ctx, "Error generating code for files", "package", pkgDir, slog.Any("error",err))
			errorCount++
		} else {
			slog.InfoContext(ctx, "Successfully generated UnmarshalJSON for package", "package", pkgDir)
			successCount++
		}
	}

	slog.InfoContext(ctx, "Generation summary", slog.Int("successful_packages", successCount), slog.Int("failed_packages/files", errorCount))
	if errorCount > 0 {
		return fmt.Errorf("%d packages failed", errorCount)
	}
	return nil
}
