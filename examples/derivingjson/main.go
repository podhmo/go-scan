package main

import (
	"context"
	"log/slog"
	"os"
)

func main() {
	ctx := context.Background() // Or your application's context

	if len(os.Args) <= 1 {
		slog.ErrorContext(ctx, "Usage: derivingjson <package_path>")
		slog.ErrorContext(ctx, "Example: derivingjson examples/derivingjson/testdata/simple") // Adjusted example path
		os.Exit(1)
	}
	pkgPath := os.Args[1] // Restore command line argument

	// Ensure the package path exists and is a directory
	stat, err := os.Stat(pkgPath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.ErrorContext(ctx, "Package path does not exist", slog.String("package_path", pkgPath))
		} else {
			slog.ErrorContext(ctx, "Error accessing package path", slog.String("package_path", pkgPath), slog.Any("error", err))
		}
		os.Exit(1)
	}
	if !stat.IsDir() {
		slog.ErrorContext(ctx, "Package path is not a directory", slog.String("package_path", pkgPath))
		os.Exit(1)
	}

	slog.InfoContext(ctx, "Generating UnmarshalJSON for package", slog.String("package_path", pkgPath))
	if err := Generate(pkgPath); err != nil { // Generate is in the same package
		slog.ErrorContext(ctx, "Error generating code", slog.Any("error", err))
		os.Exit(1)
	}
	slog.InfoContext(ctx, "Successfully generated UnmarshalJSON methods for package", slog.String("package_path", pkgPath))
}
