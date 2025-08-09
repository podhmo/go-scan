package evaluator

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo2/object"
)

// testEval is a helper function to parse and evaluate a string of code.
func testEval(t *testing.T, input string) object.Object {
	t.Helper()
	// To parse statements, we need to wrap the input in a valid Go file structure.
	fullSource := fmt.Sprintf("package main\n\nfunc main() {\n%s\n}", input)

	// Create a temporary file to hold the source code.
	tmpfile, err := os.CreateTemp("", "minigo2_test_*.go")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpfile.Name()) })

	if _, err := tmpfile.Write([]byte(fullSource)); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, tmpfile.Name(), nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse code: %v", err)
		return nil
	}

	var mainFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" {
			mainFunc = fn
			break
		}
	}
	if mainFunc == nil {
		t.Fatalf("main function not found in parsed code")
		return nil
	}

	scanner, err := goscan.New()
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}
	// For single-file tests, we create a dummy file scope.
	fscope := object.NewFileScope(file)
	packages := make(map[string]*object.Package)
	eval := New(fset, scanner, object.NewSymbolRegistry(), packages)
	env := object.NewEnvironment()

	evaluated := eval.Eval(mainFunc.Body, env, fscope)
	if retVal, ok := evaluated.(*object.ReturnValue); ok {
		return retVal.Value
	}
	return evaluated
}

// testIntegerObject is a helper to check if an object is an Integer with the expected value.
func testIntegerObject(t *testing.T, obj object.Object, expected int64) bool {
	t.Helper()

	result, ok := obj.(*object.Integer)
	if !ok {
		t.Errorf("object is not Integer. got=%T (%+v)", obj, obj)
		return false
	}
	if result.Value != expected {
		t.Errorf("object has wrong value. got=%d, want=%d", result.Value, expected)
		return false
	}
	return true
}

func testErrorObject(t *testing.T, obj object.Object, expectedMessage string) bool {
	t.Helper()
	errObj, ok := obj.(*object.Error)
	if !ok {
		t.Errorf("object is not Error. got=%T (%+v)", obj, obj)
		return false
	}
	if errObj.Message != expectedMessage {
		t.Errorf("wrong error message. expected=%q, got=%q", expectedMessage, errObj.Message)
		return false
	}
	return true
}

// testStringObject is a helper to check if an object is a String
// with the expected value.
func testStringObject(t *testing.T, obj object.Object, expected string) bool {
	t.Helper()
	result, ok := obj.(*object.String)
	if !ok {
		t.Errorf("object is not String. got=%T (%+v)", obj, obj)
		return false
	}
	if result.Value != expected {
		t.Errorf("object has wrong value. got=%q, want=%q", result.Value, expected)
		return false
	}
	return true
}

// testBooleanObject is a helper to check if an object is a Boolean
// with the expected value.
func testBooleanObject(t *testing.T, obj object.Object, expected bool) bool {
	t.Helper()
	result, ok := obj.(*object.Boolean)
	if !ok {
		t.Errorf("object is not Boolean. got=%T (%+v)", obj, obj)
		return false
	}
	if result.Value != expected {
		t.Errorf("object has wrong value. got=%t, want=%t", result.Value, expected)
		return false
	}
	return true
}

// testNilObject is a helper to check if an object is Nil.
func testNilObject(t *testing.T, obj object.Object) bool {
	t.Helper()
	if obj != object.NIL {
		t.Errorf("object is not NIL. got=%T (%+v)", obj, obj)
		return false
	}
	return true
}

func TestFunctionObject(t *testing.T) {
	input := "func(x int) { x + 2; };"
	evaluated := testEval(t, input)
	fn, ok := evaluated.(*object.Function)
	if !ok {
		t.Fatalf("object is not Function. got=%T (%+v)", evaluated, evaluated)
	}
	if fn.Parameters == nil || len(fn.Parameters.List) != 1 {
		t.Fatalf("function has wrong parameters. Parameters=%+v", fn.Parameters)
	}
	if fn.Parameters.List[0].Names[0].String() != "x" {
		t.Fatalf("parameter is not 'x'. got=%q", fn.Parameters.List[0].Names[0].String())
	}
	// The exact string representation of the body is not critical to test here,
	// as long as we know it's a block statement.
	if fn.Body == nil {
		t.Fatalf("function body is nil")
	}
}

