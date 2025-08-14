package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/podhmo/go-scan/minigo"

	json_ "github.com/podhmo/go-scan/minigo/stdlib/encoding/json"
	fmt_ "github.com/podhmo/go-scan/minigo/stdlib/fmt"
	strconv_ "github.com/podhmo/go-scan/minigo/stdlib/strconv"
	strings_ "github.com/podhmo/go-scan/minigo/stdlib/strings"
)

// newInterpreterWithStdlib is a helper to create an interpreter and register standard libs.
func newInterpreterWithStdlib() (*minigo.Interpreter, error) {
	interp, err := minigo.NewInterpreter()
	if err != nil {
		return nil, err
	}
	// Register some useful Go functions to be available in the script.
	strings_.Install(interp)
	fmt_.Install(interp)
	json_.Install(interp)
	strconv_.Install(interp)
	return interp, nil
}

func main() {
	if len(os.Args) > 1 {
		runFile(os.Args[1])
	} else {
		runREPL()
	}
}

func runFile(filename string) {
	source, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Error reading script file: %v", err)
	}

	interp, err := newInterpreterWithStdlib()
	if err != nil {
		log.Fatalf("Failed to create interpreter: %v", err)
	}

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

func runREPL() {
	fmt.Println("Welcome to the MiniGo REPL!")
	fmt.Println("Type :help for more information.")

	interp, err := newInterpreterWithStdlib()
	if err != nil {
		log.Fatalf("Failed to create interpreter: %v", err)
	}

	scanner := bufio.NewScanner(os.Stdin)
	ctx := context.Background()

	for {
		fmt.Print(">> ")
		if !scanner.Scan() {
			break // Exit on EOF (Ctrl+D)
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		switch line {
		case ":help":
			fmt.Println("Available commands:")
			fmt.Println("  :help    - Show this help message")
			fmt.Println("  :reset   - Reset the interpreter state")
			fmt.Println("  :exit    - Exit the REPL")
			fmt.Println("You can also type any valid Go expression.")
		case ":reset":
			fmt.Println("Resetting interpreter state.")
			interp, err = newInterpreterWithStdlib()
			if err != nil {
				log.Fatalf("Failed to create interpreter: %v", err)
			}
		case ":exit":
			return
		default:
			result, err := interp.EvalLine(ctx, line)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				continue
			}
			// Only print if the result is not nil
			if result.Type() != "NIL" {
				fmt.Println(result.Inspect())
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if err != io.EOF {
			fmt.Fprintln(os.Stderr, "Error reading input:", err)
		}
	}
}
