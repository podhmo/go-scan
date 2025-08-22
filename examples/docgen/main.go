package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		debug        bool
		format       string
		patternsFile string
	)
	flag.BoolVar(&debug, "debug", false, "Enable debug logging for the analysis")
	flag.StringVar(&format, "format", "json", "Output format (json or yaml)")
	flag.StringVar(&patternsFile, "patterns", "", "Path to a Go file with custom pattern configurations")
	flag.Parse()

	logLevel := slog.LevelWarn
	if debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	if err := run(logger, format, patternsFile); err != nil {
		logger.Error("docgen failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, format string, patternsFile string) error {
	if flag.NArg() == 0 {
		return fmt.Errorf("required argument: <package-path>")
	}
	sampleAPIPath := flag.Arg(0)

	s, err := goscan.New(
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		return err
	}

	customPatterns, err := loadCustomPatterns(patternsFile, logger, s)
	if err != nil {
		return err
	}

	var opts []any
	for _, p := range customPatterns {
		opts = append(opts, p)
	}
	analyzer, err := NewAnalyzer(s, logger, opts...)
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

func loadCustomPatterns(filePath string, logger *slog.Logger, scanner *goscan.Scanner) ([]patterns.Pattern, error) {
	if filePath == "" {
		return nil, nil
	}
	logger.Info("loading custom patterns", "file", filePath)
	return LoadPatternsFromConfig(filePath, logger, scanner)
}
