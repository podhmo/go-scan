package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/podhmo/go-scan/internal/convert"
)

func main() {
	var (
		inputPath  string
		outputPath string
	)

	flag.StringVar(&inputPath, "input", "", "path to input package")
	flag.StringVar(&outputPath, "output", "", "path to output file")
	flag.Parse()

	if inputPath == "" {
		log.Fatal("-input is required")
	}
	if outputPath == "" {
		log.Fatal("-output is required")
	}

	if err := run(inputPath, outputPath); err != nil {
		log.Fatalf("!! %+v", err)
	}
}

func run(inputPath string, outputPath string) error {
	ctx := context.Background()
	info, err := convert.Parse(ctx, inputPath)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(info)
}
