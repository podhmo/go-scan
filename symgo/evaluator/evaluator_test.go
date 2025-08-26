package evaluator

import (
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
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvalIntegerLiteral(t *testing.T) {
	input := "5"
	node, err := parser.ParseExpr(input)
	if err != nil {
		t.Fatalf("could not parse expression: %v", err)
	}

	s, _ := goscan.New()
	eval := New(s, nil, nil, nil)
	evaluated := eval.Eval(context.Background(), node, object.NewEnvironment(), nil)

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

	eval := New(nil, nil, nil, nil)
	evaluated := eval.Eval(context.Background(), node, object.NewEnvironment(), nil)

	str, ok := evaluated.(*object.String)
	if !ok {
		t.Fatalf("object is not String. got=%T (%+v)", evaluated, evaluated)
	}

	if str.Value != "hello world" {
		t.Errorf("String has wrong value. want=%q, got=%q", "hello world", str.Value)
	}
}

func TestEvalUnsupportedLiteral(t *testing.T) {
	input := "5.5" // float
	node, err := parser.ParseExpr(input)
	if err != nil {
		t.Fatalf("could not parse expression: %v", err)
	}

	eval := New(nil, nil, nil, nil)
	evaluated := eval.Eval(context.Background(), node, object.NewEnvironment(), nil)

	errObj, ok := evaluated.(*object.Error)
	if !ok {
		t.Fatalf("object is not Error. got=%T (%+v)", evaluated, evaluated)
	}

	expected := "unsupported literal type: FLOAT"
	if errObj.Message != expected {
		t.Errorf("wrong error message. want=%q, got=%q", expected, errObj.Message)
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
		eval := New(s, s.Logger, nil, nil)
		env := object.NewEnvironment()

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
	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
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
		eval := New(s, s.Logger, nil, nil)

		env := object.NewEnvironment()
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		doFuncObj, _ := env.Get("Do")
		doFunc := doFuncObj.(*object.Function)
		// We don't care about the result, just that the logger was called.
		eval.Apply(ctx, doFunc, []object.Object{}, pkg)
		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	t.Logf("captured log: %s", logBuf.String())

	var logged map[string]any
	if err := json.Unmarshal(logBuf.Bytes(), &logged); err != nil {
		t.Fatalf("failed to unmarshal log output: %v", err)
	}

	// Check that the level is WARN
	if level, ok := logged["level"]; !ok || level != "WARN" {
		t.Errorf("expected log level to be WARN, but got %v", level)
	}

	// Check for context from the call stack
	if _, ok := logged["in_func"]; !ok {
		t.Error("expected log to have 'in_func' key, but it was missing")
	}
	if inFunc, ok := logged["in_func"].(string); !ok || inFunc != "undefinedFunc" {
		t.Errorf("expected in_func to be 'undefinedFunc', but got %q", inFunc)
	}

	if _, ok := logged["in_func_pos"]; !ok {
		t.Error("expected log to have 'in_func_pos' key, but it was missing")
	}
	// The call is at line 10, column 3 in the source string
	if pos, ok := logged["in_func_pos"].(string); !ok || !strings.HasSuffix(pos, "main.go:10:3") {
		t.Errorf("expected in_func_pos to end with 'main.go:10:3', but got %q", pos)
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
		eval := New(s, s.Logger, nil, nil)

		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) > 0 {
				calledFunctions = append(calledFunctions, args[0])
			}
			return nil
		})

		env := object.NewEnvironment()
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

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
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
		eval := New(s, s.Logger, nil, nil)

		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) > 0 {
				calledFunctions = append(calledFunctions, args[0])
			}
			return nil
		})

		env := object.NewEnvironment()
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

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
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
		eval := New(s, s.Logger, nil, nil)

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

		env := object.NewEnvironment()
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

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
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
		eval := New(s, s.Logger, nil, nil)
		env := object.NewEnvironment()

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
	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvalUnsupportedNode(t *testing.T) {
	input := "chan int"
	node, err := parser.ParseExpr(input)
	if err != nil {
		t.Fatalf("could not parse expression: %v", err)
	}

	eval := New(nil, nil, nil, nil)
	evaluated := eval.Eval(context.Background(), node, object.NewEnvironment(), nil)

	errObj, ok := evaluated.(*object.Error)
	if !ok {
		t.Fatalf("object is not Error. got=%T (%+v)", evaluated, evaluated)
	}

	expected := "evaluation not implemented for *ast.ChanType"
	if errObj.Message != expected {
		t.Errorf("wrong error message. want=%q, got=%q", expected, errObj.Message)
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
	eval := New(s, nil, nil, nil)
	evaluated := eval.Eval(context.Background(), stmt, object.NewEnvironment(), &scanner.PackageInfo{
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
			eval := New(nil, nil, nil, nil)
			evaluated := eval.Eval(context.Background(), node, object.NewEnvironment(), nil)

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
				e := New(s, s.Logger, tracer, nil)
				env := object.NewEnvironment()
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

			if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
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
	eval := New(s, nil, nil, nil)
	env := object.NewEnvironment()
	// put a symbolic 'x' in the environment
	env.Set("x", &object.SymbolicPlaceholder{Reason: "variable x"})
	evaluated := eval.Eval(context.Background(), node, env, &scanner.PackageInfo{
		Name:     "main",
		Fset:     fset,
		AstFiles: map[string]*ast.File{"main.go": f},
	})

	placeholder, ok := evaluated.(*object.SymbolicPlaceholder)
	if !ok {
		t.Fatalf("object is not SymbolicPlaceholder. got=%T (%+v)", evaluated, evaluated)
	}

	expectedReason := "if/else statement"
	if placeholder.Reason != expectedReason {
		t.Errorf("placeholder has wrong reason. want=%q, got=%q", expectedReason, placeholder.Reason)
	}
}

func TestEvalFunctionDeclaration(t *testing.T) {
	input := `
package main
func add(a, b int) int { return a + b }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", input, parser.ParseComments)
	if err != nil {
		t.Fatalf("could not parse file: %v", err)
	}

	env := object.NewEnvironment()
	eval := New(nil, nil, nil, nil)
	eval.Eval(context.Background(), f, env, &scanner.PackageInfo{
		Name:     "main",
		Fset:     fset,
		AstFiles: map[string]*ast.File{"main.go": f},
	})

	fn, ok := env.Get("add")
	if !ok {
		t.Fatal("function add not found in environment")
	}

	if fn.Type() != object.FUNCTION_OBJ {
		t.Errorf("expected function object, got %s", fn.Type())
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
		eval := New(s, s.Logger, nil, nil)
		env := object.NewEnvironment()

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
	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
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
		eval := New(s, s.Logger, nil, nil)
		env := object.NewEnvironment()

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
	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvalBuiltinFunctions(t *testing.T) {
	input := `len("four")`
	node, err := parser.ParseExpr(input)
	if err != nil {
		t.Fatalf("could not parse expression: %v", err)
	}

	s, _ := goscan.New()
	eval := New(s, nil, nil, nil)
	evaluated := eval.Eval(context.Background(), node, object.NewEnvironment(), nil)

	if _, ok := evaluated.(*object.Error); !ok {
		t.Fatalf("expected an error for undefined function, but got %T", evaluated)
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
	if diff := cmp.Diff("Error: test error 1", err.Inspect()); diff != "" {
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
		eval := New(s, s.Logger, nil, nil)

		// Register the default intrinsic to track function calls
		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) > 0 {
				calledFunctions = append(calledFunctions, args[0]) // first arg is the function object
			}
			return nil
		})

		env := object.NewEnvironment()
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

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
