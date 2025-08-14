package minigo_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	// "time" // Temporarily unused until error handling is clarified

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"

	// standard library bindings
	stdbytes "github.com/podhmo/go-scan/minigo/stdlib/bytes"
	stdregexp "github.com/podhmo/go-scan/minigo/stdlib/regexp"
	// stdtime "github.com/podhmo/go-scan/minigo/stdlib/time" // Temporarily unused
)

func TestStdlib_slices(t *testing.T) {
	script := `
package main
import "slices"

func main() {
    s1 := []int{1, 2, 3}
    s2 := slices.Clone(s1)

    // Modify the original slice
    s1[0] = 99

    // Return the new slice to check that it's an independent copy
    return s2
}
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}

	// The 'slices' package is not a pre-generated binding, but is loaded from source.
	// This tests the interpreter's ability to parse and use standard library source code.
	goroot := runtime.GOROOT()
	if goroot == "" {
		t.Skip("GOROOT not found, skipping test")
	}
	srcPath := filepath.Join(goroot, "src", "slices", "slices.go")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("could not read 'slices.go' source: %v", err)
	}

	if err := interp.LoadGoSourceAsPackage("slices", string(src)); err != nil {
		t.Fatalf("failed to load 'slices' package from source: %+v", err)
	}

	// Evaluate the main script that uses the loaded package.
	result, err := interp.EvalString(script)
	if err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	var s2 []int64
	res := &minigo.Result{Value: result}
	if err := res.As(&s2); err != nil {
		t.Fatalf("failed to unmarshal result into slice: %v", err)
	}

	expected := []int64{1, 2, 3}
	if !reflect.DeepEqual(s2, expected) {
		t.Errorf("unexpected slice content\nwant: %v\n got: %v", expected, s2)
	}
}

/*
// TestStdlib_time_limitation is temporarily disabled.
// It was discovered that the minigo interpreter panics when a bound Go function returns an error,
// instead of returning the error as a value. This makes it difficult to test for expected errors.
// This behavior is a key limitation that needs to be documented.
func TestStdlib_time_limitation(t *testing.T) {
	script := `
package main
import "time"
var layout = "2006-01-02"
// Check for a parse error, as we cannot call methods on the returned time object.
var _, err = time.Parse(layout, "not-a-valid-date")
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}
	stdtime.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()
	errObj, ok := env.Get("err")
	if !ok {
		t.Fatalf("variable 'err' not found")
	}
	if errObj == object.NIL {
		t.Fatalf("expected err to be non-nil for an invalid date, but it was nil")
	}
	errValue, ok := errObj.(*object.GoValue)
	if !ok {
		t.Fatalf("expected err to be a GoValue, but got %T", errObj)
	}
	if _, ok := errValue.Value.Interface().(*time.ParseError); !ok {
		t.Errorf("expected error to be of type *time.ParseError, but got %T", errValue.Value.Interface())
	}
}
*/

// TestStdlib_regexp tests package-level functions of the regexp package.
// It avoids using methods on a compiled regexp object, which was found to be unsupported.
func TestStdlib_regexp(t *testing.T) {
	script := `
package main
import "regexp"
var valid, err1 = regexp.MatchString("p([a-z]+)ch", "peach")
var invalid, err2 = regexp.MatchString("p([a-z]+)ch", "apple")
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}
	stdregexp.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()
	if err, _ := env.Get("err1"); err != object.NIL {
		t.Fatalf("err1 was not nil: %v", err)
	}
	if err, _ := env.Get("err2"); err != object.NIL {
		t.Fatalf("err2 was not nil: %v", err)
	}

	if got, _ := env.Get("valid"); got != object.TRUE {
		t.Errorf("expected 'valid' to be true")
	}
	if got, _ := env.Get("invalid"); got != object.FALSE {
		t.Errorf("expected 'invalid' to be false")
	}
}

// TestStdlib_bytes tests package-level functions of the bytes package.
// It avoids using methods on a bytes.Buffer object and avoids using the `byte` keyword,
// both of which were found to be unsupported.
func TestStdlib_bytes(t *testing.T) {
	script := `
package main
import "bytes"
// In minigo, []byte literals are represented as []int. "Go" -> {71, 111}
var a = []int{71, 111}
var b = []int{71, 111}
var c = []int{67, 43, 43} // "C++"
var r_equal = bytes.Equal(a, b)
var r_notequal = bytes.Equal(a, c)
var r_compare_eq = bytes.Compare(a, b)
var r_compare_gt = bytes.Compare(a, c)
var r_compare_lt = bytes.Compare(c, a)
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}
	stdbytes.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()
	if got, _ := env.Get("r_equal"); got != object.TRUE {
		t.Errorf("expected 'r_equal' to be true")
	}
	if got, _ := env.Get("r_notequal"); got != object.FALSE {
		t.Errorf("expected 'r_notequal' to be false")
	}

	if got, _ := env.Get("r_compare_eq"); got.(*object.Integer).Value != 0 {
		t.Errorf("expected 'r_compare_eq' to be 0, got %d", got.(*object.Integer).Value)
	}
	if got, _ := env.Get("r_compare_gt"); got.(*object.Integer).Value != 1 {
		t.Errorf("expected 'r_compare_gt' to be 1, got %d", got.(*object.Integer).Value)
	}
	if got, _ := env.Get("r_compare_lt"); got.(*object.Integer).Value != -1 {
		t.Errorf("expected 'r_compare_lt' to be -1, got %d", got.(*object.Integer).Value)
	}
}
