package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	goscan "github.com/podhmo/go-scan"
	bindgen "github.com/podhmo/go-scan/examples/derivingbind/gen"
	jsongen "github.com/podhmo/go-scan/examples/derivingjson/gen"
	"github.com/podhmo/go-scan/scanner"
	"golang.org/x/tools/imports"
)

type GeneratorFunc func(context.Context, *goscan.Scanner, *scanner.PackageInfo, *goscan.ImportManager) ([]byte, error)

func formatCode(ctx context.Context, filename string, src []byte) ([]byte, error) {
	formatted, err := imports.Process(filename, src, nil)
	if err != nil {
		return nil, fmt.Errorf("goimports failed: %w", err)
	}
	return formatted, nil
}

func main() {
	var (
		cwd      string
		dryRun   bool
		inspect  bool
		logLevel string
	)

	flag.StringVar(&cwd, "cwd", ".", "current working directory")
	flag.BoolVar(&dryRun, "dry-run", false, "perform a dry run without writing files")
	flag.BoolVar(&inspect, "inspect", false, "enable inspection logging for annotation scanning")
	flag.StringVar(&logLevel, "log-level", "info", "set the log level (debug, info, warn, error)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: deriving-all [options] <file_or_dir_path_1> [file_or_dir_path_2 ...]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// Configure logger
	var level slog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		slog.Error("Invalid log level", slog.String("level", logLevel))
		os.Exit(1)
	}
	opts := slog.HandlerOptions{Level: level}
	handler := slog.NewTextHandler(os.Stderr, &opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)

	ctx := context.Background()
	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	// Configure scanner
	scanOptions := []goscan.ScannerOption{
		goscan.WithWorkDir(cwd),
		goscan.WithLogger(logger),
	}
	if dryRun {
		scanOptions = append(scanOptions, goscan.WithDryRun())
	}
	if inspect {
		scanOptions = append(scanOptions, goscan.WithInspect())
	}

	gscn, err := goscan.New(scanOptions...)
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

	generators := []GeneratorFunc{
		jsongen.Generate,
		bindgen.Generate,
	}

	processPackage := func(pkgInfo *scanner.PackageInfo) {
		if pkgInfo == nil {
			slog.ErrorContext(ctx, "Scanned package info is nil")
			errorCount++
			return
		}

		importManager := goscan.NewImportManager(pkgInfo)
		var masterCode bytes.Buffer
		var totalErrors []error

		for _, generate := range generators {
			code, err := generate(ctx, gscn, pkgInfo, importManager)
			if err != nil {
				totalErrors = append(totalErrors, err)
				continue
			}
			if len(code) > 0 {
				masterCode.Write(code)
				masterCode.WriteString("\n\n")
			}
		}

		if len(totalErrors) > 0 {
			slog.ErrorContext(ctx, "Errors during code generation", "path", pkgInfo.Path, "errors", totalErrors)
			errorCount++
			return
		}

		if masterCode.Len() == 0 {
			slog.InfoContext(ctx, "No code generated for package", "path", pkgInfo.Path)
			successCount++
			return
		}

		outputDir := goscan.NewPackageDirectory(pkgInfo.Path, pkgInfo.Name)
		goFile := goscan.GoFile{
			PackageName: pkgInfo.Name,
			Imports:     importManager.Imports(),
			CodeSet:     masterCode.String(),
		}

		outputFilename := fmt.Sprintf("%s_deriving.go", strings.ToLower(pkgInfo.Name))

		// Manually construct the file content, but without the import block.
		// Let goimports handle the imports entirely.
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "package %s\n\n", goFile.PackageName)
		buf.WriteString(goFile.CodeSet)

		// Format the code using goimports
		formatted, err := formatCode(ctx, outputFilename, buf.Bytes())
		if err != nil {
			slog.WarnContext(ctx, "code formatting failed, using unformatted code", "path", pkgInfo.Path, "error", err)
			formatted = buf.Bytes() // Fallback to unformatted code
		}

		// Write the file
		outputPath := filepath.Join(outputDir.Path, outputFilename)
		if gscn.DryRun() {
			slog.InfoContext(ctx, "[dry-run] Skipping file write", "path", outputPath)
			fmt.Println("--- GENERATED CODE ---")
			fmt.Println(string(formatted))
			fmt.Println("--- END GENERATED CODE ---")
			successCount++
		} else {
			if err := os.WriteFile(outputPath, formatted, 0644); err != nil {
				slog.ErrorContext(ctx, "Failed to save generated file for package", "path", pkgInfo.Path, slog.Any("error", err))
				errorCount++
			} else {
				slog.InfoContext(ctx, "Successfully generated combined code for package", "path", pkgInfo.Path)
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
