package minigo

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo/object"
)

func newTestInterpreter(t *testing.T, opts ...Option) *Interpreter {
	t.Helper()
	s, err := goscan.New(goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}
	interp, err := NewInterpreter(s, opts...)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}
	return interp
}

func TestNewInterpreter(t *testing.T) {
	newTestInterpreter(t)
}

func TestInterpreterEval_SimpleExpression(t *testing.T) {
	input := `package main
var x = 1 + 2`
	i := newTestInterpreter(t)

	if err := i.LoadFile("test.go", []byte(input)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	var err error
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	val, ok := i.globalEnv.Get("x")
	if !ok {
		t.Fatalf("variable 'x' not found in environment")
	}

	integer, ok := val.(*object.Integer)
	if !ok {
		t.Fatalf("x is not Integer. got=%T (%+v)", val, val)
	}
	if integer.Value != 3 {
		t.Errorf("x should be 3. got=%d", integer.Value)
	}
}

func TestInterpreterEval_Import(t *testing.T) {
	input := `package main

import "fmt"

var x = fmt.Println
`
	i := newTestInterpreter(t)

	// Register the "fmt" package with the Println function
	i.Register("fmt", map[string]any{
		"Println": fmt.Println,
	})

	if err := i.LoadFile("test.go", []byte(input)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	var err error
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	val, ok := i.globalEnv.Get("x")
	if !ok {
		t.Fatalf("variable 'x' not found in environment")
	}

	_, ok = val.(*object.Builtin)
	if !ok {
		t.Fatalf("x is not Builtin. got=%T (%+v)", val, val)
	}
}

func TestInterpreterEval_MultiFileImportAlias(t *testing.T) {
	fileA := `package main
import f "fmt"
var resultA = f.FmtFunc()
`
	fileB := `package main
import f "strings"
var resultB = f.StringsFunc()
`

	i := newTestInterpreter(t)

	// Register mock functions that return unique strings
	i.Register("fmt", map[string]any{
		"FmtFunc": func() string { return "from fmt" },
	})
	i.Register("strings", map[string]any{
		"StringsFunc": func() string { return "from strings" },
	})

	// Load both files
	if err := i.LoadFile("file_a.go", []byte(fileA)); err != nil {
		t.Fatalf("LoadFile(A) failed: %v", err)
	}
	if err := i.LoadFile("file_b.go", []byte(fileB)); err != nil {
		t.Fatalf("LoadFile(B) failed: %v", err)
	}

	// Evaluate the loaded files
	var err error
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	// Check variable 'resultA'
	valA, okA := i.globalEnv.Get("resultA")
	if !okA {
		t.Fatalf("variable 'resultA' not found in environment")
	}
	strA, okA := valA.(*object.String)
	if !okA {
		t.Fatalf("resultA is not String. got=%T (%+v)", valA, valA)
	}
	if strA.Value != "from fmt" {
		t.Errorf("resultA has wrong value. got=%q, want=%q", strA.Value, "from fmt")
	}

	// Check variable 'resultB'
	valB, okB := i.globalEnv.Get("resultB")
	if !okB {
		t.Fatalf("variable 'resultB' not found in environment")
	}
	strB, okB := valB.(*object.String)
	if !okB {
		t.Fatalf("resultB is not String. got=%T (%+v)", valB, valB)
	}
	if strB.Value != "from strings" {
		t.Errorf("resultB has wrong value. got=%q, want=%q", strB.Value, "from strings")
	}
}

func TestInterpreterEval_SharedPackageInstance(t *testing.T) {
	fooFile := `package main
import x "sharedlib"
var valA = x.Get()
`
	barFile := `package main
import "sharedlib"
var valB = sharedlib.Get()
`

	i := newTestInterpreter(t)

	// Register a mock package
	i.Register("sharedlib", map[string]any{
		"Get": func() int { return 42 },
	})

	// Load both files
	if err := i.LoadFile("foo.go", []byte(fooFile)); err != nil {
		t.Fatalf("LoadFile(foo) failed: %v", err)
	}
	if err := i.LoadFile("bar.go", []byte(barFile)); err != nil {
		t.Fatalf("LoadFile(bar) failed: %v", err)
	}

	// Evaluate
	var err error
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	// Check that both variables were set correctly
	valA, okA := i.globalEnv.Get("valA")
	if !okA {
		t.Fatal("variable 'valA' not found")
	}
	intA, okA := valA.(*object.Integer)
	if !okA || intA.Value != 42 {
		t.Errorf("valA has wrong value, got %v, want 42", valA)
	}

	valB, okB := i.globalEnv.Get("valB")
	if !okB {
		t.Fatal("variable 'valB' not found")
	}
	intB, okB := valB.(*object.Integer)
	if !okB || intB.Value != 42 {
		t.Errorf("valB has wrong value, got %v, want 42", valB)
	}

	// Check that the package instance was shared
	if len(i.packages) != 1 {
		t.Errorf("expected 1 cached package, but found %d", len(i.packages))
	}
	if _, ok := i.packages["sharedlib"]; !ok {
		t.Errorf("expected to find 'sharedlib' in package cache, but it was not there")
	}
}

func TestInterpreter_WithIO(t *testing.T) {
	// We wrap calls in var declarations because the interpreter's Eval loop
	// only evaluates top-level declarations, not statements.
	inputScript := `package main

var _ = println("Please enter your name:")
var name = readln()
var _ = println("Hello,", name)
`
	stdin := strings.NewReader("Gopher\n")
	var stdout, stderr bytes.Buffer

	// Create a new interpreter with custom I/O
	i := newTestInterpreter(t,
		WithStdin(stdin),
		WithStdout(&stdout),
		WithStderr(&stderr),
	)

	if err := i.LoadFile("test.go", []byte(inputScript)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	var err error
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v\nStderr: %s", err, stderr.String())
	}

	// Verify stdout
	expectedOutput := "Please enter your name:\nHello, Gopher\n"
	if stdout.String() != expectedOutput {
		t.Errorf("wrong output to stdout.\ngot:\n%s\nwant:\n%s", stdout.String(), expectedOutput)
	}

	// Verify stderr is empty
	if stderr.String() != "" {
		t.Errorf("stderr should be empty, but got: %s", stderr.String())
	}

	// Verify the variable was set correctly
	val, ok := i.globalEnv.Get("name")
	if !ok {
		t.Fatalf("variable 'name' not found in environment")
	}
	str, ok := val.(*object.String)
	if !ok {
		t.Fatalf("name is not a String, got %T", val)
	}
	if str.Value != "Gopher" {
		t.Errorf("name has wrong value. got=%q, want=%q", str.Value, "Gopher")
	}
}

func TestInterpreterEval_Defer_Simple(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, i *Interpreter, stdout *bytes.Buffer)
	}{
		{
			name: "simple defer",
			input: `package main
var x = 1
func main() {
	defer func() { x = 10 }()
	x = 2
}`,
			check: func(t *testing.T, i *Interpreter, stdout *bytes.Buffer) {
				val, ok := i.globalEnv.Get("x")
				if !ok {
					t.Fatalf("variable 'x' not found")
				}
				integer, ok := val.(*object.Integer)
				if !ok {
					t.Fatalf("x is not Integer, got %T", val)
				}
				if integer.Value != 10 {
					t.Errorf("x should be 10, got %d", integer.Value)
				}
			},
		},
		{
			name: "LIFO order",
			input: `package main
import "fmt"
func main() {
	defer fmt.Print("1")
	defer fmt.Print("2")
	fmt.Print("0")
}`,
			check: func(t *testing.T, i *Interpreter, stdout *bytes.Buffer) {
				expected := "021"
				if got := stdout.String(); got != expected {
					t.Errorf("stdout got %q, want %q", got, expected)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			i := newTestInterpreter(t, WithStdout(&stdout))

			i.Register("fmt", map[string]any{
				"Print": func(s string) {
					fmt.Fprint(&stdout, s)
				},
			})

			if err := i.LoadFile("test.go", []byte(tt.input)); err != nil {
				t.Fatalf("LoadFile() failed: %v", err)
			}

			if err := i.EvalDeclarations(context.Background()); err != nil {
				t.Fatalf("EvalDeclarations() failed: %v", err)
			}

			// We need to execute main to test defer
			mainFunc, fscope, err := i.FindFunction("main")
			if err != nil {
				t.Fatalf("FindFunction('main') failed: %v", err)
			}

			if _, err := i.Execute(context.Background(), mainFunc, nil, fscope); err != nil {
				t.Fatalf("Execute() failed: %v", err)
			}

			tt.check(t, i, &stdout)
		})
	}
}

func TestInterpreter_TypedNilMethodValue(t *testing.T) {
	input := `package main
type MyStruct struct {}
func (s *MyStruct) MyMethod() {}

var method = (*MyStruct)(nil).MyMethod
`
	i := newTestInterpreter(t)

	if err := i.LoadFile("test.go", []byte(input)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	var err error
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	val, ok := i.globalEnv.Get("method")
	if !ok {
		t.Fatalf("variable 'method' not found in environment")
	}

	_, ok = val.(*object.GoMethodValue)
	if !ok {
		t.Fatalf("method is not GoMethodValue. got=%T (%+v)", val, val)
	}
}

func TestInterpreter_UntypedMapInSlice(t *testing.T) {
	input := `package main
var data = []map[string]int{
	{"a": 1},
	{"b": 2, "c": 3},
}
`
	i := newTestInterpreter(t)

	if err := i.LoadFile("test.go", []byte(input)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	var err error
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	val, ok := i.globalEnv.Get("data")
	if !ok {
		t.Fatalf("variable 'data' not found")
	}

	arr, ok := val.(*object.Array)
	if !ok {
		t.Fatalf("data is not an Array, got %T", val)
	}
	if len(arr.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(arr.Elements))
	}

	m1 := arr.Elements[0].(*object.Map)
	if len(m1.Pairs) != 1 {
		t.Fatalf("m1 should have 1 pair, got %d", len(m1.Pairs))
	}
	key1 := &object.String{Value: "a"}
	pair1, ok := m1.Pairs[key1.HashKey()]
	if !ok || pair1.Value.(*object.Integer).Value != 1 {
		t.Errorf("m1 has wrong data")
	}

	m2 := arr.Elements[1].(*object.Map)
	if len(m2.Pairs) != 2 {
		t.Fatalf("m2 should have 2 pairs, got %d", len(m2.Pairs))
	}
}

func TestInterpreter_PointerReceiverMethodCall(t *testing.T) {
	input := `package main

type Counter struct {
	value int
}

func (c *Counter) Inc() {
	c.value = c.value + 1
}

func (c *Counter) Get() int {
    return c.value
}

var c Counter
var p = &c
var _ = p.Inc()
var val = p.Get()
`
	i := newTestInterpreter(t)

	if err := i.LoadFile("test.go", []byte(input)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	var err error
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	// Check the final value
	result, ok := i.globalEnv.Get("val")
	if !ok {
		t.Fatalf("variable 'val' not found in environment")
	}
	integer, ok := result.(*object.Integer)
	if !ok {
		t.Fatalf("val is not Integer. got=%T (%+v)", result, result)
	}
	if integer.Value != 1 {
		t.Errorf("val should be 1. got=%d", integer.Value)
	}
}

func TestInterpreterEval_TypeConversions(t *testing.T) {
	input := `package main
var bs = []byte("hello")
var s = string([]byte{119, 111, 114, 108, 100}) // "world"
`
	i := newTestInterpreter(t)

	if err := i.LoadFile("test.go", []byte(input)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	var err error
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	// Check []byte("hello")
	valBs, ok := i.globalEnv.Get("bs")
	if !ok {
		t.Fatalf("variable 'bs' not found in environment")
	}
	arrayBs, ok := valBs.(*object.Array)
	if !ok {
		t.Fatalf("bs is not Array. got=%T (%+v)", valBs, valBs)
	}
	expectedBytes := []byte("hello")
	if len(arrayBs.Elements) != len(expectedBytes) {
		t.Fatalf("bs should have %d elements. got=%d", len(expectedBytes), len(arrayBs.Elements))
	}
	for i, el := range arrayBs.Elements {
		integerEl, ok := el.(*object.Integer)
		if !ok {
			t.Fatalf("element %d in bs is not Integer. got=%T", i, el)
		}
		if integerEl.Value != int64(expectedBytes[i]) {
			t.Errorf("element %d should be %d. got=%d", i, expectedBytes[i], integerEl.Value)
		}
	}

	// Check string([]byte{...})
	valS, ok := i.globalEnv.Get("s")
	if !ok {
		t.Fatalf("variable 's' not found in environment")
	}
	stringS, ok := valS.(*object.String)
	if !ok {
		t.Fatalf("s is not String. got=%T (%+v)", valS, valS)
	}
	if stringS.Value != "world" {
		t.Errorf("s should be 'world'. got=%q", stringS.Value)
	}
}

func TestInterpreterEval_ByteType(t *testing.T) {
	input := `package main
var b byte = 65
var bs []byte = []byte{66, 67, 68}
`
	i := newTestInterpreter(t)

	if err := i.LoadFile("test.go", []byte(input)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	var err error
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	// Check single byte variable
	valB, ok := i.globalEnv.Get("b")
	if !ok {
		t.Fatalf("variable 'b' not found in environment")
	}
	integerB, ok := valB.(*object.Integer)
	if !ok {
		t.Fatalf("b is not Integer. got=%T (%+v)", valB, valB)
	}
	if integerB.Value != 65 {
		t.Errorf("b should be 65. got=%d", integerB.Value)
	}

	// Check byte slice variable
	valBs, ok := i.globalEnv.Get("bs")
	if !ok {
		t.Fatalf("variable 'bs' not found in environment")
	}
	arrayBs, ok := valBs.(*object.Array)
	if !ok {
		t.Fatalf("bs is not Array. got=%T (%+v)", valBs, valBs)
	}
	if len(arrayBs.Elements) != 3 {
		t.Fatalf("bs should have 3 elements. got=%d", len(arrayBs.Elements))
	}

	expected := []int64{66, 67, 68}
	for i, el := range arrayBs.Elements {
		integerEl, ok := el.(*object.Integer)
		if !ok {
			t.Fatalf("element %d in bs is not Integer. got=%T", i, el)
		}
		if integerEl.Value != expected[i] {
			t.Errorf("element %d should be %d. got=%d", i, expected[i], integerEl.Value)
		}
	}
}

func TestInterpreter_SequentialDeclaration(t *testing.T) {
	t.Run("calling a function defined later should succeed after two-pass evaluation", func(t *testing.T) {
		input := `package main
var x = getX()
func getX() int {
	return 42
}
`
		i := newTestInterpreter(t)

		if err := i.LoadFile("test.go", []byte(input)); err != nil {
			t.Fatalf("LoadFile() failed: %v", err)
		}

		// With the old single-pass evaluator, this would fail.
		// The goal is to make this pass.
		var err error
		_, err = i.Eval(context.Background())
		if err != nil {
			t.Fatalf("Eval() failed unexpectedly: %v", err)
		}

		val, ok := i.globalEnv.Get("x")
		if !ok {
			t.Fatalf("variable 'x' not found")
		}
		integer, ok := val.(*object.Integer)
		if !ok {
			t.Fatalf("x is not an Integer, got %T", val)
		}
		if integer.Value != 42 {
			t.Errorf("x should be 42, got %d", integer.Value)
		}
	})
}

func TestInterpreterEval_DestructuringAssignment(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, i *Interpreter)
	}{
		{
			name: "simple assignment",
			input: `package main
var a, b = 1, 2
`,
			check: func(t *testing.T, i *Interpreter) {
				valA, _ := i.globalEnv.Get("a")
				valB, _ := i.globalEnv.Get("b")
				if valA.(*object.Integer).Value != 1 {
					t.Errorf("a should be 1, got %v", valA)
				}
				if valB.(*object.Integer).Value != 2 {
					t.Errorf("b should be 2, got %v", valB)
				}
			},
		},
		{
			name: "swap assignment",
			input: `package main
var a, b = 1, 2
var c, d = b, a
`,
			check: func(t *testing.T, i *Interpreter) {
				valC, _ := i.globalEnv.Get("c")
				valD, _ := i.globalEnv.Get("d")
				if valC.(*object.Integer).Value != 2 {
					t.Errorf("c should be 2, got %v", valC)
				}
				if valD.(*object.Integer).Value != 1 {
					t.Errorf("d should be 1, got %v", valD)
				}
			},
		},
		{
			name: "define assignment",
			input: `package main
func main() {
	a, b := 1, 2
	_ = a
	_ = b
}
`,
			check: nil, // Can't check local variables easily, just check for no error
		},
		{
			name: "define and swap",
			input: `package main
var a, b = 1, 2
func main() {
	c, d := b, a
	_ = c
	_ = d
}
`,
			check: nil, // Can't check local variables easily, just check for no error
		},
		{
			name: "existing var swap",
			input: `package main
var a, b = 1, 2
func main() {
	a, b = b, a
}
`,
			check: func(t *testing.T, i *Interpreter) {
				valA, _ := i.globalEnv.Get("a")
				valB, _ := i.globalEnv.Get("b")
				if valA.(*object.Integer).Value != 2 {
					t.Errorf("a should be 2 after swap, got %v", valA)
				}
				if valB.(*object.Integer).Value != 1 {
					t.Errorf("b should be 1 after swap, got %v", valB)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := newTestInterpreter(t)
			if err := i.LoadFile("test.go", []byte(tt.input)); err != nil {
				t.Fatalf("LoadFile() failed: %v", err)
			}
			if err := i.LoadFile("test.go", []byte(tt.input)); err != nil {
				t.Fatalf("LoadFile() for %s failed: %v", tt.name, err)
			}

			// First, evaluate all top-level declarations
			if err := i.EvalDeclarations(context.Background()); err != nil {
				t.Fatalf("EvalDeclarations() for %s failed: %v", tt.name, err)
			}

			// If there's a main function, execute it to test assignments
			var execErr error
			mainFunc, fscope, findErr := i.FindFunction("main")
			if findErr == nil {
				_, execErr = i.Execute(context.Background(), mainFunc, nil, fscope)
			}

			if findErr != nil && strings.Contains(tt.input, "main()") {
				t.Fatalf("Expected to find main function for %s, but didn't: %v", tt.name, findErr)
			}
			if execErr != nil {
				t.Fatalf("Execution failed unexpectedly for %s: %v", tt.name, execErr)
			}

			if tt.check != nil {
				tt.check(t, i)
			}
		})
	}
}

func TestEvalLine(t *testing.T) {
	ctx := context.Background()

	t.Run("state persistence", func(t *testing.T) {
		i := newTestInterpreter(t)
		i.Register("strings", map[string]any{
			"ToUpper": strings.ToUpper,
		})

		// 1. Define a variable
		var err error
		_, err = i.EvalLine(ctx, "x := 10")
		if err != nil {
			t.Fatalf("EvalLine 1 failed: %v", err)
		}

		// 2. Use the variable
		res, err := i.EvalLine(ctx, "x * 2")
		if err != nil {
			t.Fatalf("EvalLine 2 failed: %v", err)
		}
		if integer, ok := res.(*object.Integer); !ok || integer.Value != 20 {
			t.Errorf("Expected result to be 20, got %s", res.Inspect())
		}

		// 3. Import a package
		_, err = i.EvalLine(ctx, `import "strings"`)
		if err != nil {
			t.Fatalf("EvalLine 3 failed: %v", err)
		}

		// 4. Use the imported package
		res, err = i.EvalLine(ctx, `strings.ToUpper("gopher")`)
		if err != nil {
			t.Fatalf("EvalLine 4 failed: %v", err)
		}
		if str, ok := res.(*object.String); !ok || str.Value != "GOPHER" {
			t.Errorf("Expected result to be 'GOPHER', got %s", res.Inspect())
		}
	})

	t.Run("error handling", func(t *testing.T) {
		i := newTestInterpreter(t)

		// Syntax error
		var err error
		_, err = i.EvalLine(ctx, "x :=")
		if err == nil {
			t.Error("Expected a syntax error, but got nil")
		}

		// Runtime error
		_, err = i.EvalLine(ctx, "x + 1") // x is not defined
		if err == nil {
			t.Error("Expected a runtime error, but got nil")
		} else {
			if !strings.Contains(err.Error(), "identifier not found: x") {
				t.Errorf("Expected 'identifier not found' error, got: %v", err)
			}
		}
	})
}

func TestInterpreterEval_ComparisonOperators(t *testing.T) {
	input := `package main
var t1 = 1 < 2
var t2 = 1 > 2
var t3 = 1 == 1
var t4 = 1 != 1
var t5 = 1 <= 1
var t6 = 1 <= 2
var t7 = 1 >= 1
var t8 = 1 >= 2

var f1 = 2 < 1
var f2 = 2 > 1
var f3 = 2 == 1
var f4 = 2 != 1
var f5 = 2 <= 1
var f6 = 2 >= 3
`
	i := newTestInterpreter(t)

	if err := i.LoadFile("test.go", []byte(input)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	var err error
	_, err = i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}

	tests := []struct {
		name string
		want bool
	}{
		{"t1", true}, {"t2", false}, {"t3", true}, {"t4", false},
		{"t5", true}, {"t6", true}, {"t7", true}, {"t8", false},
		{"f1", false}, {"f2", true}, {"f3", false}, {"f4", true},
		{"f5", false}, {"f6", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := i.globalEnv.Get(tt.name)
			if !ok {
				t.Fatalf("variable '%s' not found", tt.name)
			}
			b, ok := val.(*object.Boolean)
			if !ok {
				t.Fatalf("variable '%s' is not a Boolean. got=%T", tt.name, val)
			}
			if b.Value != tt.want {
				t.Errorf("variable '%s' has wrong value. got=%v, want=%v", tt.name, b.Value, tt.want)
			}
		})
	}
}

func TestDefer(t *testing.T) {
	source := `
package main

func main() {
	println("start")
	defer println("deferred 1")
	defer println("deferred 2")
	println("end")
}
`
	var stdout, stderr bytes.Buffer
	i := newTestInterpreter(t, WithStdout(&stdout), WithStderr(&stderr))

	if err := i.LoadFile("main.go", []byte(source)); err != nil {
		t.Fatalf("LoadFile() failed: %+v", err)
	}

	_, err := i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %+v", err)
	}

	want := "start\nend\ndeferred 2\ndeferred 1\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout\n got: %q\nwant: %q", got, want)
	}
}

