package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

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
	ctx := context.Background()
	if len(os.Args) > 1 {
		runFile(ctx, os.Args[1])
	} else {
		// For runREPL, we now pass standard I/O streams.
		if err := runREPL(os.Stdin, os.Stdout); err != nil {
			slog.ErrorContext(ctx, "REPL error", "error", err)
			os.Exit(1)
		}
	}
}

func runFile(ctx context.Context, filename string) {
	source, err := os.ReadFile(filename)
	if err != nil {
		slog.ErrorContext(ctx, "Error reading script file", "error", err, "filename", filename)
		os.Exit(1)
	}

	interp, err := newInterpreterWithStdlib()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create interpreter", "error", err)
		os.Exit(1)
	}

	// Load the script file.
	if err := interp.LoadFile(filename, source); err != nil {
		slog.ErrorContext(ctx, "Failed to load script", "error", err, "filename", filename)
		os.Exit(1)
	}

	// Evaluate the loaded script.
	result, err := interp.Eval(ctx)
	if err != nil {
		// This path is taken for runtime errors in the script itself.
		// We print to stderr directly to separate it from program output.
		fmt.Fprintf(os.Stderr, "Runtime error:\n%v\n", err)
		os.Exit(1)
	}

	// Print the final result of the script execution.
	if result != nil && result.Value != nil {
		fmt.Println(result.Value.Inspect())
	}
}

// runREPL now accepts an io.Reader and io.Writer for testability.
func runREPL(in io.Reader, out io.Writer) error {
	fmt.Fprintln(out, "Welcome to the MiniGo REPL!")
	fmt.Fprintln(out, "Type :help for more information.")

	interp, err := newInterpreterWithStdlib()
	if err != nil {
		return fmt.Errorf("failed to create interpreter: %w", err)
	}

	scanner := bufio.NewScanner(in)
	ctx := context.Background()

	for {
		fmt.Fprint(out, ">> ")
		if !scanner.Scan() {
			break // Exit on EOF
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, ":load ") {
			filename := strings.TrimSpace(strings.TrimPrefix(line, ":load "))
			if filename == "" {
				fmt.Fprintln(out, "Error: filename is missing for :load command")
				continue
			}

			if err := interp.EvalFileInREPL(ctx, filename); err != nil {
				fmt.Fprintf(out, "Error loading file %q: %v\n", filename, err)
				continue
			}

			fmt.Fprintln(out, "Loading file "+filename)
		} else {
			switch line {
			case ":help":
				fmt.Fprintln(out, "Available commands:")
				fmt.Fprintln(out, "  :help    - Show this help message")
				fmt.Fprintln(out, "  :reset   - Reset the interpreter state")
				fmt.Fprintln(out, "  :load    - Load a Go file into the interpreter")
				fmt.Fprintln(out, "  :exit    - Exit the REPL")
				fmt.Fprintln(out, "You can also type any valid Go expression.")
			case ":reset":
				fmt.Fprintln(out, "Resetting interpreter state.")
				interp, err = newInterpreterWithStdlib()
				if err != nil {
					// Don't exit, return the error to the caller (main).
					return fmt.Errorf("failed to reset interpreter: %w", err)
				}
			case ":exit":
				return nil
			default:
				result, err := interp.EvalLine(ctx, line)
				if err != nil {
					// Print errors to the output writer so they can be captured in tests.
					fmt.Fprintln(out, "Error:", err)
					continue
				}
				// Only print if the result is not nil
				if result != nil && result.Type() != "NIL" {
					fmt.Fprintln(out, result.Inspect())
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if err != io.EOF {
			// Also print this error to the output writer.
			fmt.Fprintln(out, "Error reading input:", err)
		}
	}
	return nil
}
