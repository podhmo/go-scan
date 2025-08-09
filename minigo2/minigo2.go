package minigo2

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"reflect"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo2/evaluator"
	"github.com/podhmo/go-scan/minigo2/object"
)

// Interpreter is the main entry point for the minigo2 language.
// It holds the state of the interpreter, including the scanner for package resolution
// and the root environment for script execution.
type Interpreter struct {
	scanner  *goscan.Scanner
	Env      *object.Environment
	Registry *object.SymbolRegistry
}

// NewInterpreter creates a new interpreter instance.
// It initializes a scanner and a root environment.
func NewInterpreter(options ...goscan.ScannerOption) (*Interpreter, error) {
	scanner, err := goscan.New(options...)
	if err != nil {
		return nil, fmt.Errorf("initializing scanner: %w", err)
	}
	return &Interpreter{
		scanner:  scanner,
		Env:      object.NewEnvironment(),
		Registry: object.NewSymbolRegistry(),
	}, nil
}

// Register makes Go symbols (variables or functions) available for import by a script.
// For example, `interp.Register("strings", map[string]any{"ToUpper": strings.ToUpper})`
// allows a script to `import "strings"` and call `strings.ToUpper()`.
func (i *Interpreter) Register(pkgPath string, symbols map[string]any) {
	i.Registry.Register(pkgPath, symbols)
}

// Options configures the interpreter environment.
type Options struct {
	// Globals allows injecting Go variables into the script's global scope.
	// The map key is the variable name in the script.
	// The value can be any Go variable, which will be made available via reflection.
	Globals map[string]any

	// Source is the script content.
	Source []byte

	// Filename is the name of the script file, used for error messages.
	Filename string
}

// Result holds the outcome of a script execution.
type Result struct {
	// Value is the raw minigo2 object returned by the script.
	Value object.Object
}

// Eval executes a minigo2 script. It evaluates the entire script from top to bottom
// within the interpreter's persistent environment.
func (i *Interpreter) Eval(ctx context.Context, opts Options) (*Result, error) {
	// Inject global variables from Go into the interpreter's environment.
	for name, value := range opts.Globals {
		i.Env.Set(name, &object.GoValue{Value: reflect.ValueOf(value)})
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, opts.Filename, opts.Source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing script: %w", err)
	}

	eval := evaluator.New(fset, i.scanner, i.Registry)
	var lastVal object.Object
	for _, decl := range node.Decls {
		lastVal = eval.Eval(decl, i.Env)
		if err, ok := lastVal.(*object.Error); ok {
			// The error object's Inspect() method now returns a fully formatted string.
			return nil, fmt.Errorf("%s", err.Inspect())
		}
	}

	return &Result{Value: lastVal}, nil
}
