package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/derivingjson/gen"
	"github.com/podhmo/go-scan/scanner"
)

func main() {
	var (
		cwd      string
		dryRun   bool
		inspect  bool
		logLevel = new(slog.LevelVar)
	)

	flag.StringVar(&cwd, "cwd", ".", "current working directory")
	flag.BoolVar(&dryRun, "dry-run", false, "don't write files, just print to stdout")
	flag.BoolVar(&inspect, "inspect", false, "enable inspection logging for annotations")
	flag.Var(logLevel, "log-level", "set log level (debug, info, warn, error)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: derivingjson [options] <file_or_dir_path_1> [file_or_dir_path_2 ...]\n")
		fmt.Fprintf(os.Stderr, "Example (file): derivingjson examples/derivingjson/testdata/simple/models.go\n")
		fmt.Fprintf(os.Stderr, "Example (dir):  derivingjson examples/derivingjson/testdata/simple/\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	opts := slog.HandlerOptions{Level: logLevel}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &opts))
	slog.SetDefault(logger)

	ctx := context.Background()
	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	scannerOptions := []goscan.ScannerOption{
		goscan.WithWorkDir(cwd),
		goscan.WithDryRun(dryRun),
		goscan.WithInspect(inspect),
		goscan.WithLogger(logger),
	}
	gscn, err := goscan.New(scannerOptions...)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create go-scan scanner", slog.Any("error", err))
		os.Exit(1)
	}

	filesByPackage := make(map[string][]string)
	dirsToScan := []string{}

	for _, path := range flag.Args() {
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

	processPackage := func(pkgInfo *scanner.PackageInfo) {
		if pkgInfo == nil {
			slog.ErrorContext(ctx, "Scanned package info is nil")
			errorCount++
			return
		}

		importManager := goscan.NewImportManager(pkgInfo)
		code, err := gen.Generate(ctx, gscn, pkgInfo, importManager)
		if err != nil {
			slog.ErrorContext(ctx, "Error generating code for package", "path", pkgInfo.Path, slog.Any("error", err))
			errorCount++
			return
		}

		if len(code) == 0 {
			slog.InfoContext(ctx, "No code generated for package", "path", pkgInfo.Path)
			successCount++
			return
		}

		outputDir := goscan.NewPackageDirectory(pkgInfo.Path, pkgInfo.Name)
		goFile := goscan.GoFile{
			PackageName: pkgInfo.Name,
			Imports:     importManager.Imports(),
			CodeSet:     string(code),
		}

		outputFilename := fmt.Sprintf("%s_deriving.go", strings.ToLower(pkgInfo.Name))
		if gscn.DryRun {
			slog.InfoContext(ctx, "Dry run: skipping file write", "path", filepath.Join(outputDir.Path, outputFilename))
			fmt.Fprintf(os.Stdout, "---\n// file: %s\n---\n", filepath.Join(outputDir.Path, outputFilename))
			if err := goFile.FormatAndWrite(os.Stdout); err != nil {
				slog.ErrorContext(ctx, "Failed to format and write to stdout", "path", pkgInfo.Path, slog.Any("error", err))
				errorCount++
			} else {
				successCount++
			}
		} else {
			if err := outputDir.SaveGoFile(ctx, goFile, outputFilename); err != nil {
				slog.ErrorContext(ctx, "Failed to save generated file for package", "path", pkgInfo.Path, slog.Any("error", err))
				errorCount++
			} else {
				slog.InfoContext(ctx, "Successfully generated UnmarshalJSON for package", "path", pkgInfo.Path)
				successCount++
			}
		}
	}

	// Process directories
	for _, dirPath := range dirsToScan {
		slog.InfoContext(ctx, "Scanning directory", "path", dirPath)
		pkgInfo, err := gscn.ScanPackage(ctx, dirPath)
		if err != nil {
			slog.ErrorContext(ctx, "Error scanning package", "path", dirPath, slog.Any("error", err))
			errorCount++
			continue
		}
		processPackage(pkgInfo)
	}

	// Process file groups
	for pkgDir, filePaths := range filesByPackage {
		slog.InfoContext(ctx, "Scanning files in package", "package", pkgDir, "files", filePaths)
		pkgInfo, err := gscn.ScanFiles(ctx, filePaths)
		if err != nil {
			slog.ErrorContext(ctx, "Error scanning files", "package", pkgDir, slog.Any("error", err))
			errorCount++
			continue
		}
		processPackage(pkgInfo)
	}

	slog.InfoContext(ctx, "Generation summary", slog.Int("successful_packages", successCount), slog.Int("failed_packages/files", errorCount))
	if errorCount > 0 {
		os.Exit(1)
	}
}
