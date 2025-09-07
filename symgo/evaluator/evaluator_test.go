package evaluator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
	_ "github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvalIntegerLiteral(t *testing.T) {
	input := "5"
	node, err := parser.ParseExpr(input)
	if err != nil {
		t.Fatalf("could not parse expression: %v", err)
	}

	s, _ := goscan.New()
	eval := New(s, nil, nil, nil, nil)
	evaluated := eval.Eval(t.Context(), node, eval.UniverseEnv, nil)

	integer, ok := evaluated.(*object.Integer)
	if !ok {
		t.Fatalf("object is not Integer. got=%T (%+v)", evaluated, evaluated)
	}

	if integer.Value != 5 {
		t.Errorf("integer has wrong value. want=%d, got=%d", 5, integer.Value)
	}
}

func TestEvalStringLiteral(t *testing.T) {
	input := `"hello world"`
	node, err := parser.ParseExpr(input)
	if err != nil {
		t.Fatalf("could not parse expression: %v", err)
	}

	eval := New(nil, nil, nil, nil, nil)
	evaluated := eval.Eval(t.Context(), node, eval.UniverseEnv, nil)

	str, ok := evaluated.(*object.String)
	if !ok {
		t.Fatalf("object is not String. got=%T (%+v)", evaluated, evaluated)
	}

	if str.Value != "hello world" {
		t.Errorf("String has wrong value. want=%q, got=%q", "hello world", str.Value)
	}
}

func TestEvalFloatLiteral(t *testing.T) {
	input := "5.5"
	node, err := parser.ParseExpr(input)
	if err != nil {
		t.Fatalf("could not parse expression: %v", err)
	}

	s, _ := goscan.New()
	eval := New(s, nil, nil, nil, nil)
	evaluated := eval.Eval(t.Context(), node, eval.UniverseEnv, nil)

	floatObj, ok := evaluated.(*object.Float)
	if !ok {
		t.Fatalf("object is not Float. got=%T (%+v)", evaluated, evaluated)
	}

	if floatObj.Value != 5.5 {
		t.Errorf("float has wrong value. want=%f, got=%f", 5.5, floatObj.Value)
	}
}

