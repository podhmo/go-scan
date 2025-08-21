package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"

	goscan "github.com/podhmo/go-scan"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		debug        bool
		format       string
		patternsPath string
	)
	flag.BoolVar(&debug, "debug", false, "Enable debug logging for the analysis")
	flag.StringVar(&format, "format", "json", "Output format (json or yaml)")
	flag.StringVar(&patternsPath, "patterns", "examples/docgen/patterns.go", "Path to the Go file containing analysis patterns")
	flag.Parse()

	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	if err := run(logger, format, patternsPath); err != nil {
		logger.Error("docgen failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, format string, patternsPath string) error {
	const sampleAPIPath = "github.com/podhmo/go-scan/examples/docgen/sampleapi"

	var customPatterns []Pattern
	var err error
	if patternsPath != "" {
		customPatterns, err = LoadPatterns(patternsPath)
		if err != nil {
			return fmt.Errorf("failed to load custom patterns: %w", err)
		}
		logger.Info("loaded custom patterns", "count", len(customPatterns), "path", patternsPath)
	}

	overrides := createStubOverrides()

	s, err := goscan.New(
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
		goscan.WithExternalTypeOverrides(overrides),
	)
	if err != nil {
		return err
	}

	analyzer, err := NewAnalyzer(s, logger, customPatterns)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := analyzer.Analyze(ctx, sampleAPIPath, "NewServeMux"); err != nil {
		return err
	}

	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(analyzer.OpenAPI)
	case "yaml":
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(analyzer.OpenAPI)
	default:
		return fmt.Errorf("unsupported format: %q", format)
	}
}