func TestFunctionApplication(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"identity := func(x int) { x; }; identity(5);", 5},
		{"identity := func(x int) { return x; }; identity(5);", 5},
		{"double := func(x int) { x * 2; }; double(5);", 10},
		{"add := func(x int, y int) { x + y; }; add(5, 5);", 10},
		{"add := func(x int, y int) { x + y; }; add(5 + 5, add(5, 5));", 20},
		{"func() { 5; }();", 5},
		{
			`
			newAdder := func(x int) {
				return func(y int) { x + y; };
			};
			addTwo := newAdder(2);
			addTwo(3);
			`,
			5,
		},
		{
			`
			fib := func(n int) {
				if (n < 2) {
					return n;
				}
				return fib(n-1) + fib(n-2);
			};
			fib(10);
			`,
			55,
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			testIntegerObject(t, testEval(t, tt.input), tt.expected)
		})
	}
}

func TestInterfaces(t *testing.T) {
	tests := []struct {
		input    string
		expected any
	}{
		{
			`
			type Shaper interface { Area() int }
			type Rect struct { width, height int }
			func (r Rect) Area() int { return r.width * r.height }
			func main() {
				var s Shaper = Rect{width: 10, height: 5}
				return s.Area()
			}
			`,
			int64(50),
		},
		{
			`
			type Greeter interface { Greet() string }
			type Person struct { name string }
			func (p *Person) Greet() string { return "Hello, " + p.name }
			func main() {
				var g Greeter = &Person{name: "Taro"}
				return g.Greet()
			}
			`,
			"Hello, Taro",
		},
		{
			`
			type Abc interface { A(); B() }
			type Def struct {}
			func (d Def) A() {}
			func main() {
				var v Abc = Def{}
			}
			`,
			"type Def does not implement Abc (missing method B)",
		},
		{
			`
			type Abc interface { A(x int) }
			type Def struct {}
			func (d Def) A() {}
			func main() {
				var v Abc = Def{}
			}
			`,
			"method A has wrong number of parameters (got 0, want 1)",
		},
		{
			`
			type Shaper interface { Area() int }
			func main() {
				var s Shaper
				return s.Area()
			}
			`,
			"nil pointer dereference (interface is nil)",
		},
		// New test cases start here
		{
			`
			type Speaker interface { Speak() string }
			type Dog struct {}
			func (d Dog) Speak() string { return "Woof!" }
			func main() {
				var s Speaker = Dog{}
				return s.Speak()
			}
			`,
			"Woof!",
		},
		{
			`
			type Nullable interface { Do() }
			func main() {
				var n Nullable = nil
				return n
			}
			`,
			"nil-interface",
		},
		{
			`
			type Abc interface { A() int }
			type Def struct {}
			func (d Def) A() {}
			func main() {
				var v Abc = Def{}
			}
			`,
			"method A has wrong number of return values (got 0, want 1)",
		},
		{
			`
			type Abc interface { A() }
			type Def struct {}
			func main() {
				var v Abc = &Def{}
			}
			`,
			"type Def does not implement Abc (missing method A)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEvalFile(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				if expected == "nil-interface" {
					iface, ok := evaluated.(*object.InterfaceInstance)
					if !ok {
						t.Errorf("expected a nil-interface, but got %T", evaluated)
					} else if iface.Value.Type() != object.NIL_OBJ {
						t.Errorf("expected interface to hold a nil value, but it holds %s", iface.Value.Type())
					}
				} else if err, ok := evaluated.(*object.Error); ok {
					if !strings.Contains(err.Inspect(), expected) {
						t.Errorf("expected error message to contain %q, but it did not.\nFull message:\n%s", expected, err.Inspect())
					}
				} else {
					testStringObject(t, evaluated, expected)
				}
			case nil:
				testNilObject(t, evaluated)
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
		})
	}
}