func TestEvalVarStatement(t *testing.T) {
	source := `package main
var x = 10
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)
		env := object.NewEnclosedEnvironment(eval.UniverseEnv)

		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

		val, ok := env.Get("x")
		if !ok {
			return fmt.Errorf("variable 'x' not found")
		}
		variable, ok := val.(*object.Variable)
		if !ok {
			return fmt.Errorf("object is not Variable. got=%T", val)
		}
		integer, ok := variable.Value.(*object.Integer)
		if !ok {
			return fmt.Errorf("variable value is not Integer. got=%T", variable.Value)
		}
		if integer.Value != 10 {
			return fmt.Errorf("integer has wrong value. want=%d, got=%d", 10, integer.Value)
		}
		return nil
	}
	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestApplyFunction_ErrorOnNonCallable(t *testing.T) {
	eval := New(nil, nil, nil, nil, nil)
	nonCallable := &object.Integer{Value: 123}
	args := []object.Object{}

	result := eval.applyFunction(context.Background(), nonCallable, args, nil, token.NoPos)

	errObj, ok := result.(*object.Error)
	if !ok {
		t.Fatalf("expected an error from applyFunction, but got %T", result)
	}

	expectedMsg := "not a function: INTEGER"
	if errObj.Message != expectedMsg {
		t.Errorf("expected error message to be %q, but got %q", expectedMsg, errObj.Message)
	}
}

func TestEvalUnsupportedNode(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))

	node := &ast.BadExpr{} // This node type is not handled by our Eval function.

	eval := New(nil, logger, nil, nil, nil)
	evaluated := eval.Eval(context.Background(), node, eval.UniverseEnv, nil)

	_, ok := evaluated.(*object.Error)
	if !ok {
		t.Fatalf("expected an error object, but got %T (%+v)", evaluated, evaluated)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, `"msg":"evaluation not implemented for *ast.BadExpr"`) {
		t.Errorf("log output does not contain the expected error message. Got: %s", logOutput)
	}
}

func TestEval_DeferStmt_WithFuncLit(t *testing.T) {
	source := `
package main
func deferredFunc() {}
func main() {
	defer func() {
		deferredFunc()
	}()
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	var calledFunctions []string

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)

		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) > 0 {
				if fn, ok := args[0].(*object.Function); ok {
					if fn.Name != nil {
						calledFunctions = append(calledFunctions, fn.Name.Name)
					}
				}
			}
			return nil
		})

		env := object.NewEnclosedEnvironment(eval.UniverseEnv)
		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

		mainFunc, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)

		if len(calledFunctions) == 0 {
			return fmt.Errorf("deferred function call was not tracked")
		}

		found := false
		for _, name := range calledFunctions {
			if name == "deferredFunc" {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("expected tracked function to be 'deferredFunc', but it was not found in %v", calledFunctions)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEval_EmptyStmt(t *testing.T) {
	source := `
package main
func main() {
	; // This is an empty statement
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)

		env := object.NewEnclosedEnvironment(eval.UniverseEnv)
		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

		mainFunc, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		result := eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)

		// The result of main is the result of its last statement. An empty statement
		// should result in something innocuous, not an error.
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed unexpectedly: %s", err.Message)
		}
		if ret, ok := result.(*object.ReturnValue); ok {
			if err, ok := ret.Value.(*object.Error); ok {
				return fmt.Errorf("evaluation failed unexpectedly: %s", err.Message)
			}
		}

		// Success is not returning an error.
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvalBooleanLiteral(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"false", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			node, err := parser.ParseExpr(tt.input)
			if err != nil {
				t.Fatalf("could not parse expression: %v", err)
			}

			eval := New(nil, nil, nil, nil, nil)
			evaluated := eval.Eval(t.Context(), node, eval.UniverseEnv, nil)

			boolean, ok := evaluated.(*object.Boolean)
			if !ok {
				t.Fatalf("object is not Boolean. got=%T (%+v)", evaluated, evaluated)
			}

			if boolean.Value != tt.expected {
				t.Errorf("boolean has wrong value. want=%t, got=%t", tt.expected, boolean.Value)
			}
		})
	}
}

func testEval(t *testing.T, input string) object.Object {
	t.Helper()
	node, err := parser.ParseExpr(input)
	if err != nil {
		// try parsing as a statement
		source := fmt.Sprintf("package main\nfunc main() { %s }", input)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "main.go", source, parser.ParseComments)
		if err != nil {
			t.Fatalf("could not parse input as expression or statement: %v", err)
		}
		if len(f.Decls) == 0 || f.Decls[0].(*ast.FuncDecl).Body == nil || len(f.Decls[0].(*ast.FuncDecl).Body.List) == 0 {
			t.Fatalf("no statements found in parsed source")
		}
		s, _ := goscan.New()
		eval := New(s, nil, nil, nil, nil)
		return eval.Eval(t.Context(), f.Decls[0].(*ast.FuncDecl).Body.List[0], eval.UniverseEnv, nil)
	}

	s, _ := goscan.New()
	eval := New(s, nil, nil, nil, nil)
	return eval.Eval(t.Context(), node, eval.UniverseEnv, nil)
}

func testIntegerObject(t *testing.T, obj object.Object, expected int64) bool {
	result, ok := obj.(*object.Integer)
	if !ok {
		t.Errorf("object is not Integer. got=%T (%+v)", obj, obj)
		return false
	}
	if result.Value != expected {
		t.Errorf("object has wrong value. got=%d, want=%d",
			result.Value, expected)
		return false
	}
	return true
}

func TestEvalBuiltinFunctions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
	}{
		{
			name:     "panic",
			input:    `panic("test panic")`,
			expected: "panic: test panic",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int:
				testIntegerObject(t, evaluated, int64(expected))
			case string:
				errObj, ok := evaluated.(*object.Error)
				if !ok {
					t.Errorf("object is not Error. got=%T (%+v)", evaluated, evaluated)
					return
				}
				if errObj.Message != expected {
					t.Errorf("wrong error message. expected=%q, got=%q",
						expected, errObj.Message)
				}
			}
		})
	}
}

func TestEvalBuiltinFunctionsPlaceholders(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected any
	}{
		{
			name:     "make",
			source:   "package main\nfunc main() { make([]int, 0, 8) }",
			expected: &object.SymbolicPlaceholder{},
		},
		{
			name:     "len-string",
			source:   `package main; func main() { len("four") }`,
			expected: &object.SymbolicPlaceholder{},
		},
		{
			name:     "len-slice",
			source:   `package main; func main() { len([]int{1, 2, 3}) }`,
			expected: &object.SymbolicPlaceholder{},
		},
		{
			name:     "cap-slice",
			source:   `package main; func main() { cap([]int{1, 2, 3}) }`,
			expected: &object.SymbolicPlaceholder{},
		},
		{
			name:     "append",
			source:   `package main; func main() { append([]int{1}, 2) }`,
			expected: &object.SymbolicPlaceholder{},
		},
		{
			name:     "new",
			source:   `package main; func main() { new(int) }`,
			expected: &object.SymbolicPlaceholder{},
		},
		{
			name:     "copy",
			source:   `package main; func main() { copy([]int{1}, []int{2}) }`,
			expected: &object.SymbolicPlaceholder{},
		},
		{
			name:     "delete",
			source:   `package main; func main() { delete(map[string]int{}, "k") }`,
			expected: object.NIL,
		},
		{
			name:     "clear",
			source:   `package main; func main() { clear([]int{1}) }`,
			expected: object.NIL,
		},
		{
			name:     "complex",
			source:   `package main; func main() { complex(1.0, 2.0) }`,
			expected: &object.SymbolicPlaceholder{},
		},
		{
			name:     "real",
			source:   `package main; func main() { real(1+2i) }`,
			expected: &object.SymbolicPlaceholder{},
		},
		{
			name:     "imag",
			source:   `package main; func main() { imag(1+2i) }`,
			expected: &object.SymbolicPlaceholder{},
		},
		{
			name:     "max",
			source:   `package main; func main() { max(1, 2) }`,
			expected: &object.SymbolicPlaceholder{},
		},
		{
			name:     "min",
			source:   `package main; func main() { min(1, 2) }`,
			expected: &object.SymbolicPlaceholder{},
		},
		{
			name:     "print",
			source:   `package main; func main() { print("hello") }`,
			expected: object.NIL,
		},
		{
			name:     "println",
			source:   `package main; func main() { println("hello") }`,
			expected: object.NIL,
		},
		{
			name:     "recover",
			source:   `package main; func main() { recover() }`,
			expected: object.NIL,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			dir, cleanup := scantest.WriteFiles(t, map[string]string{
				"go.mod":  "module example.com/me",
				"main.go": tt.source,
			})
			defer cleanup()

			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
				pkg := pkgs[0]
				eval := New(s, s.Logger, nil, nil, nil)
				env := object.NewEnclosedEnvironment(eval.UniverseEnv)
				eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)
				mainFunc, ok := env.Get("main")
				if !ok {
					return fmt.Errorf("function 'main' not found")
				}
				evaluated := eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)

				// most builtins return a value, which gets wrapped in a ReturnValue
				if ret, ok := evaluated.(*object.ReturnValue); ok {
					evaluated = ret.Value
				}

				switch expected := tt.expected.(type) {
				case *object.SymbolicPlaceholder:
					_, ok := evaluated.(*object.SymbolicPlaceholder)
					if !ok {
						t.Errorf("object is not SymbolicPlaceholder. got=%T (%+v)", evaluated, evaluated)
					}
				case object.Object:
					if evaluated != expected {
						t.Errorf("object has wrong value. got=%#v, want=%#v", evaluated, expected)
					}
				case string:
					errObj, ok := evaluated.(*object.Error)
					if !ok {
						t.Errorf("object is not Error. got=%T (%+v)", evaluated, evaluated)
						return nil
					}
					if errObj.Message != expected {
						t.Errorf("wrong error message. expected=%q, got=%q",
							expected, errObj.Message)
					}
				}
				return nil
			}

			if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
				t.Fatalf("scantest.Run() failed: %v", err)
			}
		})
	}
}

func TestLogWithContext(t *testing.T) {
	source := `
package main

func Do() {
	var ch chan int
	select {
	case <-ch:
		// This function call will cause an error during evaluation,
		// which should trigger a warning inside evalSelectStmt.
		undefinedFunc()
	}
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// Use a custom logger to capture output.
		// The scanner's logger is passed to the evaluator.
		s.Logger = logger

		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)

		env := object.NewEnclosedEnvironment(eval.UniverseEnv)
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		doFuncObj, _ := env.Get("Do")
		doFunc := doFuncObj.(*object.Function)
		// We don't care about the result, just that the logger was called.
		eval.Apply(ctx, doFunc, []object.Object{}, pkg)
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	t.Logf("captured log: %s", logBuf.String())

	// The logger now produces a stream of JSON objects, one per line.
	// We need to find the specific log entry we care about.
	scanner := bufio.NewScanner(&logBuf)
	var found bool
	for scanner.Scan() {
		line := scanner.Bytes()
		var logged map[string]any
		if err := json.Unmarshal(line, &logged); err != nil {
			continue // Skip lines that are not valid JSON
		}

		// We are looking for the WARN log from the select case.
		if level, ok := logged["level"]; !ok || level != "WARN" {
			continue
		}
		if msg, ok := logged["msg"]; !ok || msg != "error evaluating statement in select case" {
			continue
		}

		found = true
		// Check for context from the call stack
		if _, ok := logged["in_func"]; !ok {
			t.Error("expected log to have 'in_func' key, but it was missing")
		}
		if inFunc, ok := logged["in_func"].(string); !ok || inFunc != "Do" {
			t.Errorf("expected in_func to be 'Do', but got %q", inFunc)
		}
		break // Found the log we care about
	}

	if !found {
		t.Fatalf("did not find the expected warning log message")
	}
}

func TestEval_GoStmt(t *testing.T) {
	source := `
package main
func goFunc() {}
func main() {
	go goFunc()
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	var calledFunctions []object.Object

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)

		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) > 0 {
				calledFunctions = append(calledFunctions, args[0])
			}
			return nil
		})

		env := object.NewEnclosedEnvironment(eval.UniverseEnv)
		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

		mainFunc, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)

		if len(calledFunctions) == 0 {
			return fmt.Errorf("go function call was not tracked")
		}

		fn, ok := calledFunctions[0].(*object.Function)
		if !ok {
			return fmt.Errorf("tracked object is not a function, got %T", calledFunctions[0])
		}

		if fn.Name.Name != "goFunc" {
			return fmt.Errorf("expected tracked function to be 'goFunc', but got '%s'", fn.Name.Name)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEval_DeferStmt(t *testing.T) {
	source := `
package main
func deferredFunc() {}
func main() {
	defer deferredFunc()
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	var calledFunctions []object.Object

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)

		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) > 0 {
				calledFunctions = append(calledFunctions, args[0])
			}
			return nil
		})

		env := object.NewEnclosedEnvironment(eval.UniverseEnv)
		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

		mainFunc, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)

		if len(calledFunctions) == 0 {
			return fmt.Errorf("deferred function call was not tracked")
		}

		fn, ok := calledFunctions[0].(*object.Function)
		if !ok {
			return fmt.Errorf("tracked object is not a function, got %T", calledFunctions[0])
		}

		if fn.Name.Name != "deferredFunc" {
			return fmt.Errorf("expected tracked function to be 'deferredFunc', but got '%s'", fn.Name.Name)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestDefaultIntrinsic_InterfaceMethodCall(t *testing.T) {
	source := `
package main

type Writer interface {
	Write(p []byte) (n int, err error)
}

func Do(w Writer) {
	w.Write(nil)
}

func main() {
	Do(nil)
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var calledPlaceholder *object.SymbolicPlaceholder

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)

		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) == 0 {
				return nil
			}
			if p, ok := args[0].(*object.SymbolicPlaceholder); ok {
				if p.UnderlyingMethod != nil {
					calledPlaceholder = p
				}
			}
			return nil
		})

		env := object.NewEnclosedEnvironment(eval.UniverseEnv)
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed: %s", err.Message)
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if calledPlaceholder == nil {
		t.Fatalf("default intrinsic was not called with a symbolic placeholder for the interface method")
	}
	if calledPlaceholder.UnderlyingMethod.Name != "Write" {
		t.Errorf("expected placeholder for method 'Write', but got '%s'", calledPlaceholder.UnderlyingMethod.Name)
	}
	if calledPlaceholder.Receiver == nil {
		t.Errorf("expected placeholder to have a receiver, but it was nil")
	}
	if _, ok := calledPlaceholder.Receiver.(*object.Variable); !ok {
		t.Errorf("expected receiver to be a variable, but got %T", calledPlaceholder.Receiver)
	}
}

func TestEvalBlockStatement(t *testing.T) {
	source := `package main
func main() {
	var a = 5
	a
}`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)
		env := object.NewEnclosedEnvironment(eval.UniverseEnv)

		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

		mainFunc, _ := env.Get("main")
		result := eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)

		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("expected return value, got %T", result)
		}
		if diff := cmp.Diff(&object.Integer{Value: 5}, retVal.Value); diff != "" {
			return fmt.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		return nil
	}
	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestIdentifierNotFoundLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	source := `
package main
func main() {
    x := myUndefinedVar
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		allowAll := func(string) bool { return true }
		eval := New(s, logger, nil, nil, allowAll)

		env := object.NewEnclosedEnvironment(eval.UniverseEnv)
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFunc, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		eval.Apply(ctx, mainFunc, []object.Object{}, pkg)

		// The action doesn't need to return an error for a test failure.
		// We check the log buffer after the action completes.
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	// Check if the log contains the expected "in_func" attribute.
	t.Logf("captured log:\n%s", buf.String())
	if !strings.Contains(buf.String(), `"in_func":"main"`) {
		t.Errorf("expected log to contain 'in_func:main', but it didn't")
	}
	if !strings.Contains(buf.String(), `"msg":"identifier not found: myUndefinedVar"`) {
		t.Errorf("expected log to contain 'identifier not found' error, but it didn't")
	}
}

func TestEvalReturnStatement(t *testing.T) {
	input := `return 10`
	source := fmt.Sprintf("package main\nfunc main() { %s }", input)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", source, parser.ParseComments)
	if err != nil {
		t.Fatalf("could not parse file: %v", err)
	}
	stmt := f.Decls[0].(*ast.FuncDecl).Body.List[0]

	s, _ := goscan.New()
	eval := New(s, nil, nil, nil, nil)
	evaluated := eval.Eval(t.Context(), stmt, eval.UniverseEnv, &scanner.PackageInfo{
		Name:     "main",
		Fset:     fset,
		AstFiles: map[string]*ast.File{"main.go": f},
	})

	retVal, ok := evaluated.(*object.ReturnValue)
	if !ok {
		t.Fatalf("object is not ReturnValue. got=%T (%+v)", evaluated, evaluated)
	}

	integer, ok := retVal.Value.(*object.Integer)
	if !ok {
		t.Fatalf("return value is not Integer. got=%T (%+v)", retVal.Value, retVal.Value)
	}

	if integer.Value != 10 {
		t.Errorf("integer has wrong value. want=%d, got=%d", 10, integer.Value)
	}
}

func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"unknown operator", `"foo" > "bar"`},
		{"mismatched types", `"hello" - 5`},
		{"undefined variable", `x + 5`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := parser.ParseExpr(tt.input)
			if err != nil {
				t.Fatalf("could not parse expression: %v", err)
			}
			eval := New(nil, nil, nil, nil, nil)
			evaluated := eval.Eval(t.Context(), node, eval.UniverseEnv, nil)

			if evaluated == nil {
				t.Fatal("evaluation resulted in nil object")
			}
		})
	}
}

