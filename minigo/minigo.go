package minigo

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo/evaluator"
	"github.com/podhmo/go-scan/minigo/object"
)

// Options configures the interpreter environment for a single Run command.
type Options struct {
	// Source is the script content.
	Source []byte

	// Filename is the name of the script file, used for error messages.
	// Defaults to "main.go" if empty.
	Filename string

	// EntryPoint is the name of the function to execute.
	// Defaults to "main" if empty.
	EntryPoint string

	// Globals allows injecting Go variables into the script's global scope.
	// The map key is the variable name in the script.
	// The value can be any Go variable, which will be made available via reflection.
	Globals map[string]any

	// Scanner is an optional, pre-configured go-scan scanner.
	// If nil, a new default scanner is created.
	Scanner *goscan.Scanner
}

// Run executes a minigo script in a single, self-contained call.
// It is a convenience wrapper around the more complex Interpreter API.
func Run(ctx context.Context, opts Options) (*Result, error) {
	scanner := opts.Scanner
	if scanner == nil {
		var err error
		scanner, err = goscan.New()
		if err != nil {
			return nil, fmt.Errorf("creating default scanner: %w", err)
		}
	}

	interp, err := NewInterpreter(scanner, WithGlobals(opts.Globals))
	if err != nil {
		return nil, fmt.Errorf("creating interpreter: %w", err)
	}

	filename := opts.Filename
	if filename == "" {
		filename = "main.go"
	}
	if err := interp.LoadFile(filename, opts.Source); err != nil {
		return nil, fmt.Errorf("loading script: %w", err)
	}

	if err := interp.EvalDeclarations(ctx); err != nil {
		return nil, fmt.Errorf("evaluating declarations: %w", err)
	}

	entryPoint := opts.EntryPoint
	if entryPoint == "" {
		entryPoint = "main"
	}

	fn, fscope, err := interp.FindFunction(entryPoint)
	if err != nil {
		return nil, fmt.Errorf("finding entry point %q: %w", entryPoint, err)
	}

	return interp.Execute(ctx, fn, nil, fscope)
}

