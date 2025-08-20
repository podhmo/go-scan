package symgo

import (
	"fmt"
	"go/ast"
	"log/slog"

	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

// Re-export core object types for convenience.
type Object = object.Object
type Function = object.Function
type Error = object.Error
type Instance = object.Instance
type String = object.String
type Pointer = object.Pointer
type Variable = object.Variable
type SymbolicPlaceholder = object.SymbolicPlaceholder
type Slice = object.Slice
type BaseObject = object.BaseObject
type Environment = object.Environment

// NewEnclosedEnvironment creates a new environment that is enclosed by an outer one.
var NewEnclosedEnvironment = object.NewEnclosedEnvironment

// IntrinsicFunc defines the signature for a custom function handler.
type IntrinsicFunc func(eval *Interpreter, args []Object) Object

// Interpreter is the main public entry point for the symgo engine.
type Interpreter struct {
	scanner   *scanner.Scanner
	eval      *evaluator.Evaluator
	globalEnv *object.Environment
}

// Scanner returns the underlying go-scan Scanner instance.
func (i *Interpreter) Scanner() *scanner.Scanner {
	return i.scanner
}

// NewInterpreter creates a new symgo interpreter.
// It requires a pre-configured go-scan.Scanner instance.
func NewInterpreter(scanner *scanner.Scanner, logger *slog.Logger) (*Interpreter, error) {
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
// It requires the PackageInfo of the file containing the node to resolve types correctly.
func (i *Interpreter) Eval(node ast.Node, pkg *scanner.PackageInfo) (Object, error) {
	result := i.eval.Eval(node, i.globalEnv, pkg)
	if err, ok := result.(*Error); ok {
		if err.Pos.IsValid() {
			position := i.scanner.FileSet().Position(err.Pos)
			return nil, fmt.Errorf("%s: %s", position, err.Message)
		}
		return nil, fmt.Errorf("%s", err.Message)
	}
	return result, nil
}

// EvalWithEnv evaluates a node using a specific environment instead of the global one.
func (i *Interpreter) EvalWithEnv(node ast.Node, env *Environment, pkg *scanner.PackageInfo) (Object, error) {
	result := i.eval.Eval(node, env, pkg)
	if err, ok := result.(*Error); ok {
		if err.Pos.IsValid() {
			position := i.scanner.FileSet().Position(err.Pos)
			return nil, fmt.Errorf("%s: %s", position, err.Message)
		}
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

// PushIntrinsics creates a new temporary scope and registers a set of intrinsics on it.
func (i *Interpreter) PushIntrinsics(intrinsics map[string]IntrinsicFunc) {
	i.eval.PushIntrinsics()
	for key, handler := range intrinsics {
		i.RegisterIntrinsic(key, handler)
	}
}

// PopIntrinsics removes the top-most intrinsic scope.
func (i *Interpreter) PopIntrinsics() {
	i.eval.PopIntrinsics()
}

// FindObject looks up an object in the interpreter's global environment.
func (i *Interpreter) FindObject(name string) (Object, bool) {
	return i.globalEnv.Get(name)
}

// Apply is a wrapper around the internal evaluator's applyFunction.
// It is intended for advanced use cases like docgen where direct function invocation is needed.
func (i *Interpreter) Apply(fn Object, args []Object, pkg *scanner.PackageInfo) (Object, error) {
	// This is a simplified wrapper. A real implementation might need more context.
	result := i.eval.Apply(fn, args, pkg)
	if err, ok := result.(*Error); ok {
		if err.Pos.IsValid() {
			position := i.scanner.FileSet().Position(err.Pos)
			return nil, fmt.Errorf("%s: %s", position, err.Message)
		}
		return nil, fmt.Errorf("%s", err.Message)
	}
	return result, nil
}
