package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	goscan "github.com/podhmo/go-scan"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("!! %+v", err)
	}
}

func run() error {
	const sampleAPIPath = "github.com/podhmo/go-scan/examples/docgen/sampleapi"

	s, err := goscan.New()
	if err != nil {
		return err
	}

	analyzer, err := NewAnalyzer(s)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := analyzer.Analyze(ctx, sampleAPIPath); err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(analyzer.OpenAPI)
}