// testEvalFile is a helper that evaluates a full source file content.
// It evaluates all top-level declarations and then executes the main function.
func testEvalFile(t *testing.T, input string) object.Object {
	t.Helper()
	fullSource := "package main\n" + input
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", fullSource, 0)
	if err != nil {
		t.Fatalf("ParseFile error: %v\nSource:\n%s", err, fullSource)
	}

	scanner, err := goscan.New()
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}
	env := object.NewEnvironment()
	fscope := object.NewFileScope(file)
	packages := make(map[string]*object.Package)
	eval := New(fset, scanner, object.NewSymbolRegistry(), packages)

	var mainFunc *ast.FuncDecl

	// Evaluate all top-level declarations first
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" {
			mainFunc = fn
			continue // Don't evaluate main yet
		}
		result := eval.Eval(decl, env, fscope)
		if isError(result) {
			return result // Return early if a top-level declaration fails
		}
	}

	// Then evaluate the main function
	if mainFunc == nil {
		// If there's no main, there's nothing to execute.
		// We can return NIL or a specific indicator. Let's return NIL.
		return object.NIL
	}

	evaluated := eval.Eval(mainFunc.Body, env, fscope)
	if retVal, ok := evaluated.(*object.ReturnValue); ok {
		return retVal.Value
	}
	// If the last statement is an expression, Eval returns its value.
	return evaluated
}

func TestMethodCalls(t *testing.T) {
	tests := []struct {
		input    string
		expected any
	}{
		{
			`
			type Point struct {
				x int
				y int
			}
			func (p Point) Add() int {
				return p.x + p.y
			}
			func main() {
				p := Point{x: 3, y: 4}
				return p.Add()
			}
			`,
			int64(7),
		},
		{
			`
			type Counter struct {
				count int
			}
			func (c *Counter) Inc() {
				c.count = c.count + 1
			}
			func main() {
				c := &Counter{count: 0}
				c.Inc()
				c.Inc()
				return c.count
			}
			`,
			int64(2),
		},
		{
			`
			type Rect struct {
				width int
				height int
			}
			func (r Rect) Area() int {
				return r.width * r.height
			}
			func main() {
				r := Rect{width: 5, height: 10}
				return r.Area()
			}
			`,
			int64(50),
		},
		{
			`
			type Rect struct {
				width int
				height int
			}
			func (r *Rect) Scale(factor int) {
				r.width = r.width * factor
				r.height = r.height * factor
			}
			func main() {
				r := &Rect{width: 5, height: 10}
				r.Scale(2)
				return r.width + r.height
			}
			`,
			int64(30), // (5*2) + (10*2)
		},
		{
			`
			type Foo struct {
				val int
			}
			func (f Foo) GetVal() int {
				return f.val
			}
			func (f *Foo) SetVal(v int) {
				f.val = v
			}
			func main() {
				f := &Foo{val: 10}
				f.SetVal(20)
				return f.GetVal()
			}
			`,
			int64(20),
		},
		{
			`
			type Bar struct { v int }
			func (b Bar) Val() int { return b.v }
			func main() {
				b := Bar{v: 1}
				m := b.Val
				b.v = 2
				return m()
			}
			`,
			int64(1),
		},
		{
			`
			type Bar struct { v int }
			func (b *Bar) Val() int { return b.v }
			func main() {
				b := &Bar{v: 1}
				m := b.Val
				b.v = 2
				return m()
			}
			`,
			int64(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEvalFile(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				testErrorObject(t, evaluated, expected)
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
		})
	}
}

func TestMultipleReturnValues(t *testing.T) {
	tests := []struct {
		input string
		// probes is a map of expressions to test after running the input,
		// and their expected values.
		probes map[string]any
	}{
		{
			input: `f := func() { return 1, "hello" }; a, b := f()`,
			probes: map[string]any{
				"a": int64(1),
				"b": "hello",
			},
		},
		{
			input: `f := func() { return 1, "hello" }; var a int; var b string; a, b = f()`,
			probes: map[string]any{
				"a": int64(1),
				"b": "hello",
			},
		},
		{
			input: `f := func() { return 1 + 2, "a" + "b" }; x, y := f()`,
			probes: map[string]any{
				"x": int64(3),
				"y": "ab",
			},
		},
		{
			input:  `f := func() { return 1, true, 3 }; a, b := f()`,
			probes: map[string]any{"error": "assignment mismatch: 2 variables but 3 values"},
		},
		{
			input:  `f := func() { return 1, true }; a, b, c := f()`,
			probes: map[string]any{"error": "assignment mismatch: 3 variables but 2 values"},
		},
		{
			input:  `f := func() { return 1, true }; a := f()`,
			probes: map[string]any{"error": "multi-value function call in single-value context"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// For error checks, we can evaluate the input directly.
			if expectedErr, ok := tt.probes["error"]; ok {
				evaluated := testEval(t, tt.input)
				testErrorObject(t, evaluated, expectedErr.(string))
				return
			}

			// For success cases, we build a program that includes the setup
			// and then returns the variable we want to probe.
			for probeExpr, expected := range tt.probes {
				// The final expression in the block is its return value.
				fullInput := tt.input + "; " + probeExpr
				evaluated := testEval(t, fullInput)

				switch exp := expected.(type) {
				case int64:
					testIntegerObject(t, evaluated, exp)
				case string:
					testStringObject(t, evaluated, exp)
				case bool:
					testBooleanObject(t, evaluated, exp)
				default:
					t.Fatalf("unsupported probe type: %T", exp)
				}
			}
		})
	}
}

func TestStructEmbedding(t *testing.T) {
	tests := []struct {
		input    string
		expected any
	}{
		// Simple, one-level embedding
		{
			`
			type Point struct { x int; y int }
			type Circle struct {
				Point
				radius int
			}
			c := Circle{Point: Point{x: 1, y: 2}, radius: 10}
			return c.x
			`,
			int64(1),
		},
		{
			`
			type Point struct { x int; y int }
			type Circle struct {
				Point
				radius int
			}
			c := Circle{Point: Point{x: 1, y: 2}, radius: 10}
			return c.y
			`,
			int64(2),
		},
		{
			`
			type Point struct { x int; y int }
			type Circle struct {
				Point
				radius int
			}
			c := Circle{Point: Point{x: 1, y: 2}, radius: 10}
			return c.radius
			`,
			int64(10),
		},
		// Multi-level embedding
		{
			`
			type A struct { a int }
			type B struct { A; b int }
			type C struct { B; c int }
			instance := C{B: B{A: A{a: 1}, b: 2}, c: 3}
			return instance.a
			`,
			int64(1),
		},
		// Field shadowing
		{
			`
			type A struct { val int }
			type B struct {
				A
				val int
			}
			instance := B{A: A{val: 100}, val: 200}
			return instance.val
			`,
			int64(200),
		},
		// Access via pointer to outer struct
		{
			`
			type Point struct { x int }
			type Figure struct { Point; name string }
			f := Figure{Point: Point{x: 99}, name: "fig"}
			p := &f
			return p.x
			`,
			int64(99),
		},
		// Access via pointer to embedded struct
		{
			`
			type Point struct { x int }
			type Figure struct { *Point; name string }
			p := &Figure{Point: &Point{x: 99}, name: "fig"}
			return p.x
			`,
			int64(99),
		},
		// Error: field not found
		{
			`
			type A struct { a int }
			type B struct { A; b int }
			instance := B{A: A{a: 1}, b: 2}
			return instance.c
			`,
			"undefined field or method 'c' on struct 'B'",
		},
		// Error: nil pointer dereference on embedded pointer
		{
			`
			type Point struct { x int }
			type Figure struct { *Point; name string }
			p := &Figure{name: "fig"} // Point is nil
			return p.x
			`,
			"undefined field or method 'x' on struct 'Figure'", // It's undefined because the path to it is nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				testErrorObject(t, evaluated, expected)
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
		})
	}
}

