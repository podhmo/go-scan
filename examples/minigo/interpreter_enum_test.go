package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helper to create a temp .mgo file for testing
func createTempMgoFile(t *testing.T, nameHint string, content string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "minigo_enum_tests_"+nameHint)
	if err != nil {
		t.Fatalf("Failed to create temp dir for %s: %v", nameHint, err)
	}
	tmpFn := filepath.Join(dir, nameHint+".mgo")
	if err := os.WriteFile(tmpFn, []byte(content), 0666); err != nil {
		t.Fatalf("Failed to write to temp file %s: %v", tmpFn, err)
	}
	// Add cleanup for the whole directory created for the test case
	t.Cleanup(func() { os.RemoveAll(dir) })
	return tmpFn
}


func TestStringEnumValidDefinitionAndInitialization(t *testing.T) {
	input := `
package main

type Status string
const (
	Active Status = "active"
	Pending Status = "pending"
	Done Status = "done"
)

var s1 Status = Active
var s2 Status = "pending"
// var s3 Status = Done // Save for inspect test

func main() {
	if s1 != Active {
		panic("s1 should be Active")
	}
	if s2 != Pending {
		panic("s2 should be Pending")
	}

	// Check types of constants
	// This requires a way to inspect the type from minigo, or check behavior
	// For now, functional tests imply correct type.

	// Test direct assignment of valid string
	var sAssign Status
	sAssign = "done"
	if sAssign != Done {
		panic("sAssign should be Done after assigning \"done\"")
	}
}
`
	i := NewInterpreter()
	err := i.LoadAndRun(context.Background(), "test_enum_valid.mgo", "main")
	if err != nil {
		t.Fatalf("Interpreter run failed: %v", err)
	}
	// Further checks might involve inspecting global environment if possible/needed
}