func TestBranchStmt(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedVisits []string
	}{
		{
			name: "for loop with continue",
			input: `
package main
func main() {
	var y int
	for i := 0; i < 10; i++ {
		if i == 1 {
			y = 2 // should be visited
			continue
		}
		y = 3
	}
}
`,
			expectedVisits: []string{
				"y = 2",
				"continue",
			},
		},
		{
			name: "for loop with break",
			input: `
package main
func main() {
	var y int
	for i := 0; i < 10; i++ {
		if i == 0 {
			y = 2 // should be visited
			break
		}
		y = 3
	}
	y = 4 // should be visited
}
`,
			expectedVisits: []string{
				"y = 2",
				"break",
				"y = 4",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, cleanup := scantest.WriteFiles(t, map[string]string{
				"go.mod":  "module example.com/main",
				"main.go": tt.input,
			})
			defer cleanup()

			var visitedNodes []string
			tracer := object.TracerFunc(func(node ast.Node) {
				if node == nil {
					return
				}
				switch node.(type) {
				case *ast.AssignStmt, *ast.BranchStmt:
					var buf bytes.Buffer
					printer.Fprint(&buf, token.NewFileSet(), node)
					visitedNodes = append(visitedNodes, buf.String())
				}
			})

			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
				pkg := pkgs[0]
				e := New(s, s.Logger, tracer, nil, nil)
				env := object.NewEnclosedEnvironment(e.UniverseEnv)
				for _, file := range pkg.AstFiles {
					e.Eval(ctx, file, env, pkg)
				}
				mainFunc, ok := env.Get("main")
				if !ok {
					return fmt.Errorf("main function not found")
				}
				result := e.Apply(ctx, mainFunc, []object.Object{}, pkg)
				if _, ok := result.(*object.Error); ok {
					return fmt.Errorf("evaluation failed: %v", result.Inspect())
				}

				// Assertions now happen inside the action
				for _, expected := range tt.expectedVisits {
					found := false
					for _, visited := range visitedNodes {
						if strings.Contains(visited, expected) {
							found = true
							break
						}
					}
					if !found {
						return fmt.Errorf("expected to visit node containing %q, but it was not visited.\nVisited nodes:\n%s", expected, strings.Join(visitedNodes, "\n"))
					}
				}

				// NOTE: With the current simple symbolic 'if' evaluation, which explores
				// all branches, we cannot assert that code after a break/continue is
				// NOT visited. The 'if' statement's evaluator would need to be more
				// complex to handle path-specific termination.
				//
				// This test's primary value is ensuring that the break/continue statements
				// themselves are processed without crashing the evaluator and that the
				// statements preceding them are correctly visited.
				return nil
			}

			if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
				t.Fatalf("scantest.Run() failed: %v", err)
			}
		})
	}
}

