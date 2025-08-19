package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"

	goscan "github.com/podhmo/go-scan"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("docgen failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	const sampleAPIPath = "github.com/podhmo/go-scan/examples/docgen/sampleapi"

	s, err := goscan.New(
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		return err
	}

	analyzer, err := NewAnalyzer(s)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := analyzer.Analyze(ctx, sampleAPIPath, "NewServeMux"); err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(analyzer.OpenAPI)
}
