package symgo

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"log/slog"
	"strings"

	goscan "github.com/podhmo/go-scan"
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
	scanner   *goscan.Scanner
	eval      *evaluator.Evaluator
	globalEnv *object.Environment
	logger    *slog.Logger
}

// Option is a functional option for configuring the Interpreter.
type Option func(*Interpreter)

// WithLogger sets the logger for the interpreter.
func WithLogger(logger *slog.Logger) Option {
	return func(i *Interpreter) {
		i.logger = logger
	}
}

// Scanner returns the underlying go-scan Scanner instance.
func (i *Interpreter) Scanner() *goscan.Scanner {
	return i.scanner
}

// NewInterpreter creates a new symgo interpreter.
// It requires a pre-configured go-scan.Scanner instance.
func NewInterpreter(scanner *goscan.Scanner, options ...Option) (*Interpreter, error) {
	if scanner == nil {
		return nil, fmt.Errorf("scanner cannot be nil")
	}

	i := &Interpreter{
		scanner:   scanner,
		globalEnv: object.NewEnvironment(),
	}

	for _, opt := range options {
		opt(i)
	}

	// Set a default logger if one wasn't provided.
	if i.logger == nil {
		i.logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	}

	i.eval = evaluator.New(scanner, i.logger)

	// Register default intrinsics
	i.RegisterIntrinsic("fmt.Sprintf", i.intrinsicSprintf)

	return i, nil
}

// intrinsicSprintf provides a basic implementation of fmt.Sprintf for the symbolic engine.
func (i *Interpreter) intrinsicSprintf(eval *Interpreter, args []Object) Object {
	if len(args) == 0 {
		return &Error{Message: "Sprintf requires at least one argument", Pos: token.NoPos}
	}

	format, ok := args[0].(*String)
	if !ok {
		if _, isSymbolic := args[0].(*SymbolicPlaceholder); isSymbolic {
			return &SymbolicPlaceholder{Reason: "fmt.Sprintf with symbolic format string"}
		}
		return &Error{Message: fmt.Sprintf("the first argument to Sprintf must be a string, got %s", args[0].Type()), Pos: token.NoPos}
	}

	result := format.Value
	argIndex := 1

	var newStr strings.Builder
	for i := 0; i < len(result); i++ {
		if result[i] == '%' && i+1 < len(result) {
			verb := result[i+1]
			if verb == '%' {
				newStr.WriteByte('%')
				i++ // skip the second '%'
				continue
			}

			if argIndex >= len(args) {
				newStr.WriteByte('%')
				newStr.WriteByte(verb)
				i++
				continue
			}

			if verb == 's' || verb == 'd' || verb == 'v' {
				arg := args[argIndex]
				replacement := arg.Inspect()
				if str, ok := arg.(*String); ok {
					replacement = str.Value
				}
				newStr.WriteString(replacement)
				argIndex++
				i++ // skip the verb
			} else {
				newStr.WriteByte('%')
			}
		} else {
			newStr.WriteByte(result[i])
		}
	}

	return &String{Value: newStr.String()}
}

// Eval evaluates a given AST node in the interpreter's persistent environment.
// It requires the PackageInfo of the file containing the node to resolve types correctly.
func (i *Interpreter) Eval(ctx context.Context, node ast.Node, pkg *scanner.PackageInfo) (Object, error) {
	result := i.eval.Eval(ctx, node, i.globalEnv, pkg)
	if err, ok := result.(*Error); ok {
		if err.Pos.IsValid() {
			position := i.scanner.Fset().Position(err.Pos)
			return nil, fmt.Errorf("%s: %s", position, err.Message)
		}
		return nil, fmt.Errorf("%s", err.Message)
	}
	return result, nil
}

// EvalWithEnv evaluates a node using a specific environment instead of the global one.
func (i *Interpreter) EvalWithEnv(ctx context.Context, node ast.Node, env *Environment, pkg *scanner.PackageInfo) (Object, error) {
	result := i.eval.Eval(ctx, node, env, pkg)
	if err, ok := result.(*Error); ok {
		if err.Pos.IsValid() {
			position := i.scanner.Fset().Position(err.Pos)
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
func (i *Interpreter) PushIntrinsics(newIntrinsics map[string]IntrinsicFunc) {
	i.eval.PushIntrinsics()
	for key, handler := range newIntrinsics {
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
func (i *Interpreter) Apply(ctx context.Context, fn Object, args []Object, pkg *scanner.PackageInfo) (Object, error) {
	// This is a simplified wrapper. A real implementation might need more context.
	result := i.eval.Apply(ctx, fn, args, pkg)
	if err, ok := result.(*Error); ok {
		if err.Pos.IsValid() {
			position := i.scanner.Fset().Position(err.Pos)
			return nil, fmt.Errorf("%s: %s", position, err.Message)
		}
		return nil, fmt.Errorf("%s", err.Message)
	}
	return result, nil
}
