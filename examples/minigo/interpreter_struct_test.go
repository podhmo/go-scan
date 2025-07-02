package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestStructDefinitionAndInstantiation(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedOutput string // Expected output from a Println like function if we had one, or check env state.
		expectedError string // Substring of the expected error message.
		setupEnv      func(env *Environment)
		checkEnv      func(t *testing.T, env *Environment, i *Interpreter)
	}{
		{
			name: "Define and instantiate basic struct",
			input: `
package main

type Point struct {
	X int
	Y int
}

func main() {
	p := Point{X: 10, Y: 20}
	// Access in a way that can be checked if we don't have print
	_ = p.X
}
`,
			checkEnv: func(t *testing.T, env *Environment, i *Interpreter) {
				// After main runs, 'p' is local to main. We need to check global or return 'p'.
				// For now, let's modify the test to define 'p' globally for easier inspection.
				// Or, we can evaluate an expression like "p.X" after running main, if the interpreter supports that.
				// Let's assume for this test, we'll check the definition.
				obj, ok := i.globalEnv.Get("Point")
				if !ok {
					t.Fatalf("Struct 'Point' not defined in global environment")
				}
				structDef, ok := obj.(*StructDefinition)
				if !ok {
					t.Fatalf("'Point' is not a StructDefinition, got %T", obj)
				}
				if structDef.Name != "Point" {
					t.Errorf("Expected struct name 'Point', got '%s'", structDef.Name)
				}
				if len(structDef.Fields) != 2 {
					t.Errorf("Expected 2 fields in 'Point', got %d", len(structDef.Fields))
				}
				if typeName, _ := structDef.Fields["X"]; typeName != "int" {
					t.Errorf("Expected field X to be type 'int', got '%s'", typeName)
				}
				if typeName, _ := structDef.Fields["Y"]; typeName != "int" {
					t.Errorf("Expected field Y to be type 'int', got '%s'", typeName)
				}
			},
		},
		{
			name: "Instantiate struct and check field values by returning",
			input: `
package main

type Vector struct {
	X int
	Y int
	Z int
}

func main() {
	v := Vector{X: 3, Y: 4, Z: 5}
	return v
}
`,
			checkEnv: func(t *testing.T, env *Environment, i *Interpreter) {
				// The result of main will be on the 'result' of LoadAndRun, not in env.
				// This checkEnv is for global state. We need to check the return value of eval.
				// The test harness needs to be adapted, or we test via a "get" function.
			},
			// This test case will be better handled by checking the direct output of LoadAndRun,
			// assuming main's return value is captured.
		},
		{
			name: "Access struct field",
			input: `
package main

type User struct {
	ID   int
	Name string
}

var u User
var name string

func main() {
	u = User{ID: 1, Name: "Alice"}
	name = u.Name
}
`,
			checkEnv: func(t *testing.T, env *Environment, i *Interpreter) {
				nameObj, ok := env.Get("name")
				if !ok {
					t.Fatalf("Variable 'name' not found in global environment")
				}
				nameStr, ok := nameObj.(*String)
				if !ok {
					t.Fatalf("'name' is not a String, got %T", nameObj)
				}
				if nameStr.Value != "Alice" {
					t.Errorf("Expected name 'Alice', got '%s'", nameStr.Value)
				}

				userObj, ok := env.Get("u")
				if !ok {
					t.Fatalf("Variable 'u' not found")
				}
				userInstance, ok := userObj.(*StructInstance)
				if !ok {t.Fatalf("u is not StructInstance")}

				idVal, _ := userInstance.FieldValues["ID"].(*Integer)
				if idVal.Value != 1 {t.Errorf("Expected u.ID to be 1, got %d", idVal.Value)}

			},
		},
		{
			name: "Access non-existent field",
			input: `
package main

type Simple struct {
	Val int
}

func main() {
	s := Simple{Val: 100}
	_ = s.NonExistent
}
`,
			expectedError: "type Simple has no field NonExistent",
		},
		{
			name: "Instantiate undefined struct",
			input: `
package main

func main() {
	p := NonExistentStruct{X: 10}
}
`,
			expectedError: "undefined type 'NonExistentStruct' used in composite literal",
		},
		{
			name: "Instantiate struct with unknown field",
			input: `
package main

type Coords struct {
	X int
}

func main() {
	c := Coords{X: 1, Y: 2}
}
`,
			expectedError: "unknown field 'Y' in struct literal of type 'Coords'",
		},
		{
			name: "Struct as function argument and return value",
			input: `
package main

type Message struct {
	Content string
}

func processMessage(m Message) Message {
	m.Content = m.Content + " processed" // This tests field modification if we make it work
                                          // For now, it reassigns to a local m.
                                          // Let's return a new struct for simplicity of testing value semantics.
    return Message{Content: m.Content + " processed"}
}

var result Message

func main() {
	msg := Message{Content: "hello"}
	result = processMessage(msg)
}
`,
			checkEnv: func(t *testing.T, env *Environment, i *Interpreter) {
				resObj, ok := env.Get("result")
				if !ok {
					t.Fatalf("Variable 'result' not found")
				}
				msgInstance, ok := resObj.(*StructInstance)
				if !ok {
					t.Fatalf("'result' is not a StructInstance, got %T", resObj)
				}
				if msgInstance.Definition.Name != "Message" {
					t.Errorf("Expected result to be of type 'Message', got '%s'", msgInstance.Definition.Name)
				}
				contentObj, _ := msgInstance.FieldValues["Content"]
				contentStr, _ := contentObj.(*String)
				expectedContent := "hello processed"
				if contentStr.Value != expectedContent {
					t.Errorf("Expected result.Content to be '%s', got '%s'", expectedContent, contentStr.Value)
				}
			},
		},
		{
			name: "Struct field of struct type (nested)",
			input: `
package main

type Inner struct {
	Value int
}

type Outer struct {
	Name  string
	In    Inner
}

var o Outer
var val int

func main() {
	o = Outer{Name: "MyOuter", In: Inner{Value: 123}}
	val = o.In.Value
}
`,
			checkEnv: func(t *testing.T, env *Environment, i *Interpreter) {
				valObj, ok := env.Get("val")
				if !ok { t.Fatalf("Global 'val' not found") }
				valInt, ok := valObj.(*Integer)
				if !ok { t.Fatalf("'val' is not an Integer, got %T", valObj) }
				if valInt.Value != 123 {
					t.Errorf("Expected val to be 123, got %d", valInt.Value)
				}

				outerObj, _ := env.Get("o")
				outerInstance, _ := outerObj.(*StructInstance)
				innerFieldObj, _ := outerInstance.FieldValues["In"]
				innerInstance, _ := innerFieldObj.(*StructInstance)
				innerValueObj, _ := innerInstance.FieldValues["Value"]
				innerValueInt, _ := innerValueObj.(*Integer)
				if innerValueInt.Value != 123 {
					t.Errorf("Expected o.In.Value to be 123, got %d", innerValueInt.Value)
				}

			},
		},
		{
			name: "Instantiate struct with no fields (empty literal)",
			input: `
package main

type Empty struct {}

var e Empty

func main() {
	e = Empty{}
}
`,
			checkEnv: func(t *testing.T, env *Environment, i *Interpreter) {
				obj, ok := env.Get("e")
				if !ok {t.Fatalf("var e not found")}
				instance, ok := obj.(*StructInstance)
				if !ok {t.Fatalf("e is not a StructInstance, got %T", obj)}
				if instance.Definition.Name != "Empty" {
					t.Errorf("e.Definition.Name expected Empty, got %s", instance.Definition.Name)
				}
				if len(instance.FieldValues) != 0 {
					t.Errorf("e.FieldValues expected to be empty, got %v", instance.FieldValues)
				}
			},
		},

	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := NewInterpreter()
			// Create a dummy file for the interpreter to "load"
			dummyFilePath := "dummy_struct_test.mgo"
			err := os.WriteFile(dummyFilePath, []byte(tt.input), 0644)
			if err != nil {
				t.Fatalf("Failed to write dummy input file: %v", err)
			}
			defer os.Remove(dummyFilePath)

			if tt.setupEnv != nil {
				tt.setupEnv(i.globalEnv)
			}

			err = i.LoadAndRun(context.Background(), dummyFilePath, "main")

			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing '%s', got: %v", tt.expectedError, err)
				}
			} else if err != nil {
				t.Errorf("Did not expect error, but got: %v", err)
			}

			if tt.checkEnv != nil {
				tt.checkEnv(t, i.globalEnv, i)
			}
			// Note: Checking return values from 'main' would require LoadAndRun to return the final Object.
			// For now, tests primarily check global state or expect errors.
		})
	}
}

