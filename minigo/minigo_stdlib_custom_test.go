package minigo_test

import (
	"context"
	"testing"
	"time"

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"

	"path/filepath"

	// standard library bindings
	stdbytes "github.com/podhmo/go-scan/minigo/stdlib/bytes"
	stdjson "github.com/podhmo/go-scan/minigo/stdlib/encoding/json"
	stderrors "github.com/podhmo/go-scan/minigo/stdlib/errors"
	stdmathrand "github.com/podhmo/go-scan/minigo/stdlib/math/rand"
	stdpathfilepath "github.com/podhmo/go-scan/minigo/stdlib/path/filepath"
	stdregexp "github.com/podhmo/go-scan/minigo/stdlib/regexp"
	stdsort "github.com/podhmo/go-scan/minigo/stdlib/sort"
	stdtemplate "github.com/podhmo/go-scan/minigo/stdlib/text/template"
	stdtime "github.com/podhmo/go-scan/minigo/stdlib/time"
)

// TestStdlib_time_error_handling verifies that the FFI bridge correctly returns
// a Go error as a usable value in the minigo script, rather than panicking.
func TestStdlib_time_error_handling(t *testing.T) {
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


// TestStdlib_time_success_as unmarshals a time.Time object from a script result.
// It verifies that a successful time.Parse call in-script can be returned
// and correctly converted back to a Go time.Time object using the As() method.
func TestStdlib_time_success_as(t *testing.T) {
	script := `
package main

import "time"

// main returns the result of time.Parse as a tuple (time.Time, error).
// The Go test will unmarshal this tuple into a struct.
func main() (any, any) {
	layout := "2006-01-02T15:04:05Z"
	return time.Parse(layout, "2024-07-26T10:30:00Z")
}
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}
	stdtime.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("LoadFile() failed: %v", err)
	}

	result, err := interp.Eval(context.Background())
	if err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	// Define a Go struct to receive the (time.Time, error) tuple.
	type TimeParseResult struct {
		Time time.Time
		Err  error
	}

	var res TimeParseResult
	if err := result.As(&res); err != nil {
		t.Fatalf("As() failed: %v", err)
	}

	// Verify the results.
	if res.Err != nil {
		t.Errorf("expected nil error, but got: %v", res.Err)
	}

	expectedTime, _ := time.Parse("2006-01-02T15:04:05Z", "2024-07-26T10:30:00Z")
	if !res.Time.Equal(expectedTime) {
		t.Errorf("time mismatch:\ngot:  %v\nwant: %v", res.Time, expectedTime)
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

// TestStdlib_SortInts_FFI tests the `sort.Ints` function using the FFI bindings.
// Direct source interpretation failed due to limitations in constant evaluation
// in transitive dependencies (math/bits).
func TestStdlib_SortInts_FFI(t *testing.T) {
	script := `
package main
import "sort"
var s = []int{5, 2, 6, 3, 1, 4}
var _ = sort.Ints(s)

var f = []float64{3.3, 1.1, 4.4, 2.2}
var _ = sort.Float64s(f)
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}

	// Use the pre-generated FFI bindings as a fallback.
	stdsort.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}

	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()

	// Check the sorted integer slice
	sObj, ok := env.Get("s")
	if !ok {
		t.Fatalf("variable 's' not found")
	}
	sArr, ok := sObj.(*object.Array)
	if !ok {
		t.Fatalf("variable 's' is not an array, got %T", sObj)
	}
	expectedInts := []int64{1, 2, 3, 4, 5, 6}
	if len(sArr.Elements) != len(expectedInts) {
		t.Fatalf("sorted int slice has wrong length, got %d, want %d", len(sArr.Elements), len(expectedInts))
	}
	for i, el := range sArr.Elements {
		intVal, ok := el.(*object.Integer)
		if !ok {
			t.Fatalf("element %d is not an integer, got %T", i, el)
		}
		if intVal.Value != expectedInts[i] {
			t.Errorf("s[%d] is wrong, got %d, want %d", i, intVal.Value, expectedInts[i])
		}
	}

	// Check the sorted float slice
	fObj, ok := env.Get("f")
	if !ok {
		t.Fatalf("variable 'f' not found")
	}
	fArr, ok := fObj.(*object.Array)
	if !ok {
		t.Fatalf("variable 'f' is not an array, got %T", fObj)
	}
	expectedFloats := []float64{1.1, 2.2, 3.3, 4.4}
	if len(fArr.Elements) != len(expectedFloats) {
		t.Fatalf("sorted float slice has wrong length, got %d, want %d", len(fArr.Elements), len(expectedFloats))
	}
	for i, el := range fArr.Elements {
		floatVal, ok := el.(*object.Float)
		if !ok {
			t.Fatalf("element %d is not a float, got %T", i, el)
		}
		if floatVal.Value != expectedFloats[i] {
			t.Errorf("f[%d] is wrong, got %f, want %f", i, floatVal.Value, expectedFloats[i])
		}
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

func TestStdlib_regexp(t *testing.T) {
	script := `
package main
import "regexp"
var re, err1 = regexp.Compile("p([a-z]+)ch")
var matched = re.MatchString("peach")
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

	if got, _ := env.Get("matched"); got != object.TRUE {
		t.Errorf("expected 'matched' to be true, but got %v", got)
	}
}

func TestStdlib_TextTemplate(t *testing.T) {
	script := `
package main
import (
	"bytes"
	"text/template"
)
type Person struct {
	Name string
}
var data = Person{Name: "World"}
var tpl = template.New("test")
var _, err1 = tpl.Parse("Hello, {{.Name}}!")
var buf = bytes.NewBuffer(nil)
var err2 = tpl.Execute(buf, data)
var result = buf.String()
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}

	stdtemplate.Install(interp)
	stdbytes.Install(interp) // Need bytes.Buffer

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()

	// Check for errors during template processing
	if err1, _ := env.Get("err1"); err1 != object.NIL {
		t.Errorf("expected err1 to be nil, but got: %v", err1.Inspect())
	}
	if err2, _ := env.Get("err2"); err2 != object.NIL {
		t.Errorf("expected err2 to be nil, but got: %v", err2.Inspect())
	}

	// Check the final rendered string
	result, ok := env.Get("result")
	if !ok {
		t.Fatalf("variable 'result' not found")
	}
	str, ok := result.(*object.String)
	if !ok {
		t.Fatalf("result is not a string, got %T", result)
	}

	expected := "Hello, World!"
	if str.Value != expected {
		t.Errorf("unexpected result:\ngot:  %q\nwant: %q", str.Value, expected)
	}
}