func TestEvalIfElseStmt(t *testing.T) {
	input := `if x > 0 { 10 } else { 20 }` // use a symbolic condition
	source := fmt.Sprintf("package main\nvar x int\nfunc main() { %s }", input)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", source, parser.ParseComments)
	if err != nil {
		t.Fatalf("could not parse file: %v", err)
	}
	node := f.Decls[1].(*ast.FuncDecl).Body.List[0] // decls[1] is main func

	s, _ := goscan.New()
	eval := New(s, nil, nil, nil, nil)
	env := object.NewEnclosedEnvironment(eval.UniverseEnv)
	// put a symbolic 'x' in the environment
	env.Set("x", &object.SymbolicPlaceholder{Reason: "variable x"})
	evaluated := eval.Eval(t.Context(), node, env, &scanner.PackageInfo{
		Name:     "main",
		Fset:     fset,
		AstFiles: map[string]*ast.File{"main.go": f},
	})

	if evaluated != nil {
		t.Fatalf("expected result of if statement to be nil, but got %T (%+v)", evaluated, evaluated)
	}
}

func TestEvalFunctionDeclaration(t *testing.T) {
	source := `
package main
func add(a, b int) int { return a + b }
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)
		env := object.NewEnclosedEnvironment(eval.UniverseEnv)

		// Eval the whole file to populate the environment
		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

		fn, ok := env.Get("add")
		if !ok {
			return fmt.Errorf("function add not found in environment")
		}

		if fn.Type() != object.FUNCTION_OBJ {
			return fmt.Errorf("expected function object, got %s", fn.Type())
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvalFunctionApplication(t *testing.T) {
	source := `
package main
func add(a, b int) int { return a + b }
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)
		env := object.NewEnclosedEnvironment(eval.UniverseEnv)

		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

		fn, _ := env.Get("add")
		args := []object.Object{
			&object.Integer{Value: 5},
			&object.Integer{Value: 5},
		}

		result := eval.applyFunction(ctx, fn, args, pkg, token.NoPos)

		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("result is not ReturnValue, got %T", result)
		}
		integer, ok := retVal.Value.(*object.Integer)
		if !ok {
			return fmt.Errorf("return value is not Integer, got %T", retVal.Value)
		}
		if want := int64(10); integer.Value != want {
			return fmt.Errorf("integer has wrong value. want=%d, got=%d", want, integer.Value)
		}
		return nil
	}
	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvalClosures(t *testing.T) {
	source := `
package main
func newAdder(x int) func(int) int {
	return func(y int) int { return x + y }
}
func main() {
	addTwo := newAdder(2)
	addTwo(3)
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)
		env := object.NewEnclosedEnvironment(eval.UniverseEnv)

		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

		newAdder, _ := env.Get("newAdder")
		addTwoFnResult := eval.applyFunction(ctx, newAdder, []object.Object{&object.Integer{Value: 2}}, pkg, token.NoPos)
		if isError(addTwoFnResult) {
			return fmt.Errorf("calling newAdder failed: %s", addTwoFnResult.Inspect())
		}
		addTwoFnRet, ok := addTwoFnResult.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("newAdder did not return a ReturnValue, got %T", addTwoFnResult)
		}
		addTwoFn := addTwoFnRet.Value

		result := eval.applyFunction(ctx, addTwoFn, []object.Object{&object.Integer{Value: 3}}, pkg, token.NoPos)
		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("addTwo did not return a value, got %T", result)
		}
		integer, ok := retVal.Value.(*object.Integer)
		if !ok {
			return fmt.Errorf("return value is not Integer, got %T", retVal.Value)
		}
		if want := int64(5); integer.Value != want {
			return fmt.Errorf("integer has wrong value. want=%d, got=%d", want, integer.Value)
		}
		return nil
	}
	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvalLetStatements(t *testing.T) {
	// This test is skipped because `let` is not a Go keyword.
}

func TestTypeCoercionInBinaryExpr(t *testing.T) {
	// This test is currently a placeholder.
}

func TestArrayLiterals(t *testing.T) {
	// This test is skipped because Go arrays are different from Monkey arrays.
	// Slice literals are tested in evaluator_slice_test.go.
}

func TestHashLiterals(t *testing.T) {
	// This test is skipped because Go maps are different from Monkey hashes.
	// Map literals (composite literals) are tested separately.
}

func TestArrayIndexExpressions(t *testing.T) {
	// This test is skipped because Go array/slice indexing is different.
	// See evaluator_slice_test.go for slice indexing tests.
}

func TestHashIndexExpressions(t *testing.T) {
	// This test is skipped because Go map indexing is different.
}

func TestErrorObject(t *testing.T) {
	err := &object.Error{Message: "test error 1"}
	if diff := cmp.Diff("symgo runtime error: test error 1\n", err.Inspect()); diff != "" {
		t.Errorf("Inspect() mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(object.ERROR_OBJ, err.Type()); diff != "" {
		t.Errorf("Type() mismatch (-want +got):\n%s", diff)
	}
}

func TestDefaultIntrinsic(t *testing.T) {
	source := `
package main
func add(a, b int) int { return a + b }
func main() {
	add(1, 2)
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	var calledFunctions []object.Object

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil, nil)

		// Register the default intrinsic to track function calls
		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) > 0 {
				calledFunctions = append(calledFunctions, args[0]) // first arg is the function object
			}
			return nil
		})

		env := object.NewEnclosedEnvironment(eval.UniverseEnv)
		eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

		mainFunc, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("function 'main' not found")
		}

		// Execute main, which should trigger the call to 'add'
		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)

		if len(calledFunctions) != 1 {
			return fmt.Errorf("expected 1 function call to be tracked, got %d", len(calledFunctions))
		}

		fn, ok := calledFunctions[0].(*object.Function)
		if !ok {
			return fmt.Errorf("tracked object is not a function, got %T", calledFunctions[0])
		}

		if fn.Name.Name != "add" {
			return fmt.Errorf("expected tracked function to be 'add', but got '%s'", fn.Name.Name)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
