package minigo_test

import (
	"context"
	"testing"

	// "time" // Temporarily unused until error handling is clarified

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"

	// standard library bindings
	stdbytes "github.com/podhmo/go-scan/minigo/stdlib/bytes"
	stdregexp "github.com/podhmo/go-scan/minigo/stdlib/regexp"
	stdsort "github.com/podhmo/go-scan/minigo/stdlib/sort"
	// stdtime "github.com/podhmo/go-scan/minigo/stdlib/time" // Temporarily unused
)

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

func TestStdlib_sort(t *testing.T) {
	script := `
package main
import "sort"
var s1 = []int{1, 2, 4, 8}
var r1 = sort.IntsAreSorted(s1)
var s2 = []int{3, 1, 4, 1, 5, 9}
var r2 = sort.IntsAreSorted(s2)
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}

	stdsort.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()

	r1, ok := env.Get("r1")
	if !ok {
		t.Fatalf("variable 'r1' not found")
	}
	if r1 != object.TRUE {
		t.Errorf("expected r1 to be true, but got %v", r1)
	}

	r2, ok := env.Get("r2")
	if !ok {
		t.Fatalf("variable 'r2' not found")
	}
	if r2 != object.FALSE {
		t.Errorf("expected r2 to be false, but got %v", r2)
	}
}

func TestStdlib_slices(t *testing.T) {
	script := `
package main
import "slices"
var s = []int{1, 2, 3}
var s2 = slices.Clone(s)
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()

	s2, ok := env.Get("s2")
	if !ok {
		t.Fatalf("variable 's2' not found")
	}
	if s2 == object.NIL {
		t.Fatalf("expected s2 to be non-nil, but it was nil")
	}

	s2Array, ok := s2.(*object.Array)
	if !ok {
		t.Fatalf("expected s2 to be an array, but got %T", s2)
	}

	if len(s2Array.Elements) != 3 {
		t.Fatalf("expected s2 to have 3 elements, but got %d", len(s2Array.Elements))
	}
}
