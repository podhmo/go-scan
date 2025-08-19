package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("!! %+v", err)
	}
}

func run() error {
	const sampleAPIPath = "github.com/podhmo/go-scan/examples/docgen/sampleapi"

	analyzer, err := NewAnalyzer()
	if err != nil {
		return fmt.Errorf("failed to create analyzer: %w", err)
	}

	ctx := context.Background()
	if err := analyzer.Analyze(ctx, sampleAPIPath); err != nil {
		return fmt.Errorf("failed to analyze package: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(analyzer.OpenAPI); err != nil {
		return fmt.Errorf("failed to encode openapi spec: %w", err)
	}

	return nil
}