func TestVariadicFunctions(t *testing.T) {
	tests := []struct {
		input    string
		expected any
	}{
		{
			`
			sum := func(nums ...int) {
				total := 0
				for _, num := range nums {
					total = total + num
				}
				return total
			}
			sum(1, 2, 3, 4)
			`,
			int64(10),
		},
		{
			`
			sum := func(nums ...int) {
				total := 0
				for _, num := range nums {
					total = total + num
				}
				return total
			}
			sum()
			`,
			int64(0),
		},
		{
			`
			print := func(s string, vals ...int) {
				// This test just checks if the call works and returns the first value
				if len(vals) > 0 {
					return vals[0]
				}
				return -1
			}
			print("hello", 10, 20)
			`,
			int64(10),
		},
		{
			`
			print := func(s string, vals ...int) {
				return len(s)
			}
			print("hello")
			`,
			int64(5), // string length
		},
		{
			`
			f := func(a int, b ...int) {}
			f()
			`,
			"wrong number of arguments for variadic function. got=0, want at least 1",
		},
		{
			`
			f := func(a ...int) {
				return a[1]
			}
			f(1, 2, 3)
			`,
			int64(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				testErrorObject(t, evaluated, expected)
			default:
				t.Fatalf("unsupported test type: %T", expected)
			}
		})
	}
}