// Interpreter is the main entry point for the minigo language.
// It holds the state of the interpreter, including the scanner for package resolution
// and the root environment for script execution.
type Interpreter struct {
	scanner       *goscan.Scanner
	Registry      *object.SymbolRegistry
	eval          *evaluator.Evaluator
	globalEnv     *object.Environment
	specialForms  map[string]*evaluator.SpecialForm
	files         []*object.FileScope
	packages      map[string]*object.Package
	replFileScope *object.FileScope

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// Option is a functional option for configuring the Interpreter.
type Option func(*Interpreter)

// WithStdin sets the standard input for the interpreter.
func WithStdin(r io.Reader) Option {
	return func(i *Interpreter) {
		i.stdin = r
	}
}

// WithStdout sets the standard output for the interpreter.
func WithStdout(w io.Writer) Option {
	return func(i *Interpreter) {
		i.stdout = w
	}
}

// WithStderr sets the standard error for the interpreter.
func WithStderr(w io.Writer) Option {
	return func(i *Interpreter) {
		i.stderr = w
	}
}

// WithGlobals allows injecting Go variables and functions into the script's global scope.
func WithGlobals(globals map[string]any) Option {
	return func(i *Interpreter) {
		for name, value := range globals {
			rv := reflect.ValueOf(value)
			if rv.Kind() == reflect.Func {
				// It's a function, so wrap it in a Builtin.
				i.globalEnv.Set(name, &object.Builtin{
					Fn: func(ctx *object.BuiltinContext, pos token.Pos, args ...object.Object) object.Object {
						fnType := rv.Type()
						numIn := fnType.NumIn()

						// Check if the number of arguments is correct.
						if fnType.IsVariadic() {
							if len(args) < numIn-1 {
								return ctx.NewError(pos, "wrong number of arguments for variadic function: want at least %d, got %d", numIn-1, len(args))
							}
						} else {
							if len(args) != numIn {
								return ctx.NewError(pos, "wrong number of arguments: want=%d, got=%d", numIn, len(args))
							}
						}

						// Convert minigo args to reflect.Value
						in := make([]reflect.Value, len(args))
						for i, arg := range args {
							var paramType reflect.Type
							if fnType.IsVariadic() && i >= numIn-1 {
								paramType = fnType.In(numIn - 1).Elem()
							} else {
								paramType = fnType.In(i)
							}

							val := reflect.New(paramType).Elem()
							if err := unmarshal(arg, val); err != nil {
								return ctx.NewError(pos, "argument %d type mismatch: %v", i, err)
							}
							in[i] = val
						}

						// Call the Go function
						out := rv.Call(in)

						// Convert reflect.Value result back to minigo object
						if len(out) == 0 {
							return object.NIL
						}
						if len(out) == 1 {
							return fromReflectValue(out[0])
						}

						// Handle multiple return values
						res := make([]object.Object, len(out))
						for i, val := range out {
							res[i] = fromReflectValue(val)
						}
						return &object.Tuple{Elements: res}
					},
				})
			} else {
				// It's a variable, so wrap it in a GoValue.
				i.globalEnv.Set(name, &object.GoValue{Value: rv})
			}
		}
	}
}

// New creates a new interpreter instance with default I/O streams.
// It panics if initialization fails.
func New(scanner *goscan.Scanner, r io.Reader, stdout, stderr io.Writer) *Interpreter {
	i, err := NewInterpreter(scanner, WithStdin(r), WithStdout(stdout), WithStderr(stderr))
	if err != nil {
		panic(err) // Should not happen with default options
	}
	return i
}

// NewInterpreter creates a new interpreter instance, configured with options.
func NewInterpreter(scanner *goscan.Scanner, options ...Option) (*Interpreter, error) {
	i := &Interpreter{
		scanner:      scanner,
		Registry:     object.NewSymbolRegistry(),
		globalEnv:    object.NewEnvironment(),
		specialForms: make(map[string]*evaluator.SpecialForm),
		files:        make([]*object.FileScope, 0),
		packages:     make(map[string]*object.Package),
		stdin:        os.Stdin,
		stdout:       os.Stdout,
		stderr:       os.Stderr,
	}

	for _, opt := range options {
		opt(i)
	}

	i.eval = evaluator.New(evaluator.Config{
		Fset:         i.scanner.Fset(),
		Scanner:      i.scanner,
		Registry:     i.Registry,
		SpecialForms: i.specialForms,
		Packages:     i.packages,
		Stdin:        i.stdin,
		Stdout:       i.stdout,
		Stderr:       i.stderr,
	})

	return i, nil
}

// Register makes Go symbols (variables or functions) available for import by a script.
// For example, `interp.Register("strings", map[string]any{"ToUpper": strings.ToUpper})`
// allows a script to `import "strings"` and call `strings.ToUpper()`.
func (i *Interpreter) Register(pkgPath string, symbols map[string]any) {
	i.Registry.Register(pkgPath, symbols)
}

// RegisterSpecial registers a "special form" function.
// A special form receives the AST of its arguments directly, without them being
// evaluated first. This is useful for implementing DSLs or control structures.
// These functions are available in the global scope.
func (i *Interpreter) RegisterSpecial(name string, fn evaluator.SpecialFormFunction) {
	i.specialForms[name] = &evaluator.SpecialForm{Fn: fn}
}

// LoadGoSourceAsPackage parses and evaluates a single Go source file as a self-contained package.
func (i *Interpreter) LoadGoSourceAsPackage(pkgName, source string) error {
	node, err := parser.ParseFile(i.scanner.Fset(), pkgName+".go", source, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing source for package %q: %w", pkgName, err)
	}

	pkgObj := &object.Package{
		Name:    pkgName,
		Path:    pkgName,
		Members: make(map[string]object.Object),
	}

	pkgObj.Env = object.NewEnclosedEnvironment(i.globalEnv)
	fileScope := object.NewFileScope(node)

	// Manually process imports to populate the file scope for the evaluator.
	for _, importSpec := range node.Imports {
		path, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			return fmt.Errorf("invalid import path in %s: %w", pkgName, err)
		}
		var alias string
		if importSpec.Name != nil {
			alias = importSpec.Name.Name
		} else {
			parts := strings.Split(path, "/")
			alias = parts[len(parts)-1]
		}

		if alias == "." {
			fileScope.DotImports = append(fileScope.DotImports, path)
		} else if alias != "_" {
			fileScope.Aliases[alias] = path
		}
	}

	// Use the new two-pass evaluation logic to correctly handle out-of-order declarations.
	var decls []object.DeclWithScope
	for _, decl := range node.Decls {
		decls = append(decls, object.DeclWithScope{Decl: decl, Scope: fileScope})
	}
	result := i.eval.EvalToplevel(decls, pkgObj.Env)
	if isError(result) {
		return fmt.Errorf("error evaluating package %s: %s", pkgName, result.Inspect())
	}

	// Populate the package's public members from its environment.
	for name, obj := range pkgObj.Env.GetAll() {
		pkgObj.Members[name] = obj
	}

	i.packages[pkgName] = pkgObj
	return nil
}

