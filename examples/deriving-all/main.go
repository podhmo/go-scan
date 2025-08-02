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
)

type GeneratorFunc func(context.Context, *goscan.Scanner, *scanner.PackageInfo, *goscan.ImportManager) ([]byte, error)

func main() {
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelDebug)
	opts := slog.HandlerOptions{Level: logLevel}
	handler := slog.NewTextHandler(os.Stderr, &opts)
	slog.SetDefault(slog.New(handler))

	var cwd string
	flag.StringVar(&cwd, "cwd", ".", "current working directory")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: deriving-all [options] <file_or_dir_path_1> [file_or_dir_path_2 ...]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	ctx := context.Background()
	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	gscn, err := goscan.New(goscan.WithWorkDir(cwd))
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
		if err := outputDir.SaveGoFile(ctx, goFile, outputFilename); err != nil {
			slog.ErrorContext(ctx, "Failed to save generated file for package", "path", pkgInfo.Path, slog.Any("error", err))
			errorCount++
		} else {
			slog.InfoContext(ctx, "Successfully generated combined code for package", "path", pkgInfo.Path)
			successCount++
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
