package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"example.com/convert2/generator"
	"example.com/convert2/parser"
)

func main() {
	log.SetFlags(0) // No timestamps or prefixes for cleaner output

	var inputDir string
	var outputDir string // Optional: defaults to inputDir

	flag.StringVar(&inputDir, "input", "", "Directory containing Go files to parse (required)")
	flag.StringVar(&outputDir, "output", "", "Directory to write generated files (defaults to input directory)")
	flag.Parse()

	if inputDir == "" {
		flag.Usage()
		log.Fatalf("Error: Input directory is required.\n")
	}

	absInputDir, err := filepath.Abs(inputDir)
	if err != nil {
		log.Fatalf("Error getting absolute path for input directory %s: %v\n", inputDir, err)
	}

	if outputDir == "" {
		outputDir = absInputDir // Default output to input directory
	} else {
		absOutputDir, err := filepath.Abs(outputDir)
		if err != nil {
			log.Fatalf("Error getting absolute path for output directory %s: %v\n", outputDir, err)
		}
		outputDir = absOutputDir
	}

	// Ensure input directory exists
	if _, err := os.Stat(absInputDir); os.IsNotExist(err) {
		log.Fatalf("Error: Input directory %s does not exist.\n", absInputDir)
	}

	log.Printf("Starting converter generator...\n")
	log.Printf("Input directory: %s\n", absInputDir)
	log.Printf("Output directory: %s\n", outputDir)

	// 1. Parse the input directory
	log.Printf("Parsing source files in %s...\n", absInputDir)
	parsedInfo, err := parser.ParseDirectory(absInputDir)
	if err != nil {
		log.Fatalf("Error parsing directory %s: %v\n", absInputDir, err)
	}

	if parsedInfo == nil || (len(parsedInfo.ConversionPairs) == 0 && len(parsedInfo.GlobalRules) == 0) {
		log.Println("No conversion directives (convert:pair or convert:rule) found. Nothing to generate.")
		return
	}
	log.Printf("Parsing complete. Found %d conversion pairs and %d global rules for package %s.\n",
		len(parsedInfo.ConversionPairs), len(parsedInfo.GlobalRules), parsedInfo.PackageName)

	// 2. Generate conversion code
	log.Printf("Generating conversion code...\n")
	err = generator.GenerateConversionCode(parsedInfo, outputDir)
	if err != nil {
		log.Fatalf("Error generating code: %v\n", err)
	}

	log.Printf("Code generation successful. Output files are in %s.\n", outputDir)
}