// EvalString evaluates the given source code string as a complete file.
// It parses the source, evaluates all declarations, and then executes the main function if it exists.
func (i *Interpreter) EvalString(source string) (object.Object, error) {
	fset := i.scanner.Fset()
	node, err := parser.ParseFile(fset, "main.go", source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing script: %w", err)
	}

	fileScope := object.NewFileScope(node)
	result := i.eval.Eval(node, i.globalEnv, fileScope)
	if err, ok := result.(*object.Error); ok {
		return nil, fmt.Errorf("%s", err.Inspect())
	}
	return result, nil
}

// Result holds the outcome of a script execution.
type Result struct {
	Value object.Object
}

// As unmarshals the result of the script execution into a Go variable.
// The target must be a pointer to a Go variable.
// It uses reflection to populate the fields of the target, similar to how
// `json.Unmarshal` works.
func (r *Result) As(target any) error {
	if target == nil {
		return fmt.Errorf("target cannot be nil")
	}
	dstVal := reflect.ValueOf(target)
	if dstVal.Kind() != reflect.Ptr || dstVal.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer, but got %T", target)
	}
	return unmarshal(r.Value, dstVal.Elem())
}

// fromReflectValue converts a reflect.Value to a minigo object.
func fromReflectValue(val reflect.Value) object.Object {
	if !val.IsValid() {
		return object.NIL
	}
	// Use val.Interface() to get the underlying value
	switch v := val.Interface().(type) {
	case int:
		return &object.Integer{Value: int64(v)}
	case int64:
		return &object.Integer{Value: v}
	case float64:
		return &object.Float{Value: v}
	case string:
		return &object.String{Value: v}
	case bool:
		if v {
			return object.TRUE
		}
		return object.FALSE
	case nil:
		return object.NIL
	default:
		// For other types (slices, maps, structs, etc.), we can wrap them
		// back into a GoValue. The script can then access them.
		return &object.GoValue{Value: val}
	}
}

