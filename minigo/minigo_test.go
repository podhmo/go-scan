package minigo

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo/object"
)

func TestNewInterpreter(t *testing.T) {
	_, err := NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}
}

func TestInterpreterEval_SimpleExpression(t *testing.T) {
	input := `package main
var x = 1 + 2`
	i, err := NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}

	if err := i.LoadFile("test.go", []byte(input)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

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
	i, err := NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}

	// Register the "fmt" package with the Println function
	i.Register("fmt", map[string]any{
		"Println": fmt.Println,
	})

	if err := i.LoadFile("test.go", []byte(input)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

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

	i, err := NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}

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

	i, err := NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}

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
	i, err := NewInterpreter(
		WithStdin(stdin),
		WithStdout(&stdout),
		WithStderr(&stderr),
	)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}

	if err := i.LoadFile("test.go", []byte(inputScript)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

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
			i, err := NewInterpreter(WithStdout(&stdout))
			if err != nil {
				t.Fatalf("NewInterpreter() failed: %v", err)
			}

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

			_, err = i.Execute(context.Background(), mainFunc, nil, fscope)
			if err != nil {
				t.Fatalf("Execute() failed: %v", err)
			}

			tt.check(t, i, &stdout)
		})
	}
}

func TestEvalLine(t *testing.T) {
	ctx := context.Background()

	t.Run("state persistence", func(t *testing.T) {
		i, err := NewInterpreter()
		if err != nil {
			t.Fatalf("NewInterpreter() failed: %v", err)
		}
		i.Register("strings", map[string]any{
			"ToUpper": strings.ToUpper,
		})

		// 1. Define a variable
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
		i, err := NewInterpreter()
		if err != nil {
			t.Fatalf("NewInterpreter() failed: %v", err)
		}

		// Syntax error
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