func TestStdlib_MathRand(t *testing.T) {
	script := `
package main
import "math/rand"
var src = rand.NewSource(1)
var r = rand.New(src)
var n = r.Intn(100)
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}
	stdmathrand.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()
	val, ok := env.Get("n")
	if !ok {
		t.Fatalf("variable 'n' not found")
	}

	integer, ok := val.(*object.Integer)
	if !ok {
		t.Fatalf("n is not an Integer, got %T", val)
	}

	// With rand.Seed(1), the first call to Intn(100) returns 81.
	expected := int64(81)
	if integer.Value != expected {
		t.Errorf("expected n to be %d, but got %d", expected, integer.Value)
	}
}

// TestStdlib_PathFilepath_FFI tests the `path/filepath` package using the
// pre-generated FFI bindings. This is used as a fallback because direct source
// interpretation resulted in a compile error in the test code.
func TestStdlib_PathFilepath_FFI(t *testing.T) {
	script := `
package main
import "path/filepath"
var joined = filepath.Join("a", "b", "c")
var base = filepath.Base(joined)
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}

	stdpathfilepath.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()

	// Check joined path
	joinedObj, ok := env.Get("joined")
	if !ok {
		t.Fatalf("variable 'joined' not found")
	}
	joinedStr, ok := joinedObj.(*object.String)
	if !ok {
		t.Fatalf("variable 'joined' is not a string, got %T", joinedObj)
	}

	// Use filepath.Join from the host Go's standard library to get the expected
	// OS-specific path. This makes the test robust across platforms.
	expectedJoined := filepath.Join("a", "b", "c")
	if joinedStr.Value != expectedJoined {
		t.Errorf("unexpected joined path: got %q, want %q", joinedStr.Value, expectedJoined)
	}

	// Check base name
	baseObj, ok := env.Get("base")
	if !ok {
		t.Fatalf("variable 'base' not found")
	}
	baseStr, ok := baseObj.(*object.String)
	if !ok {
		t.Fatalf("variable 'base' is not a string, got %T", baseObj)
	}
	expectedBase := "c"
	if baseStr.Value != expectedBase {
		t.Errorf("unexpected base name: got %q, want %q", baseStr.Value, expectedBase)
	}
}

// TestStdlib_Errors_FFI tests the `errors` package using the pre-generated
// FFI bindings. This is the fallback test after direct source interpretation
// was found to be incompatible.
func TestStdlib_Errors_FFI(t *testing.T) {
	script := `
package main
import "errors"
var err = errors.New("a new error")
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}

	stderrors.Install(interp)

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
		t.Fatalf("expected err to be a non-nil error object, but it was nil")
	}

	// We can check if the returned object is a GoValue wrapping an error.
	goVal, ok := errObj.(*object.GoValue)
	if !ok {
		t.Fatalf("expected 'err' to be a GoValue, but got %T", errObj)
	}

	// And we can check the error string.
	nativeErr, ok := goVal.Value.Interface().(error)
	if !ok {
		t.Fatalf("GoValue does not wrap an error, but %T", goVal.Value.Interface())
	}

	expectedMsg := "a new error"
	if nativeErr.Error() != expectedMsg {
		t.Errorf("unexpected error message: got %q, want %q", nativeErr.Error(), expectedMsg)
	}
}

// TestStdlib_EncodingJson_FFI tests the `encoding/json` package using FFI bindings.
func TestStdlib_EncodingJson_FFI(t *testing.T) {
	script := `
package main
import "encoding/json"

type Point struct { X int; Y int }
var p = Point{X: 1, Y: 2}
var data, err = json.Marshal(p)
var result = string(data)
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}

	stdjson.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()

	if errObj, _ := env.Get("err"); errObj != object.NIL {
		t.Fatalf("json.Marshal returned an unexpected error: %v", errObj.Inspect())
	}

	resultObj, ok := env.Get("result")
	if !ok {
		t.Fatal("variable 'result' not found")
	}
	resultStr, ok := resultObj.(*object.String)
	if !ok {
		t.Fatalf("result is not a string, got %T", resultObj)
	}

	// NOTE: The order of fields in JSON is not guaranteed.
	expected1 := `{"X":1,"Y":2}`
	expected2 := `{"Y":2,"X":1}`
	if resultStr.Value != expected1 && resultStr.Value != expected2 {
		t.Errorf("unexpected json output: got %q, want %q or %q", resultStr.Value, expected1, expected2)
	}
}
