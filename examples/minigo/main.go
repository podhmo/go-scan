package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/podhmo/go-scan/minigo"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: minigo <scriptfile>")
		os.Exit(1)
	}
	filename := os.Args[1]

	source, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Error reading script file: %v", err)
	}

	// Create a new interpreter.
	// The minigo library is located in the parent project, so we use a relative path.
	interp, err := minigo.NewInterpreter()
	if err != nil {
		log.Fatalf("Failed to create interpreter: %v", err)
	}

	// Register some useful Go functions to be available in the script.
	interp.Register("fmt", map[string]any{
		"Sprintf": fmt.Sprintf,
	})
	interp.Register("strings", map[string]any{
		"ToUpper": strings.ToUpper,
		"ToLower": strings.ToLower,
		"Join":    strings.Join,
	})

	// Load the script file.
	if err := interp.LoadFile(filename, source); err != nil {
		log.Fatalf("Failed to load script: %v", err)
	}

	// Evaluate the loaded script.
	result, err := interp.Eval(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Runtime error:\n%v\n", err)
		os.Exit(1)
	}

	// Print the final result of the script execution.
	fmt.Println(result.Value.Inspect())
}