func TestForRangeStatements(t *testing.T) {
	tests := []struct {
		input    string
		expected any
	}{
		// Array iteration
		{
			`
			sum := 0;
			for _, v := range []int{1, 2, 3} {
				sum = sum + v;
			}
			sum
			`,
			int64(6),
		},
		{
			`
			sum := 0;
			indices := 0;
			for i, v := range []int{10, 20, 30} {
				indices = indices + i;
				sum = sum + v;
			}
			sum + indices
			`,
			int64(63), // sum=60, indices=3
		},
		// String iteration
		{
			`
			sum := 0;
			for _, r := range "abc" { // runes 'a', 'b', 'c' are 97, 98, 99
				sum = sum + r;
			}
			sum
			`,
			int64(97 + 98 + 99),
		},
		// Map iteration - order is not guaranteed, so we test existence and sum
		{
			`
			sum := 0;
			m := map[string]int{"a": 1, "b": 2, "c": 3};
			for k, v := range m {
				if k == "a" { sum = sum + v; }
				if k == "b" { sum = sum + v; }
				if k == "c" { sum = sum + v; }
			}
			sum
			`,
			int64(6),
		},
		// Break statement
		{
			`
			sum := 0;
			for _, v := range []int{1, 2, 3, 4, 5} {
				sum = sum + v;
				if v == 3 {
					break;
				}
			}
			sum
			`,
			int64(6), // 1 + 2 + 3
		},
		// Continue statement
		{
			`
			sum := 0;
			for _, v := range []int{1, 2, 3, 4, 5} {
				if v % 2 == 0 {
					continue;
				}
				sum = sum + v;
			}
			sum
			`,
			int64(9), // 1 + 3 + 5
		},
		// Shadowing
		{
			`
			v := 100;
			sum := 0;
			for _, v := range []int{1, 2, 3} {
				sum = sum + v;
			}
			sum + v
			`,
			int64(106), // sum is 6, outer v is 100
		},
		// Empty array
		{
			`
			sum := 1;
			for _, v := range []int{} {
				sum = 100;
			}
			sum
			`,
			int64(1),
		},
		// Error case: ranging over integer
		{
			`for i := range 123 {}`,
			"range operator not supported for INTEGER",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				testErrorObject(t, evaluated, expected)
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
		})
	}
}

func TestIndexExpressions(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
	}{
		{"var a = []int{1, 2, 3}; a[0]", 1},
		{"var a = []int{1, 2, 3}; a[1]", 2},
		{"var a = []int{1, 2, 3}; a[2]", 3},
		{"var i = 0; var a = []int{1}; a[i]", 1},
		{"var a = []int{1, 2, 3}; a[1 + 1]", 3},
		{"var myArray = []int{1, 2, 3}; myArray[2]", 3},
		{"var myArray = []int{1, 2, 3}; myArray[0] + myArray[1] + myArray[2]", 6},
		{"var myArray = []int{1, 2, 3}; var i = myArray[0]; myArray[i]", 2},
		{"var a = []int{1, 2, 3}; a[3]", nil},
		{"var a = []int{1, 2, 3}; a[-1]", nil},
		{`var m = map[string]int{"foo": 5}; m["foo"]`, 5},
		{`var myMap = map[string]int{"foo": 5}; myMap["foo"]`, 5},
		{`var m = map[string]int{"foo": 5}; m["bar"]`, nil},
		{`var m = map[string]int{}; m["foo"]`, nil},
		{`var m = map[int]int{5: 5}; m[5]`, 5},
		{`var m = map[bool]int{true: 5}; m[true]`, 5},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int:
				testIntegerObject(t, evaluated, int64(expected))
			case nil:
				testNilObject(t, evaluated)
			default:
				t.Errorf("unsupported expected type %T", expected)
			}
		})
	}
}

func TestIndexErrors(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"var a = 5; a[0]", "index operator not supported for INTEGER"},
		{"var a = []int{1, 2, 3}; a[\"a\"]", "index into array is not an integer"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			errObj, ok := evaluated.(*object.Error)
			if !ok {
				t.Fatalf("expected error object, got %T (%+v)", evaluated, evaluated)
			}
			if errObj.Message != tt.expected {
				t.Errorf("wrong error message. expected=%q, got=%q", tt.expected, errObj.Message)
			}
		})
	}
}