func TestDeferPanicRecover(t *testing.T) {
	source := `
package main

func main() {
	defer func() {
		if r := recover(); r != nil {
			println("recovered:", r)
		}
	}()
	panic("oh no")
}
`
	var stdout, stderr bytes.Buffer
	i := newTestInterpreter(t, WithStdout(&stdout), WithStderr(&stderr))

	if err := i.LoadFile("main.go", []byte(source)); err != nil {
		t.Fatalf("LoadFile() failed: %+v", err)
	}

	_, err := i.Eval(context.Background())
	// A panic that is recovered should not return an error from Eval.
	if err != nil {
		t.Fatalf("Eval() failed: %+v", err)
	}

	want := "recovered: oh no\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout\n got: %q\nwant: %q", got, want)
	}
}

func TestRecoverWithoutPanic(t *testing.T) {
	source := `
package main

func main() {
	r1 := recover()
	println("r1:", r1)
	defer func() {
		r2 := recover()
		println("r2:", r2)
	}()
	println("ok")
}
`
	var stdout, stderr bytes.Buffer
	i := newTestInterpreter(t, WithStdout(&stdout), WithStderr(&stderr))

	if err := i.LoadFile("main.go", []byte(source)); err != nil {
		t.Fatalf("LoadFile() failed: %+v", err)
	}

	_, err := i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %+v", err)
	}

	want := "r1: nil\nok\nr2: nil\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout\n got: %q\nwant: %q", got, want)
	}
}

