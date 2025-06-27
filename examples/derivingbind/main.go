package main

import (
	"context"
	"log/slog"
	"os"
)

func main() {
	ctx := context.Background() // Or your application's context

	if len(os.Args) <= 1 {
		slog.ErrorContext(ctx, "Usage: derivingbind <package_path>")
		slog.ErrorContext(ctx, "Example: derivingbind examples/derivingbind/testdata/simple")
		os.Exit(1)
	}
	pkgPath := os.Args[1]

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

	slog.InfoContext(ctx, "Generating Bind method for package", slog.String("package_path", pkgPath))
	if err := Generate(ctx, pkgPath); err != nil { // Generate will be in generator.go
		slog.ErrorContext(ctx, "Error generating code", slog.Any("error", err))
		os.Exit(1)
	}
	slog.InfoContext(ctx, "Successfully generated Bind methods for package", slog.String("package_path", pkgPath))
}