func TestArrayLiterals(t *testing.T) {
	input := "[]int{1, 2 * 2, 3 + 3}"

	evaluated := testEval(t, input)
	result, ok := evaluated.(*object.Array)
	if !ok {
		t.Fatalf("object is not Array. got=%T (%+v)", evaluated, evaluated)
	}

	if len(result.Elements) != 3 {
		t.Fatalf("array has wrong num of elements. got=%d", len(result.Elements))
	}

	testIntegerObject(t, result.Elements[0], 1)
	testIntegerObject(t, result.Elements[1], 4)
	testIntegerObject(t, result.Elements[2], 6)
}

func TestMapLiterals(t *testing.T) {
	input := `
		map[string]int{
			"one": 10 - 9,
			"two": 2,
			"three": 6 / 2,
		}
	`
	evaluated := testEval(t, input)
	result, ok := evaluated.(*object.Map)
	if !ok {
		t.Fatalf("object is not Map. got=%T (%+v)", evaluated, evaluated)
	}

	expected := map[object.HashKey]int64{
		(&object.String{Value: "one"}).HashKey():   1,
		(&object.String{Value: "two"}).HashKey():   2,
		(&object.String{Value: "three"}).HashKey(): 3,
	}

	if len(result.Pairs) != len(expected) {
		t.Fatalf("map has wrong num of pairs. got=%d", len(result.Pairs))
	}

	for expectedKey, expectedValue := range expected {
		pair, ok := result.Pairs[expectedKey]
		if !ok {
			t.Errorf("no pair for given key in Pairs")
			continue
		}

		testIntegerObject(t, pair.Value, expectedValue)
	}
}

func TestErrorHandling(t *testing.T) {
	tests := []struct {
		input    string
		expected any // string for single message, []string for multiple substrings
	}{
		{
			"5 + true;",
			"type mismatch: INTEGER + BOOLEAN",
		},
		{
			"5 + true; 5;",
			"type mismatch: INTEGER + BOOLEAN",
		},
		{
			"-true",
			"unknown operator: -BOOLEAN",
		},
		{
			"true + false;",
			"unknown operator for booleans: +",
		},
		{
			"5; true + false; 5",
			"unknown operator for booleans: +",
		},
		{
			"if (10 > 1) { true + false; }",
			"unknown operator for booleans: +",
		},
		{
			`
			if (10 > 1) {
				if (10 > 1) {
					return true + false;
				}
				return 1;
			}
			`,
			"unknown operator for booleans: +",
		},
		{
			"foobar",
			"identifier not found: foobar",
		},
		{
			`"Hello" - "World"`,
			"unknown operator: STRING - STRING",
		},
		{
			`x := 1; x(1)`,
			"not a function: INTEGER",
		},
		{
			`
			f := func(x int) {
				g := func() {
					x / 0;
				}
				g();
			}
			f(10);
			`,
			"division by zero",
		},
		{
			`
			bar := func() {
				"hello" - "world"
			};
			foo := func() {
				bar()
			};
			foo();
			`,
			[]string{
				"runtime error: unknown operator: STRING - STRING",
				"in bar",
				`"hello" - "world"`,
				"in foo",
				"bar()",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			errObj, ok := evaluated.(*object.Error)
			if !ok {
				t.Fatalf("no error object returned. got=%T(%+v)", evaluated, evaluated)
			}

			switch expected := tt.expected.(type) {
			case string:
				if errObj.Message != expected {
					t.Errorf("wrong error message. expected=%q, got=%q", expected, errObj.Message)
				}
			case []string:
				fullMessage := errObj.Inspect()
				for _, sub := range expected {
					if !strings.Contains(fullMessage, sub) {
						t.Errorf("expected error message to contain %q, but it did not.\nFull message:\n%s", sub, fullMessage)
					}
				}
			}
		})
	}
}

