package main

import (
	"flag"
	"fmt"
	"os"
	// "github.com/podhmo/go-scan/scanner" // Will be used later
)

func main() {
	entryPoint := flag.String("entry", "main", "entry point function name")
	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: minigo [options] <filename>")
		os.Exit(1)
	}

	filename := flag.Args()[0]
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: file %s not found\n", filename)
		os.Exit(1)
	}

	fmt.Printf("Interpreting %s, entry point: %s\n", filename, *entryPoint)

	interpreter := NewInterpreter()
	// Store the initial environment, which might be useful for inspection or a REPL later.
	// For now, LoadAndRun creates its own scope for the main function.
	err = interpreter.LoadAndRun(filename, *entryPoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running interpreter: %v\n", err)
		os.Exit(1)
	}
}
