package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
	json_ "github.com/podhmo/go-scan/minigo/stdlib/encoding/json"
	fmt_ "github.com/podhmo/go-scan/minigo/stdlib/fmt"
	strconv_ "github.com/podhmo/go-scan/minigo/stdlib/strconv"
	strings_ "github.com/podhmo/go-scan/minigo/stdlib/strings"
)

// newInterpreterWithStdlib is a helper to create an interpreter and register standard libs.
func newInterpreterWithStdlib() (*minigo.Interpreter, error) {
	s, err := goscan.New(goscan.WithGoModuleResolver())
	if err != nil {
		return nil, err
	}
	interp, err := minigo.NewInterpreter(s)
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
	var (
		fileOption   string
		funcOption   string
		outputOption string
		evalOption   string
	)

	// Custom flag set to allow mixing flags and positional args
	fs := flag.NewFlagSet("minigo", flag.ContinueOnError)
	fs.StringVar(&fileOption, "file", "", "Go file to load as configuration")
	fs.StringVar(&funcOption, "func", "Config", "function to call in the file")
	fs.StringVar(&outputOption, "output", "inspect", "output format (inspect or json)")
	fs.StringVar(&evalOption, "code", "", "evaluate Go code snippet")

	// Parse flags, allowing for positional arguments to be processed later.
	fs.Parse(os.Args[1:])

	ctx := context.Background()

	// High-priority: 'gen-bindings' subcommand
	if len(os.Args) > 1 && os.Args[1] == "gen-bindings" {
		runGenBindings(os.Args[2:])
		return
	}

	// New execution path with flags
	if fileOption != "" || evalOption != "" {
		if err := run(ctx, fileOption, funcOption, outputOption, evalOption); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Legacy execution path for positional arguments
	if fs.NArg() > 0 {
		runFile(ctx, fs.Arg(0))
		return
	}

	// Default to REPL
	if err := runREPL(os.Stdin, os.Stdout, nil); err != nil {
		slog.ErrorContext(ctx, "REPL error", "error", err)
		os.Exit(1)
	}
}

// run handles the new execution mode for file/eval-based script execution.
func run(ctx context.Context, filename, funcname, output, eval string) error {
	interp, err := newInterpreterWithStdlib()
	if err != nil {
		return fmt.Errorf("failed to create interpreter: %w", err)
	}

	if eval != "" {
		// Execute code snippet from -c flag
		if err := interp.LoadFile("<cmdline>", []byte(eval)); err != nil {
			return fmt.Errorf("failed to load code snippet: %w", err)
		}
	} else {
		// Execute from file
		source, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("error reading script file %q: %w", filename, err)
		}
		if err := interp.LoadFile(filename, source); err != nil {
			return fmt.Errorf("failed to load script %q: %w", filename, err)
		}
	}

	// Evaluate all top-level declarations
	if err := interp.EvalDeclarations(ctx); err != nil {
		return fmt.Errorf("evaluating declarations: %w", err)
	}

	// Find the function to execute.
	fn, fscope, err := interp.FindFunction(funcname)
	if err != nil {
		return fmt.Errorf("finding entry point %q: %w", funcname, err)
	}

	// Execute the function.
	result, err := interp.Execute(ctx, fn, nil, fscope)
	if err != nil {
		return fmt.Errorf("runtime error in %q: %w", funcname, err)
	}

	if result == nil || result.Value == nil {
		return nil // No result to print
	}

	// Handle the output format.
	switch output {
	case "json":
		nativeResult, err := toGoValue(result.Value)
		if err != nil {
			return fmt.Errorf("failed to convert result to Go value: %w", err)
		}
		b, err := json.MarshalIndent(nativeResult, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal result to JSON: %w", err)
		}
		fmt.Println(string(b))
	case "inspect":
		fmt.Println(result.Value.Inspect())
	default:
		return fmt.Errorf("unsupported output format: %q", output)
	}

	return nil
}

// toGoValue converts a minigo object to a native Go value.
func toGoValue(src object.Object) (any, error) {
	switch s := src.(type) {
	case *object.Nil:
		return nil, nil
	case *object.Integer:
		return s.Value, nil
	case *object.String:
		return s.Value, nil
	case *object.Boolean:
		return s.Value, nil
	case *object.GoValue:
		if s.Value.IsValid() {
			return s.Value.Interface(), nil
		}
		return nil, nil
	case *object.Array:
		arr := make([]any, len(s.Elements))
		for i, elem := range s.Elements {
			var err error
			arr[i], err = toGoValue(elem)
			if err != nil {
				return nil, err
			}
		}
		return arr, nil
	case *object.Map:
		stringMap := make(map[string]any)
		for _, pair := range s.Pairs {
			key, err := toGoValue(pair.Key)
			if err != nil {
				return nil, err
			}
			val, err := toGoValue(pair.Value)
			if err != nil {
				return nil, err
			}
			keyStr, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("json map keys must be strings, got %T", key)
			}
			stringMap[keyStr] = val
		}
		return stringMap, nil
	case *object.StructInstance:
		m := make(map[string]any)
		for fieldName, srcFieldVal := range s.Fields {
			var err error
			m[fieldName], err = toGoValue(srcFieldVal)
			if err != nil {
				return nil, err
			}
		}
		return m, nil
	default:
		return nil, fmt.Errorf("unsupported object type for conversion: %s", src.Type())
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
func runREPL(in io.Reader, out io.Writer, interp *minigo.Interpreter) error {
	fmt.Fprintln(out, "Welcome to the MiniGo REPL!")
	fmt.Fprintln(out, "Type :help for more information.")

	var err error
	if interp == nil {
		interp, err = newInterpreterWithStdlib()
		if err != nil {
			return fmt.Errorf("failed to create interpreter: %w", err)
		}
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
				if result != nil && result.Type() != object.NIL_OBJ {
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