func TestPanicWithoutRecover(t *testing.T) {
	source := `
package main
func main() {
	panic("unrecovered")
}
`
	var stdout, stderr bytes.Buffer
	i := newTestInterpreter(t, WithStdout(&stdout), WithStderr(&stderr))

	if err := i.LoadFile("main.go", []byte(source)); err != nil {
		t.Fatalf("LoadFile() failed: %+v", err)
	}
	_, err := i.Eval(context.Background())
	if err == nil {
		t.Fatal("Eval() should have failed, but it didn't")
	}

	// The error from Eval() should wrap a minigo.PanicError
	if !strings.Contains(err.Error(), "unrecovered") {
		t.Fatalf("expected a panic error containing 'unrecovered', but got %T: %v", err, err)
	}
}

func TestDeferWithNamedReturn(t *testing.T) {
	source := `
package main

func test() (i int) {
	i = 1
	defer func() {
		i = 2
	}()
	return 3
}

func main() {
	println(test())
}
`
	var stdout, stderr bytes.Buffer
	i := newTestInterpreter(t, WithStdout(&stdout), WithStderr(&stderr))

	if err := i.LoadFile("main.go", []byte(source)); err != nil {
		t.Fatalf("LoadFile() failed: %+v", err)
	}
	_, err := i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %+v", err)
	}

	want := "2\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout\n got: %q\nwant: %q", got, want)
	}
}