func TestStringEnumInvalidInitialization(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "assign undeclared string to enum var",
			input: `
package main
type Color string
const (
	Red Color = "red"
)
var c Color = "blue" // Error: "blue" is not a valid Color
func main() {}
`,
			wantErr: `string value "blue" is not allowed for enum type 'Color'`,
		},
		{
			name: "assign undeclared string to enum var direct",
			input: `
package main
type Direction string
const (
	North Direction = "north"
)
func main() {
	var d Direction = "south" // Error
}
`,
			wantErr: `string value "south" is not allowed for enum type 'Direction'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := NewInterpreter()
			err := i.LoadAndRun(context.Background(), "test_enum_invalid_init.mgo", "main")
			if err == nil {
				t.Fatalf("Expected error but got none")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Expected error message to contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}


func TestStringEnumInvalidAssignment(t *testing.T) {
	input := `
package main
type Priority string
const (
	High Priority = "high"
	Low Priority = "low"
)
func main() {
	var p Priority = High
	p = "medium" // Error: "medium" is not valid for Priority
}
`
	i := NewInterpreter()
	err := i.LoadAndRun(context.Background(), "test_enum_invalid_assign.mgo", "main")
	if err == nil {
		t.Fatalf("Expected error but got none for invalid assignment")
	}
	expectedErr := `value "medium" is not allowed for enum type 'Priority'`
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("Expected error message to contain %q, got %q", expectedErr, err.Error())
	}
}


func TestStringEnumComparisons(t *testing.T) {
	input := `
package main

type Enum1 string
const (
	ValA Enum1 = "A"
	ValB Enum1 = "B"
)

type Enum2 string // Different enum type
const (
	ValX Enum2 = "A" // Same underlying string value as ValA but different type
)

func main() {
	if !(ValA == ValA) { panic("ValA == ValA should be true") }
	if ValA == ValB { panic("ValA == ValB should be false") }
	if !(ValA != ValB) { panic("ValA != ValB should be true") }

	var e1a Enum1 = ValA
	var e1b Enum1 = "B" // Valid assignment

	if !(e1a == ValA) { panic("e1a == ValA should be true") }
	if e1a == e1b { panic("e1a == e1b should be false") }

	// ValA == ValX // This should cause a type error at comparison
}
`
	i := NewInterpreter()
	err := i.LoadAndRun(context.Background(), "test_enum_compare.mgo", "main")
	if err != nil {
		t.Fatalf("Interpreter run failed for valid comparisons: %v", err)
	}
}

func TestStringEnumComparisonTypeMismatch(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "compare enum with raw string",
			input: `
package main
type MyEnum string
const MyVal MyEnum = "val"
func main() {
	_ = MyVal == "val" // Error
}`,
			wantErr: "type mismatch or unsupported operation for binary expression: CONSTRAINED_STRING_INSTANCE == STRING",
		},
		{
			name: "compare different enum types",
			input: `
package main
type EnumA string
const ValA EnumA = "a"
type EnumB string
const ValB EnumB = "a"
func main() {
	_ = ValA == ValB // Error
}`,
			// The error comes from evalConstrainedStringBinaryExpr
			wantErr: "type mismatch: cannot compare instances of different enum types (EnumA vs EnumB)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := NewInterpreter()
			err := i.LoadAndRun(context.Background(), "test_enum_compare_mismatch.mgo", "main")
			if err == nil {
				t.Fatalf("Expected error but got none")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Expected error message to contain %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

// TODO:
// - Test inspecting a ConstrainedStringInstance (e.g. fmt.Sprintf("%v", StatusActive) )
// - Test inspecting a ConstrainedStringTypeDefinition (e.g. if it's possible to get it in minigo)
// - Test behavior of uninitialized enum variable (`var s Status; ...`)
// - Test assignment to an enum variable from a function returning a string (should it be allowed if string is valid?)
// - Test using enum as map key
// - Test enum from imported package
// - Test errors for non-string const initializers for string enums.
// - Test that `type MyInt int; const C MyInt = 0` does NOT become an enum.
// - Test that `type MyStr string; const S MyStr = "s"` (without other consts) is still an enum (CSD with 1 value).
//   This means any `const X MyStrType = "val"` where `MyStrType` is an alias to string will trigger promotion.
//   This might be too aggressive if user just wanted a typed string constant.
//   The plan said: "It will then *promote* or *convert* the definition of `Status` from a simple alias to a `ConstrainedStringTypeDefinition`"
//   This implies the first const of that type makes it an enum. This is probably fine.
package main

import (
	"context"
	"strings"
	"testing"
)

func TestStringEnumValidDefinitionAndInitialization(t *testing.T) {
	input := `
package main

type Status string
const (
	Active Status = "active"
	Pending Status = "pending"
	Done Status = "done"
)

var s1 Status = Active
var s2 Status = "pending"
// var s3 Status = Done // Save for inspect test

func main() {
	if s1 != Active {
		panic("s1 should be Active")
	}
	if s2 != Pending {
		panic("s2 should be Pending")
	}

	// Check types of constants
	// This requires a way to inspect the type from minigo, or check behavior
	// For now, functional tests imply correct type.

	// Test direct assignment of valid string
	var sAssign Status
	sAssign = "done"
	if sAssign != Done {
		panic("sAssign should be Done after assigning \"done\"")
	}
}
`
	filename := createTempMgoFile(t, "valid_def_init", input)
	// defer os.Remove(filename) // Cleanup is handled by t.Cleanup in createTempMgoFile

	i := NewInterpreter()
	err := i.LoadAndRun(context.Background(), filename, "main")
	if err != nil {
		t.Fatalf("Interpreter run failed for valid enum definition and initialization: %v\nScript:\n%s", err, input)
	}
	// Further checks might involve inspecting global environment if possible/needed
	// For now, if it runs without panic from the script, it's a good sign.
}

func TestStringEnumInvalidInitialization(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "assign undeclared string to enum var",
			input: `
package main
type Color string
const (
	Red Color = "red"
)
var c Color = "blue" // Error: "blue" is not a valid Color
func main() {}
`,
			wantErr: `string value "blue" is not allowed for enum type 'Color'`,
		},
		{
			name: "assign undeclared string to enum var direct in main",
			input: `
package main
type Direction string
const (
	North Direction = "north"
)
func main() {
	var d Direction = "south" // Error
}
`,
			wantErr: `string value "south" is not allowed for enum type 'Direction'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename := createTempMgoFile(t, "invalid_init_"+tt.name, tt.input)
			i := NewInterpreter()
			err := i.LoadAndRun(context.Background(), filename, "main")
			if err == nil {
				t.Fatalf("Expected error but got none for test '%s'. Script:\n%s", tt.name, tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("For test '%s', expected error message to contain %q, got %q. Script:\n%s", tt.name, tt.wantErr, err.Error(), tt.input)
			}
		})
	}
}


