package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"

	goscan "github.com/podhmo/go-scan"
)

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "Enable debug logging for the analysis")
	flag.Parse()

	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	if err := run(logger); err != nil {
		logger.Error("docgen failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	const sampleAPIPath = "github.com/podhmo/go-scan/examples/docgen/sampleapi"

	overrides := createStubOverrides()

	s, err := goscan.New(
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
		goscan.WithExternalTypeOverrides(overrides),
	)
	if err != nil {
		return err
	}

	analyzer, err := NewAnalyzer(s, logger)
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
