package symgo

import (
	"context"
	"fmt"
	"go/ast"
	"log/slog"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

// Re-export core object types for convenience.
type Object = object.Object
type Function = object.Function
type Error = object.Error
type Instance = object.Instance
type String = object.String

// IntrinsicFunc defines the signature for a custom function handler.
type IntrinsicFunc func(eval *Interpreter, args []Object) Object

// Interpreter is the main public entry point for the symgo engine.
type Interpreter struct {
	scanner   *goscan.Scanner
	eval      *evaluator.Evaluator
	globalEnv *object.Environment
}

// NewInterpreter creates a new symgo interpreter.
// It requires a pre-configured go-scan.Scanner instance.
func NewInterpreter(scanner *goscan.Scanner, logger *slog.Logger) (*Interpreter, error) {
	if scanner == nil {
		return nil, fmt.Errorf("scanner cannot be nil")
	}

	eval := evaluator.New(scanner, logger)

	i := &Interpreter{
		scanner:   scanner,
		eval:      eval,
		globalEnv: object.NewEnvironment(),
	}
	return i, nil
}

// Eval evaluates a given AST node in the interpreter's persistent environment.
func (i *Interpreter) Eval(ctx context.Context, node ast.Node) (Object, error) {
	result := i.eval.Eval(node, i.globalEnv)
	if err, ok := result.(*Error); ok {
		return nil, fmt.Errorf("%s", err.Message)
	}
	return result, nil
}

// RegisterIntrinsic registers a custom handler for a given function.
// The key is the fully qualified function name, e.g., "fmt.Println".
func (i *Interpreter) RegisterIntrinsic(key string, handler IntrinsicFunc) {
	// Wrap the user-friendly IntrinsicFunc into the evaluator's required signature.
	wrappedHandler := func(args ...object.Object) object.Object {
		// The handler passed by the user gets the interpreter instance, allowing
		// it to perform powerful operations if needed.
		return handler(i, args)
	}
	i.eval.RegisterIntrinsic(key, wrappedHandler)
}
