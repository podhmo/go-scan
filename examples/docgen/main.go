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

// stringSlice is a custom type for handling repeatable string flags
type stringSlice []string

func (s *stringSlice) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	var (
		format       string
		patternsFile string
		entrypoint   string
		extraPkgs    stringSlice
		logLevel     = slog.LevelInfo
	)
	flag.StringVar(&format, "format", "json", "Output format (json or yaml)")
	flag.StringVar(&patternsFile, "patterns", "", "Path to a Go file with custom pattern configurations")
	flag.StringVar(&entrypoint, "entrypoint", "NewServeMux", "The entrypoint function name")
	flag.Var(&extraPkgs, "include-pkg", "Specify an external package to treat as internal (can be used multiple times)")
	flag.TextVar(&logLevel, "log-level", &logLevel, "set log level (debug, info, warn, error)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: &logLevel}))

	if err := run(logger, format, patternsFile, entrypoint, extraPkgs); err != nil {
		logger.Error("docgen failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, format string, patternsFile string, entrypoint string, extraPkgs []string) error {
	if flag.NArg() == 0 {
		return fmt.Errorf("required argument: <package-path>")
	}
	ctx := context.Background()
	sampleAPIPath, err := goscan.ResolvePath(ctx, flag.Arg(0))
	if err != nil {
		return fmt.Errorf("failed to resolve package path: %w", err)
	}

	// Let symgo's interpreter configuration handle which packages are declarations-only.
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

	analyzer, err := NewAnalyzer(s, logger, extraPkgs, opts...)
	if err != nil {
		return err
	}

	if err := analyzer.Analyze(ctx, sampleAPIPath, entrypoint); err != nil {
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