func TestDeferWithBareReturn(t *testing.T) {
	source := `
package main

func test() (i int) {
	i = 1
	defer func() {
		i = 2
	}()
	return
}

func main() {
	println(test())
}
`
	var stdout, stderr bytes.Buffer
	i := newTestInterpreter(t, WithStdout(&stdout), WithStderr(&stderr))

	if err := i.LoadFile("main.go", []byte(source)); err != nil {
		t.Fatalf("LoadFile() failed: %+v", err)
	}
	_, err := i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %+v", err)
	}
	want := "2\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout\n got: %q\nwant: %q", got, want)
	}
}

// This test verifies that a panic inside a deferred call replaces an existing panic.
func TestPanicInDefer(t *testing.T) {
	source := `
package main

func main() {
    defer func() {
        if r := recover(); r != nil {
            println("recovered in main:", r)
        }
    }()
    f()
}

func f() {
    defer func() {
        panic("panic in defer")
    }()
    panic("original panic")
}
`
	var stdout, stderr bytes.Buffer
	i := newTestInterpreter(t, WithStdout(&stdout), WithStderr(&stderr))

	if err := i.LoadFile("main.go", []byte(source)); err != nil {
		t.Fatalf("LoadFile() failed: %+v", err)
	}

	_, err := i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %+v", err)
	}

	// The `recover` in `main` should catch the *second* panic.
	want := "recovered in main: panic in defer\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout\n got: %q\nwant: %q", got, want)
	}
}

// This test verifies that arguments to a deferred call are evaluated when the defer statement is executed, not when the call is executed.
func TestDeferArgumentEvaluation(t *testing.T) {
	source := `
package main

func printArg(s string) {
	println(s)
}

func main() {
	s := "hello"
	defer printArg(s)
	s = "world"
}
`
	var stdout, stderr bytes.Buffer
	i := newTestInterpreter(t, WithStdout(&stdout), WithStderr(&stderr))

	if err := i.LoadFile("main.go", []byte(source)); err != nil {
		t.Fatalf("LoadFile() failed: %+v", err)
	}

	_, err := i.Eval(context.Background())
	if err != nil {
		t.Fatalf("Eval() failed: %+v", err)
	}

	// The argument "hello" should be captured at the time of the defer statement.
	want := "hello\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout\n got: %q\nwant: %q", got, want)
	}
}
