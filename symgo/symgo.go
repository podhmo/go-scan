package symgo

import (
	"context"
	"errors"
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
type MultiReturn = object.MultiReturn
type Nil = object.Nil
type BaseObject = object.BaseObject
type Environment = object.Environment
type Tracer = object.Tracer
type TracerFunc = object.TracerFunc

// NewEnclosedEnvironment creates a new environment that is enclosed by an outer one.
var NewEnclosedEnvironment = object.NewEnclosedEnvironment

// IntrinsicFunc defines the signature for a custom function handler.
type IntrinsicFunc func(eval *Interpreter, args []Object) Object

// ScanPolicyFunc is a function that determines whether a package should be scanned from source.
type ScanPolicyFunc = object.ScanPolicyFunc

// Interpreter is the main public entry point for the symgo engine.
type Interpreter struct {
	scanner           *goscan.Scanner
	eval              *evaluator.Evaluator
	globalEnv         *object.Environment
	logger            *slog.Logger
	tracer            object.Tracer
	interfaceBindings map[string]*goscan.TypeInfo
	scanPolicy        object.ScanPolicyFunc
}

// Option is a functional option for configuring the Interpreter.
type Option func(*Interpreter)

// WithLogger sets the logger for the interpreter.
func WithLogger(logger *slog.Logger) Option {
	return func(i *Interpreter) {
		i.logger = logger
	}
}

// WithTracer sets the tracer for the interpreter.
func WithTracer(tracer object.Tracer) Option {
	return func(i *Interpreter) {
		i.tracer = tracer
	}
}

// WithScanPolicy sets a custom policy function to determine which packages to scan from source.
func WithScanPolicy(policy object.ScanPolicyFunc) Option {
	return func(i *Interpreter) {
		i.scanPolicy = policy
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
		scanner:           scanner,
		globalEnv:         object.NewEnvironment(),
		interfaceBindings: make(map[string]*goscan.TypeInfo),
	}

	for _, opt := range options {
		opt(i)
	}

	// Set a default logger if one wasn't provided.
	if i.logger == nil {
		i.logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	}

	// Set a default scan policy if none was provided.
	// The default policy is to only scan packages within the main module(s).
	if i.scanPolicy == nil {
		modules := i.scanner.Modules()
		if len(modules) > 0 {
			modulePaths := make([]string, len(modules))
			for i, m := range modules {
				modulePaths[i] = m.Path
			}
			i.scanPolicy = func(importPath string) bool {
				for _, modulePath := range modulePaths {
					if strings.HasPrefix(importPath, modulePath) {
						return true
					}
				}
				return false
			}
		} else {
			// Fallback if module path is not available: scan nothing extra.
			i.scanPolicy = func(importPath string) bool {
				return false
			}
		}
	}

	i.eval = evaluator.New(scanner, i.logger, i.tracer, i.scanPolicy)

	// Register default intrinsics
	i.RegisterIntrinsic("fmt.Sprintf", i.intrinsicSprintf)

	return i, nil
}

// BindInterface instructs the interpreter to treat a given interface type as a
// specific concrete type during analysis.
// The interface name is the fully qualified name (e.g., "io.Writer").
// The concrete type name can be a pointer or non-pointer type name (e.g., "*bytes.Buffer").
func (i *Interpreter) BindInterface(ifaceTypeName string, concreteTypeName string) error {
	isPointer := strings.HasPrefix(concreteTypeName, "*")
	if isPointer {
		concreteTypeName = strings.TrimPrefix(concreteTypeName, "*")
	}

	pkgPath, typeName := splitQualifiedName(concreteTypeName)
	if pkgPath == "" {
		return fmt.Errorf("concrete type name must be fully qualified (e.g., 'bytes.Buffer'), got %s", concreteTypeName)
	}

	pkg, err := i.scanner.ScanPackageByImport(context.Background(), pkgPath)
	if err != nil {
		return fmt.Errorf("could not scan package %q for concrete type: %w", pkgPath, err)
	}

	var foundType *goscan.TypeInfo
	for _, t := range pkg.Types {
		if t.Name == typeName {
			foundType = t
			break
		}
	}

	if foundType == nil {
		return fmt.Errorf("concrete type %q not found in package %q", typeName, pkgPath)
	}

	// The binding in the evaluator needs the fully qualified name.
	i.eval.BindInterface(ifaceTypeName, foundType)
	return nil
}

// NewSymbolic creates a new symbolic variable with a given type.
// This is a helper for setting up analysis entrypoints.
func (i *Interpreter) NewSymbolic(name string, typeName string) (Object, error) {
	pkgPath, simpleTypeName := splitQualifiedName(typeName)
	if pkgPath == "" {
		return nil, fmt.Errorf("type name must be fully qualified (e.g., 'io.Writer'), got %s", typeName)
	}

	pkg, err := i.scanner.ScanPackageByImport(context.Background(), pkgPath)
	if err != nil {
		return nil, fmt.Errorf("could not scan package %q for symbolic var type: %w", pkgPath, err)
	}

	var foundType *goscan.TypeInfo
	for _, t := range pkg.Types {
		if t.Name == simpleTypeName {
			foundType = t
			break
		}
	}
	if foundType == nil {
		return nil, fmt.Errorf("type %q not found in package %q", simpleTypeName, pkgPath)
	}

	return &Variable{
		Name: name,
		BaseObject: BaseObject{
			ResolvedTypeInfo: foundType,
		},
		Value: &SymbolicPlaceholder{Reason: "function parameter"},
	}, nil
}

// splitQualifiedName splits a name like "pkg/path.Name" into "pkg/path" and "Name".
func splitQualifiedName(name string) (pkgPath, typeName string) {
	// To handle both "io.Writer" and "github.com/user/repo/pkg.Type",
	// we find the last dot.
	lastDot := strings.LastIndex(name, ".")
	if lastDot == -1 {
		return "", name // Should not happen for qualified names.
	}

	// Now we need to determine if the part before the dot is the package path.
	// A simple heuristic: if it contains a '/', it's a full path.
	// If not, it's a simple package name like 'io' or 'bytes'.
	// This heuristic isn't perfect but works for stdlib and typical repos.
	pkgCandidate := name[:lastDot]
	typeName = name[lastDot+1:]

	// In the context of go-scan, the package path is what's used in the import statement.
	// For "bytes.Buffer", the import path is "bytes".
	// For "mypackage.MyType" in the current module, it might be "mymodule/mypackage".
	// The scanner handles resolving this. We just need to provide the parts.
	// The logic in BindInterface and NewSymbolic which calls scanner.ScanPackageByImport
	// with the pkgCandidate works correctly for both "bytes" and "github.com/foo/bar".
	return pkgCandidate, typeName
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
	// The evaluator itself is responsible for handling imports and populating
	// the environment. The logic here was redundant and used an incorrect
	// method for guessing package names. By removing it, we centralize the
	// import handling logic within the evaluator's `evalFile` and `evalImportSpec`
	// methods.
	result := i.eval.Eval(ctx, node, i.globalEnv, pkg)
	if err, ok := result.(*Error); ok {
		return nil, errors.New(err.Inspect())
	}
	return result, nil
}

// EvalWithEnv evaluates a node using a specific environment instead of the global one.
func (i *Interpreter) EvalWithEnv(ctx context.Context, node ast.Node, env *Environment, pkg *scanner.PackageInfo) (Object, error) {
	result := i.eval.Eval(ctx, node, env, pkg)
	if err, ok := result.(*Error); ok {
		return nil, errors.New(err.Inspect())
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

// RegisterDefaultIntrinsic registers a default function to be called for any function call.
func (i *Interpreter) RegisterDefaultIntrinsic(handler IntrinsicFunc) {
	wrappedHandler := func(args ...object.Object) object.Object {
		return handler(i, args)
	}
	i.eval.RegisterDefaultIntrinsic(wrappedHandler)
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
		return nil, errors.New(err.Inspect())
	}
	return result, nil
}
