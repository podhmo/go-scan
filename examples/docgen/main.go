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
	var debugFunc string
	flag.StringVar(&debugFunc, "debug-analysis", "", "The name of the function to enable debug logging for")
	flag.Parse()

	logLevel := slog.LevelInfo
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	if err := run(logger, debugFunc); err != nil {
		logger.Error("docgen failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, debugFunc string) error {
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

	analyzer, err := NewAnalyzer(s, logger, debugFunc)
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
