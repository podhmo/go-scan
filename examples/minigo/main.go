package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	// "github.com/podhmo/go-scan/scanner" // Will be used later

	"github.com/podhmo/go-scan/examples/minigo/eval" // Added import for eval package
	"github.com/podhmo/go-scan/examples/minigo/stringutils"
)

func main() {
	// Call stringutils.Concat and print the result
	s1 := "Hello, "
	s2 := "World!"
	concatenatedString := stringutils.Concat(s1, s2)
	fmt.Println("Concatenated string:", concatenatedString)

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

	interpreter := eval.NewInterpreter() // Changed to eval.NewInterpreter
	// Store the initial environment, which might be useful for inspection or a REPL later.
	// For now, LoadAndRun creates its own scope for the main function.
	err = interpreter.LoadAndRun(context.Background(), filename, *entryPoint) // interpreter is now *eval.Interpreter
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running interpreter: %v\n", err)
		os.Exit(1)
	}
}