// unmarshal is a recursive helper function that populates a Go `reflect.Value` (dst)
// from a minigo `object.Object` (src).
func unmarshal(src object.Object, dst reflect.Value) error {
	if !dst.CanSet() {
		return fmt.Errorf("cannot set destination value of type %s", dst.Type())
	}
	for dst.Kind() == reflect.Ptr {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst = dst.Elem()
	}
	switch s := src.(type) {
	case *object.Nil:
		dst.Set(reflect.Zero(dst.Type()))
		return nil
	case *object.Integer:
		switch dst.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			dst.SetInt(s.Value)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			dst.SetUint(uint64(s.Value))
		case reflect.Float32, reflect.Float64:
			dst.SetFloat(float64(s.Value))
		case reflect.Interface:
			dst.Set(reflect.ValueOf(s.Value))
		default:
			return fmt.Errorf("cannot unmarshal integer into %s", dst.Type())
		}
		return nil
	case *object.String:
		switch dst.Kind() {
		case reflect.String:
			dst.SetString(s.Value)
		case reflect.Interface:
			dst.Set(reflect.ValueOf(s.Value))
		default:
			return fmt.Errorf("cannot unmarshal string into %s", dst.Type())
		}
		return nil
	case *object.Boolean:
		switch dst.Kind() {
		case reflect.Bool:
			dst.SetBool(s.Value)
		case reflect.Interface:
			dst.Set(reflect.ValueOf(s.Value))
		default:
			return fmt.Errorf("cannot unmarshal boolean into %s", dst.Type())
		}
		return nil
	case *object.GoValue:
		if !s.Value.Type().AssignableTo(dst.Type()) {
			if s.Value.Type().ConvertibleTo(dst.Type()) {
				dst.Set(s.Value.Convert(dst.Type()))
				return nil
			}
			return fmt.Errorf("cannot assign Go value of type %s to %s", s.Value.Type(), dst.Type())
		}
		dst.Set(s.Value)
		return nil
	case *object.GoSourceFunction:
		// The target field must be of type `any` (interface{}) to accept the raw object.
		if dst.Kind() != reflect.Interface {
			return fmt.Errorf("cannot unmarshal GoSourceFunction into non-interface type %s", dst.Type())
		}
		dst.Set(reflect.ValueOf(s))
		return nil
	case *object.BoundMethod:
		if dst.Kind() != reflect.Interface {
			return fmt.Errorf("cannot unmarshal BoundMethod into non-interface type %s", dst.Type())
		}
		dst.Set(reflect.ValueOf(s))
		return nil
	case *object.GoMethodValue:
		if dst.Kind() != reflect.Interface {
			return fmt.Errorf("cannot unmarshal GoMethodValue into non-interface type %s", dst.Type())
		}
		dst.Set(reflect.ValueOf(s))
		return nil
	case *object.Array:
		if dst.Kind() != reflect.Slice {
			return fmt.Errorf("cannot unmarshal array into non-slice type %s", dst.Type())
		}
		sliceType := dst.Type()
		newSlice := reflect.MakeSlice(sliceType, len(s.Elements), len(s.Elements))
		for i, elem := range s.Elements {
			if err := unmarshal(elem, newSlice.Index(i)); err != nil {
				return fmt.Errorf("error in slice element %d: %w", i, err)
			}
		}
		dst.Set(newSlice)
		return nil
	case *object.Map:
		if dst.Kind() != reflect.Map {
			return fmt.Errorf("cannot unmarshal map into non-map type %s", dst.Type())
		}
		mapType := dst.Type()
		keyType := mapType.Key()
		valType := mapType.Elem()
		newMap := reflect.MakeMap(mapType)
		for _, pair := range s.Pairs {
			key := reflect.New(keyType).Elem()
			if err := unmarshal(pair.Key, key); err != nil {
				return fmt.Errorf("error in map key: %w", err)
			}
			val := reflect.New(valType).Elem()
			if err := unmarshal(pair.Value, val); err != nil {
				return fmt.Errorf("error in map value for key %v: %w", key, err)
			}
			newMap.SetMapIndex(key, val)
		}
		dst.Set(newMap)
		return nil
	case *object.Tuple:
		if dst.Kind() != reflect.Struct {
			return fmt.Errorf("cannot unmarshal tuple into non-struct type %s", dst.Type())
		}
		if len(s.Elements) > dst.NumField() {
			return fmt.Errorf("tuple has more elements (%d) than destination struct has fields (%d)", len(s.Elements), dst.NumField())
		}
		for i, elem := range s.Elements {
			dstField := dst.Field(i)
			if err := unmarshal(elem, dstField); err != nil {
				return fmt.Errorf("error in tuple element %d into field %s: %w", i, dst.Type().Field(i).Name, err)
			}
		}
		return nil
	case *object.StructInstance:
		if dst.Kind() != reflect.Struct {
			return fmt.Errorf("cannot unmarshal struct instance into non-struct type %s", dst.Type())
		}
		dstFields := make(map[string]reflect.Value)
		for i := 0; i < dst.NumField(); i++ {
			field := dst.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			dstFields[strings.ToLower(field.Name)] = dst.Field(i)
		}
		for fieldName, srcFieldVal := range s.Fields {
			if dstField, ok := dstFields[strings.ToLower(fieldName)]; ok {
				if err := unmarshal(srcFieldVal, dstField); err != nil {
					return fmt.Errorf("error in struct field %q: %w", fieldName, err)
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported object type for unmarshaling: %s", src.Type())
	}
}

// LoadFile parses a file and adds it to the interpreter's state without evaluating it yet.
// This is the first stage of a multi-file evaluation.
func (i *Interpreter) LoadFile(filename string, source []byte) error {
	fset := i.scanner.Fset()
	node, err := parser.ParseFile(fset, filename, source, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing script %q: %w", filename, err)
	}
	fileScope := object.NewFileScope(node)
	i.files = append(i.files, fileScope)
	return nil
}

// EvalDeclarations evaluates all top-level declarations in the loaded files.
func (i *Interpreter) EvalDeclarations(ctx context.Context) error {
	// Associate each declaration with its original file scope to respect
	// file-scoped imports.
	var allDecls []object.DeclWithScope
	for _, file := range i.files {
		for _, decl := range file.AST.Decls {
			allDecls = append(allDecls, object.DeclWithScope{Decl: decl, Scope: file})
		}
	}

	// The new top-level evaluation function will handle the two-pass evaluation.
	result := i.eval.EvalToplevel(allDecls, i.globalEnv)
	if err, ok := result.(*object.Error); ok {
		return fmt.Errorf("%s", err.Inspect())
	}
	return nil
}

// Eval executes the loaded files.
// It first processes all declarations and then runs the main function if it exists.
func (i *Interpreter) Eval(ctx context.Context) (*Result, error) {
	if err := i.EvalDeclarations(ctx); err != nil {
		return nil, err
	}
	mainFunc, fscope, err := i.FindFunction("main")
	if err != nil {
		return &Result{Value: object.NIL}, nil
	}
	return i.Execute(ctx, mainFunc, nil, fscope)
}

// FindFunction finds a function in the global scope and the file scope it was defined in.
func (i *Interpreter) FindFunction(name string) (*object.Function, *object.FileScope, error) {
	obj, ok := i.globalEnv.Get(name)
	if !ok {
		return nil, nil, fmt.Errorf("function %q not found", name)
	}
	fn, ok := obj.(*object.Function)
	if !ok {
		return nil, nil, fmt.Errorf("%q is not a function, but %s", name, obj.Type())
	}
	if fn.Env == i.globalEnv {
		if len(i.files) > 0 {
			return fn, i.files[0], nil
		}
	}
	return fn, nil, fmt.Errorf("could not find file scope for function %q", name)
}

// Execute runs a given function with the provided arguments using the interpreter's persistent evaluator.
func (i *Interpreter) Execute(ctx context.Context, fn *object.Function, args []object.Object, fscope *object.FileScope) (*Result, error) {
	result := i.eval.ApplyFunction(nil, fn, args, fscope)
	if err, ok := result.(*object.Error); ok {
		return nil, fmt.Errorf("%s", err.Inspect())
	}
	return &Result{Value: result}, nil
}

// EvalLine evaluates a single line of input for the REPL.
// It maintains state across calls by using a persistent, single FileScope.
func (i *Interpreter) EvalLine(ctx context.Context, line string) (object.Object, error) {
	if i.replFileScope == nil {
		node, err := parser.ParseFile(i.scanner.Fset(), "REPL", "package REPL", parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("initializing repl scope: %w", err)
		}
		i.replFileScope = object.NewFileScope(node)
	}
	srcAsDecl := "package REPL\n" + line
	node, err := parser.ParseFile(i.scanner.Fset(), "REPL", srcAsDecl, parser.ParseComments)
	if err == nil {
		i.replFileScope.AST.Decls = append(i.replFileScope.AST.Decls, node.Decls...)
		var result object.Object = object.NIL
		for _, decl := range node.Decls {
			result = i.eval.Eval(decl, i.globalEnv, i.replFileScope)
			if err, ok := result.(*object.Error); ok {
				return nil, fmt.Errorf("%s", err.Inspect())
			}
		}
		return result, nil
	}
	srcAsStmt := "package REPL\nfunc _() {\n" + line + "\n}"
	node, err = parser.ParseFile(i.scanner.Fset(), "REPL", srcAsStmt, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if len(node.Decls) == 0 || len(node.Decls[0].(*ast.FuncDecl).Body.List) == 0 {
		return object.NIL, nil // Empty line or just comments
	}
	stmts := node.Decls[0].(*ast.FuncDecl).Body.List
	var result object.Object = object.NIL
	for _, stmt := range stmts {
		result = i.eval.Eval(stmt, i.globalEnv, i.replFileScope)
		if err, ok := result.(*object.Error); ok {
			return nil, fmt.Errorf("%s", err.Inspect())
		}
	}
	return result, nil
}

func isError(obj object.Object) bool {
	if obj != nil {
		return obj.Type() == object.ERROR_OBJ
	}
	return false
}

// evaluatorForTest returns the interpreter's evaluator instance.
// This method is intended for use in tests only.
func (i *Interpreter) evaluatorForTest() *evaluator.Evaluator {
	return i.eval
}

// EvalFileInREPL parses and evaluates a file's declarations within the persistent REPL scope.
// This allows loaded files to affect the REPL's state, including imports.
func (i *Interpreter) EvalFileInREPL(ctx context.Context, filename string) error {
	// Initialize the REPL's file scope on the first call, if it doesn't exist.
	if i.replFileScope == nil {
		node, err := parser.ParseFile(i.scanner.Fset(), "REPL", "package REPL", parser.ParseComments)
		if err != nil {
			return fmt.Errorf("initializing repl scope: %w", err)
		}
		i.replFileScope = object.NewFileScope(node)
	}

	source, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("reading file %q: %w", filename, err)
	}

	// Parse the file content.
	node, err := parser.ParseFile(i.scanner.Fset(), filename, source, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing script %q: %w", filename, err)
	}

	// Add the declarations from the loaded file to the REPL's scope AST.
	i.replFileScope.AST.Decls = append(i.replFileScope.AST.Decls, node.Decls...)

	// Evaluate each new declaration in the context of the REPL.
	// This will process imports and function/variable definitions.
	for _, decl := range node.Decls {
		result := i.eval.Eval(decl, i.globalEnv, i.replFileScope)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("%s", err.Inspect())
		}
	}

	return nil
}

// Scanner returns the underlying goscan.Scanner instance.
func (i *Interpreter) Scanner() *goscan.Scanner {
	return i.scanner
}

// Files returns the file scopes that have been loaded into the interpreter.
func (i *Interpreter) Files() []*object.FileScope {
	return i.files
}

// GlobalEnvForTest returns the interpreter's global environment.
// This method is intended for use in tests only.
func (i *Interpreter) GlobalEnvForTest() *object.Environment {
	return i.globalEnv
}