func TestStringEnumInvalidAssignment(t *testing.T) {
	input := `
package main
type Priority string
const (
	High Priority = "high"
	Low Priority = "low"
)
func main() {
	var p Priority = High
	p = "medium" // Error: "medium" is not valid for Priority
}
`
	filename := createTempMgoFile(t, "invalid_assign", input)
	i := NewInterpreter()
	err := i.LoadAndRun(context.Background(), filename, "main")
	if err == nil {
		t.Fatalf("Expected error but got none for invalid assignment. Script:\n%s", input)
	}
	expectedErr := `value "medium" is not allowed for enum type 'Priority'`
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("Expected error message to contain %q, got %q. Script:\n%s", expectedErr, err.Error(), input)
	}
}


func TestStringEnumComparisons(t *testing.T) {
	input := `
package main

type Enum1 string
const (
	ValA Enum1 = "A"
	ValB Enum1 = "B"
)

type Enum2 string // Different enum type
const (
	ValX Enum2 = "A" // Same underlying string value as ValA but different type
)

func main() {
	if !(ValA == ValA) { panic("ValA == ValA should be true") }
	if ValA == ValB { panic("ValA == ValB should be false") }
	if !(ValA != ValB) { panic("ValA != ValB should be true") }

	var e1a Enum1 = ValA
	var e1b Enum1 = "B" // Valid assignment

	if !(e1a == ValA) { panic("e1a == ValA should be true") }
	if e1a == e1b { panic("e1a == e1b should be false") }

	// ValA == ValX // This should cause a type error at comparison if uncommented and tested separately
}
`
	filename := createTempMgoFile(t, "compare_valid", input)
	i := NewInterpreter()
	err := i.LoadAndRun(context.Background(), filename, "main")
	if err != nil {
		t.Fatalf("Interpreter run failed for valid comparisons: %v. Script:\n%s", err, input)
	}
}

func TestStringEnumComparisonTypeMismatch(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "compare enum with raw string",
			input: `
package main
type MyEnum string
const MyVal MyEnum = "val"
func main() {
	_ = MyVal == "val" // Error
}`,
			wantErr: "type mismatch or unsupported operation for binary expression: CONSTRAINED_STRING_INSTANCE == STRING",
		},
		{
			name: "compare different enum types",
			input: `
package main
type EnumA string
const ValA EnumA = "a"
type EnumB string
const ValB EnumB = "a"
func main() {
	_ = ValA == ValB // Error
}`,
			// The error comes from evalConstrainedStringBinaryExpr
			wantErr: "type mismatch: cannot compare instances of different enum types (EnumA vs EnumB)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename := createTempMgoFile(t, "compare_mismatch_"+tt.name, tt.input)
			i := NewInterpreter()
			err := i.LoadAndRun(context.Background(), filename, "main")
			if err == nil {
				t.Fatalf("Expected error but got none for test '%s'. Script:\n%s", tt.name, tt.input)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("For test '%s', expected error message to contain %q, got %q. Script:\n%s", tt.name, tt.wantErr, err.Error(), tt.input)
			}
		})
	}
}

// TODO:
// - Test inspecting a ConstrainedStringInstance (e.g. fmt.Sprintf("%v", StatusActive) )
// - Test inspecting a ConstrainedStringTypeDefinition (e.g. if it's possible to get it in minigo)
// - Test behavior of uninitialized enum variable (`var s Status; ...`)
// - Test assignment to an enum variable from a function returning a string (should it be allowed if string is valid?)
// - Test using enum as map key
// - Test enum from imported package
// - Test errors for non-string const initializers for string enums.
// - Test that `type MyInt int; const C MyInt = 0` does NOT become an enum.
// - Test that `type MyStr string; const S MyStr = "s"` (without other consts) is still an enum (CSD with 1 value).
//   This means any `const X MyStrType = "val"` where `MyStrType` is an alias to string will trigger promotion.
//   This might be too aggressive if user just wanted a typed string constant.
//   The plan said: "It will then *promote* or *convert* the definition of `Status` from a simple alias to a `ConstrainedStringTypeDefinition`"
//   This implies the first const of that type makes it an enum. This is probably fine.