func TestConstDeclarations(t *testing.T) {
	tests := []struct {
		input    string
		expected any // int64 or error message string
	}{
		{"const x = 10; x", int64(10)},
		{"const x = 10; const y = 20; y", int64(20)},
		{"const x = 10; var y = x; y", int64(10)},
		{"const x = 10; x = 20;", "cannot assign to constant x"},
		{"const ( a = 1 ); a = 2", "cannot assign to constant a"},
		{"const ( a = iota ); a", int64(0)},
		{"const ( a = iota; b ); b", int64(1)},
		{"const ( a = iota; b; c ); c", int64(2)},
		{"const ( a = 10; b; c ); c", int64(10)},
		{"const ( a = 10; b = 20; c ); c", int64(20)},
		{"const ( a = 1 << iota; b; c; d ); d", int64(8)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				testErrorObject(t, evaluated, expected)
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
		})
	}
}

func TestSwitchStatements(t *testing.T) {
	tests := []struct {
		input    string
		expected any // int64 or "nil"
	}{
		// Basic switch with tag
		{"x := 2; switch x { case 1: 10; case 2: 20; case 3: 30; };", int64(20)},
		// Tag evaluation
		{"switch 1 + 1 { case 1: 10; case 2: 20; };", int64(20)},
		// Default case
		{"x := 4; switch x { case 1: 10; case 2: 20; default: 99; };", int64(99)},
		// No matching case, no default
		{"x := 4; switch x { case 1: 10; case 2: 20; };", nil},
		// Expressionless switch (switch true)
		{"x := 10; switch { case x > 5: 100; case x < 5: 200; };", int64(100)},
		{"x := 1; switch { case x > 5: 100; case x < 5: 200; };", int64(200)},
		// Case with multiple expressions
		{"x := 3; switch x { case 1, 2, 3: 30; default: 99; };", int64(30)},
		{"x := 4; switch x { case 1, 2, 3: 30; default: 99; };", int64(99)},
		// Switch with init statement
		{"switch x := 2; x { case 2: 20; };", int64(20)},
		// Init statement shadowing
		{"x := 10; switch x := 2; x { case 2: 20; }; x", int64(10)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case nil:
				testNilObject(t, evaluated)
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
		})
	}
}

func TestForStatements(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{
			`
			var a = 0;
			for i := 0; i < 10; i = i + 1 {
				a = a + 1;
			}
			a;
			`,
			10,
		},
		{
			`
			var a = 0;
			for {
				a = a + 1;
				if a > 5 {
					break;
				}
			}
			a;
			`,
			6,
		},
		{
			`
			var a = 0;
			var i = 0;
			for i < 10 {
				i = i + 1;
				if i % 2 == 0 {
					continue;
				}
				a = a + 1;
			}
			a;
			`,
			5, // Only increments for odd numbers (1, 3, 5, 7, 9)
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testIntegerObject(t, evaluated, tt.expected)
		})
	}
}

func TestIfElseStatements(t *testing.T) {
	tests := []struct {
		input    string
		expected any // int64 or nil
	}{
		{"if (true) { 10 }", int64(10)},
		{"if (false) { 10 }", nil},
		{"if (1) { 10 }", int64(10)}, // 1 is truthy
		{"if (1 < 2) { 10 }", int64(10)},
		{"if (1 > 2) { 10 }", nil},
		{"if (1 > 2) { 10 } else { 20 }", int64(20)},
		{"if (1 < 2) { 10 } else { 20 }", int64(10)},
		{"x := 10; if (x > 5) { 100 } else { 200 };", int64(100)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case nil:
				testNilObject(t, evaluated)
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
		})
	}
}

func TestEvalBooleanExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"true == true", true},
		{"false == false", true},
		{"true == false", false},
		{"true != false", true},
		{"false != true", true},
		{"(1 < 2) == true", true},
		{"(1 > 2) == false", true},
		{"1 < 2", true},
		{"1 > 2", false},
		{"1 < 1", false},
		{"1 > 1", false},
		{"1 == 1", true},
		{"1 != 1", false},
		{"1 == 2", false},
		{"1 != 2", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testBooleanObject(t, evaluated, tt.expected)
		})
	}
}

func TestEvalIntegerExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"5", 5},
		{"10", 10},
		{"-5", -5},
		{"-10", -10},
		{"5 + 5 + 5 + 5 - 10", 10},
		{"2 * 2 * 2 * 2 * 2", 32},
		{"-50 + 100 + -50", 0},
		{"5 * 2 + 10", 20},
		{"5 + 2 * 10", 25},
		{"20 + 2 * -10", 0},
		{"50 / 2 * 2 + 10", 60},
		{"2 * (5 + 10)", 30},
		{"3 * 3 * 3 + 10", 37},
		{"3 * (3 * 3) + 10", 37},
		{"(5 + 10 * 2 + 15 / 3) * 2 + -10", 50},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testIntegerObject(t, evaluated, tt.expected)
		})
	}
}

func TestEvalStringExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"hello"`, "hello"},
		{`"hello" + " " + "world"`, "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testStringObject(t, evaluated, tt.expected)
		})
	}
}

func TestVarStatements(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"var a = 5; a;", 5},
		{"var a = 5 * 5; a;", 25},
		{"var a = 5; var b = a; b;", 5},
		{"var a = 5; var b = a; var c = a + b + 5; c;", 15},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testIntegerObject(t, evaluated, tt.expected)
		})
	}
}

func TestAssignmentStatements(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"var a = 5; a = 10; a;", 10},
		{"var a = 5; var b = 10; a = b; a;", 10},
		{"var a = 5; { a = 10; }; a;", 10}, // assignment affects outer scope
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testIntegerObject(t, evaluated, tt.expected)
		})
	}
}

func TestShortVarDeclarations(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"a := 5; a;", 5},
		{"a := 5 * 5; a;", 25},
		{"a := 5; b := a; b;", 5},
		{"a := 5; b := a; c := a + b + 5; c;", 15},
		{"a := 5; a = 10; a;", 10},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testIntegerObject(t, evaluated, tt.expected)
		})
	}
}

func TestStructs(t *testing.T) {
	tests := []struct {
		input    string
		expected any
	}{
		{
			`
			type Person struct {
				name string
				age int
			}
			p := Person{name: "Alice", age: 30}
			p.name
			`,
			"Alice",
		},
		{
			`
			type Point struct {
				x int
				y int
			}
			p := Point{x: 3, y: 5}
			p.x + p.y
			`,
			int64(8),
		},
		{
			`
			type User struct {
				active bool
				name string
			}
			u := User{name: "Bob", active: false}
			u.active = true
			u.active
			`,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				testStringObject(t, evaluated, expected)
			case bool:
				testBooleanObject(t, evaluated, expected)
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
		})
	}
}

func TestPointers(t *testing.T) {
	tests := []struct {
		input    string
		expected any
	}{
		// Basic pointer operations
		{"var a = 10; var p = &a; return *p;", int64(10)},
		{"var a = 10; var p = &a; *p = 20; return a;", int64(20)},
		{"var a = 10; var p = &a; var b = *p; return b;", int64(10)},

		// Pointer to struct
		{
			`
			type T struct { V int }
			var t = T{V: 1}
			var p = &t
			return p.V
			`,
			int64(1),
		},
		{
			`
			type T struct { V int }
			var t = T{V: 1}
			var p = &t
			p.V = 2
			return t.V
			`,
			int64(2),
		},

		// new() function
		{
			`
			type T struct { V int }
			var p = new(T)
			return p.V
			`,
			nil, // Fields are zero-initialized to NULL for now
		},
		{
			`
			type T struct { V int }
			var p = new(T)
			p.V = 5
			return p.V
			`,
			int64(5),
		},

		// Error cases
		{"return *10", "invalid indirect of 10 (type INTEGER)"},
		{"var a = 10; return *a", "invalid indirect of 10 (type INTEGER)"},
		{"return &10", "cannot take the address of *ast.BasicLit"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			switch expected := tt.expected.(type) {
			case int64:
				testIntegerObject(t, evaluated, expected)
			case string:
				testErrorObject(t, evaluated, expected)
			case nil:
				testNilObject(t, evaluated)
			default:
				t.Fatalf("unsupported expected type for test: %T", expected)
			}
		})
	}
}

func TestLexicalScoping(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"var a = 5; { var a = 10; }; a;", 5}, // shadowing with var
		{"a := 5; { a := 10; }; a;", 5},       // shadowing with :=
		{"var a = 5; { var b = 10; }; a;", 5},
		{"var a = 5; { b := 10; }; a;", 5},
		{"a := 5; { b := a; }; a;", 5},
		{"var a = 1; { var a = 2; { var a = 3; }; a; }; a;", 1},
		{"var a = 1; { a = 2; { a = 3; }; a; }; a;", 3}, // nested assignment
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			evaluated := testEval(t, tt.input)
			testIntegerObject(t, evaluated, tt.expected)
		})
	}
}
