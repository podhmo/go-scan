package evaluator_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestUint64Evaluation(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedType object.ObjectType
		expectedVal  uint64
	}{
		{
			name:         "LargeHexLiteral",
			input:        "0xffffffffffffffff",
			expectedType: object.UNSIGNED_INTEGER_OBJ,
			expectedVal:  18446744073709551615,
		},
		{
			name:         "LargeDecimalLiteral",
			input:        "18446744073709551615",
			expectedType: object.UNSIGNED_INTEGER_OBJ,
			expectedVal:  18446744073709551615,
		},
		{
			name:         "MaxInt64PlusOne",
			input:        "9223372036854775808", // MaxInt64 is 9223372036854775807
			expectedType: object.UNSIGNED_INTEGER_OBJ,
			expectedVal:  9223372036854775808,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			symgotest.RunExpression(t, tt.input, func(t *testing.T, r *symgotest.Result) {
				uintResult, ok := r.ReturnValue.(*object.UnsignedInteger)
				if !ok {
					t.Fatalf("Expected UnsignedInteger, got %T (%s)", r.ReturnValue, r.ReturnValue.Inspect())
				}

				if uintResult.Type() != tt.expectedType {
					t.Errorf("Expected type %s, got %s", tt.expectedType, uintResult.Type())
				}
				if uintResult.Value != tt.expectedVal {
					t.Errorf("Expected value %d, got %d", tt.expectedVal, uintResult.Value)
				}
			})
		})
	}
}

func TestUint64Constant(t *testing.T) {
	source := map[string]string{
		"go.mod": "module example.com/main",
		"main.go": `
package main
const MaxUint64 = 18446744073709551615
const AlsoMaxUint64 = 0xffffffffffffffff
const AlmostMaxInt64 = 9223372036854775807

func main() {}
`,
	}

	tc := symgotest.TestCase{
		Source:     source,
		EntryPoint: "example.com/main.main",
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		pkgPath := "example.com/main"
		pkgEnv, ok := r.Interpreter.PackageEnvForTest(pkgPath)
		if !ok {
			t.Fatalf("could not find package env for %q", pkgPath)
		}

		// Check MaxUint64
		obj, ok := pkgEnv.Get("MaxUint64")
		if !ok {
			t.Fatal("Constant MaxUint64 not found in package environment")
		}
		uintConst, ok := obj.(*object.UnsignedInteger)
		if !ok {
			t.Fatalf("Expected MaxUint64 to be UnsignedInteger, got %T", obj)
		}
		if want := uint64(18446744073709551615); uintConst.Value != want {
			t.Errorf("Expected MaxUint64 to be %d, got %d", want, uintConst.Value)
		}

		// Check AlsoMaxUint64
		obj, ok = pkgEnv.Get("AlsoMaxUint64")
		if !ok {
			t.Fatal("Constant AlsoMaxUint64 not found in package environment")
		}
		uintConst, ok = obj.(*object.UnsignedInteger)
		if !ok {
			t.Fatalf("Expected AlsoMaxUint64 to be UnsignedInteger, got %T", obj)
		}
		if want := uint64(18446744073709551615); uintConst.Value != want {
			t.Errorf("Expected AlsoMaxUint64 to be %d, got %d", want, uintConst.Value)
		}

		// Check AlmostMaxInt64 (should be a regular Integer)
		obj, ok = pkgEnv.Get("AlmostMaxInt64")
		if !ok {
			t.Fatal("Constant AlmostMaxInt64 not found in package environment")
		}
		intConst, ok := obj.(*object.Integer)
		if !ok {
			t.Fatalf("Expected AlmostMaxInt64 to be Integer, got %T", obj)
		}
		if want := int64(9223372036854775807); intConst.Value != want {
			t.Errorf("Expected AlmostMaxInt64 to be %d, got %d", want, intConst.Value)
		}
	})
}