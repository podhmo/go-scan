package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	// "strings" // Unused
)

func main() {
	ctx := context.Background() // Or your application's context

	// The main program now implicitly works from the "examples/derivingjson" directory.
	// It will generate for "./models" and then for subdirectories under "./testdata".

	baseDir := "." // Assuming this generator is run from examples/derivingjson

	// 1. Generate for ./models directory
	modelsDir := filepath.Join(baseDir, "models")
	slog.InfoContext(ctx, "Generating UnmarshalJSON for models directory", slog.String("package_path", modelsDir))
	if err := ensureDirAndGenerate(ctx, modelsDir, modelsDir); err != nil {
		slog.ErrorContext(ctx, "Error generating for models", slog.String("package_path", modelsDir), slog.Any("error", err))
		os.Exit(1)
	}
	slog.InfoContext(ctx, "Successfully generated for models directory", slog.String("package_path", modelsDir))

	// 2. Generate for ./testdata subdirectories, outputting to the subdirectory itself
	testdataBaseDir := filepath.Join(baseDir, "testdata")

	entries, err := os.ReadDir(testdataBaseDir)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to read testdata directory", slog.String("path", testdataBaseDir), slog.Any("error", err))
		os.Exit(1)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			pkgPath := filepath.Join(testdataBaseDir, entry.Name())
			// Output directory is the same as the input package path for testdata subdirectories

			slog.InfoContext(ctx, "Generating UnmarshalJSON for testdata subdirectory",
				slog.String("package_path", pkgPath))

			if err := ensureDirAndGenerate(ctx, pkgPath, pkgPath); err != nil {
				slog.ErrorContext(ctx, "Error generating for testdata subdirectory",
					slog.String("package_path", pkgPath),
					slog.Any("error", err))
				// Decide if one failure should stop all: for now, yes.
				os.Exit(1)
			}
			slog.InfoContext(ctx, "Successfully generated for testdata subdirectory",
				slog.String("package_path", pkgPath))
		}
	}

	slog.InfoContext(ctx, "All generation tasks complete.")
}

// ensureDirAndGenerate checks if the input path is a directory and then calls Generate.
func ensureDirAndGenerate(ctx context.Context, pkgInputPath string, pkgOutputPath string) error {
	stat, err := os.Stat(pkgInputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("package input path does not exist: %s", pkgInputPath)
		}
		return fmt.Errorf("error accessing package input path %s: %w", pkgInputPath, err)
	}
	if !stat.IsDir() {
		return fmt.Errorf("package input path is not a directory: %s", pkgInputPath)
	}

	// If pkgOutputPath is for "models", it's the same as input.
	// If for "testdata/foo", output is "integratetest/foo".
	// The generator's Generate function needs the original pkgInputPath for scanning,
	// and pkgOutputPath for placing the file.
	// The import paths inside the generated file must be correct.
	// If generator.go uses pkgInfo.ImportPath for types, and if go-scan correctly
	// resolves these to full module paths (e.g., "github.com/your/module/examples/derivingjson/testdata/simple"),
	// then the generated code should be fine even if it lives in `integratetest/simple`.
	// The crucial part is that `go build` or `go test` from `integratetest/simple` can resolve
	// "github.com/your/module/examples/derivingjson/testdata/simple". This is standard.

	// The package declaration inside the generated `_deriving.go` file needs to be correct.
	// It should be `package <leaf_dir_name_of_pkgInputPath>`.
	// Example: if pkgInputPath is "testdata/simple", generated package is "package simple".
	// The Generate function already uses `pkgInfo.Name` for this, which should be correct.

	return Generate(ctx, pkgInputPath, pkgOutputPath)
}
