package minigo2

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"

	"github.com/podhmo/go-scan/minigo2/evaluator"
	"github.com/podhmo/go-scan/minigo2/object"
)

// Options configures the interpreter environment.
type Options struct {
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

// Run executes a minigo2 script. It evaluates the entire script from top to bottom.
func Run(ctx context.Context, opts Options) (*Result, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, opts.Filename, opts.Source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing script: %w", err)
	}

	eval := evaluator.New(fset)
	env := object.NewEnvironment()

	evaluated := eval.Eval(node, env)
	if evaluated != nil && evaluated.Type() == object.ERROR_OBJ {
		// The error object's Inspect() method now returns a fully formatted string.
		return nil, fmt.Errorf("%s", evaluated.Inspect())
	}

	return &Result{Value: evaluated}, nil
}