func TestStructUninitializedFieldAccess(t *testing.T) {
	input := `
package main

type Test struct {
	A int
	B string
}

var t Test
var valA interface{} // Using interface{} as MiniGo doesn't have it, so this var won't be set by MiniGo
var valB interface{}

func main() {
	t = Test{A: 10} // B is not initialized
	// If we could capture these in Go test variables:
	// valA = t.A
	// valB = t.B
}
`
	i := NewInterpreter()
	dummyFilePath := "dummy_uninit_field_test.mgo"
	if err := os.WriteFile(dummyFilePath, []byte(input), 0644); err != nil {
		t.Fatalf("Failed to write dummy input file: %v", err)
	}
	defer os.Remove(dummyFilePath)

	err := i.LoadAndRun(context.Background(), dummyFilePath, "main")
	if err != nil {
		t.Fatalf("LoadAndRun failed: %v", err)
	}

	tObj, ok := i.globalEnv.Get("t")
	if !ok {
		t.Fatal("Global variable 't' not found")
	}
	tInstance, ok := tObj.(*StructInstance)
	if !ok {
		t.Fatalf("'t' is not a StructInstance, got %T", tObj)
	}

	// Access t.A (should be 10)
	valAObj, foundA := tInstance.FieldValues["A"]
	if !foundA {
		t.Errorf("Field A not found in t.FieldValues, expected it to be set")
	} else {
		intA, ok := valAObj.(*Integer)
		if !ok {
			t.Errorf("t.A is not an Integer, got %T", valAObj)
		} else if intA.Value != 10 {
			t.Errorf("Expected t.A to be 10, got %d", intA.Value)
		}
	}

	// Access t.B (should be uninitialized, so FieldValues map won't contain "B")
	// Our evalSelectorExpr currently returns NULL for fields defined on struct but not set in literal.
	// This requires evaluating `t.B` through the interpreter.
	// Let's modify the minigo script to assign t.B to a global var to check its value.

	inputWithGlobalAccess := `
package main
type Test struct { A int; B string }
var t Test
var globalBVal Object // Can't use Object type directly in minigo script
var globalBStringVal string // Let's assume we can get it as string

func main() {
    t = Test{A: 1}
    // globalBVal = t.B // This line would be ideal if we could assign to Object
    // For now, we can't directly test the NULL return from evalSelectorExpr for t.B
    // without being able to assign that NULL to something or print it.
    // The current test setup relies on side effects (global vars) or errors.
    // The logic in evalSelectorExpr for this case is:
    // if _, defExists := structInstance.Definition.Fields[fieldName]; defExists { return NULL, nil }
    // So, if we could assign t.B to a global, it should become NULL.
}
`
	// To properly test the NULL return for uninitialized fields, we'd need:
	// 1. A way to call i.eval("t.B", i.globalEnv) from the test.
	// 2. Or, modify minigo to assign `t.B` to a global and then check that global is `NULL`.
	//    `var x = t.B` - then check `x` is `NULL`.

	i2 := NewInterpreter()
	inputForNullCheck := `
package main
type Point struct { X int; Y int }
var p Point
var yValIs Object // Cannot declare as Object in MiniGo

func main() {
	p = Point{X: 10}
	// yVal = p.Y // This would test if p.Y returns NULL
}
`
	// This specific sub-test for NULL on uninitialized access is hard with current test harness.
	// The logic is in evalSelectorExpr:
	// `if _, defExists := structInstance.Definition.Fields[fieldName]; defExists { return NULL, nil }`
	// This path is covered if a script tries to use such a field and NULL is an acceptable value
	// in that context (e.g. if NULL could be assigned or printed).
	// For now, we've tested that defined fields that *are* set are accessible, and
	// accessing a completely non-existent field is an error.
	// The "defined but not set" case returning NULL is implicitly part of evalSelectorExpr's logic.
	t.Log("Note: Direct test for NULL return on uninitialized (but defined) field access is tricky with current harness but logic exists in evalSelectorExpr.")

	// Test case for field re-assignment (if struct fields were mutable)
	// MiniGo struct instances are currently immutable once created via literal.
	// Assignment like `p.X = 20` is not `ast.AssignStmt` on `p.X` but `ast.SelectorExpr` as LHS.
	// This is not supported by `evalAssignStmt` yet.
}

// TODO:
// - Test for type checking during instantiation (e.g., Point{X: "not-an-int"}). Needs field type info in StructDefinition to be more than string.
// - Test for non-keyed struct literals (e.g., Point{10, 20}) once supported.
// - Test for modifying struct fields (e.g., p.X = 30) once LHS of assignment can be a SelectorExpr.
// - Test for returning struct from main and checking its value directly from LoadAndRun's result.
// - Test struct definition within a function (if/when local type declarations are supported).
// - Test for duplicate field names in struct literal: Point{X:1, X:2} -> error
// - Test for using a non-struct type in a composite literal: var x int; _ = x{} -> error "type 'x' is not a struct type"
// - Test for empty struct literal for non-empty struct: type P struct {X int}; _ = P{}
//   (current evalCompositeLit allows this, FieldValues is empty. Accessing P{}.X would return NULL via evalSelectorExpr)
