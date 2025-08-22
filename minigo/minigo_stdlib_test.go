package minigo_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/minigo/object"
	stdjson "github.com/podhmo/go-scan/minigo/stdlib/encoding/json"
	stdfmt "github.com/podhmo/go-scan/minigo/stdlib/fmt"
	stdstrconv "github.com/podhmo/go-scan/minigo/stdlib/strconv"
	stdstrings "github.com/podhmo/go-scan/minigo/stdlib/strings"
)

func TestStdlib_json(t *testing.T) {
	script := `
package main
import "encoding/json"
type Point struct {
	X int
	Y int
}
var p1 = Point{X: 10, Y: 20}
var data, err1 = json.Marshal(p1)
var p2 Point
var err2 = json.Unmarshal(data, &p2)
`
	interp := newTestInterpreter(t)
	stdjson.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()
	{
		got, ok := env.Get("err1")
		if !ok {
			t.Fatalf("variable 'err1' not found")
		}
		if got != object.NIL {
			t.Errorf("variable 'err1' is not nil, but %#v", got)
		}
	}
	{
		got, ok := env.Get("err2")
		if !ok {
			t.Fatalf("variable 'err2' not found")
		}
		if got != object.NIL {
			t.Errorf("variable 'err2' is not nil, but %#v", got)
		}
	}
	{
		want := `{"X":10,"Y":20}`
		got, ok := env.Get("data")
		if !ok {
			t.Fatalf("variable 'data' not found")
		}
		// data is []byte, which is an array of integers in minigo
		gotSlice, ok := got.(*object.Array)
		if !ok {
			t.Fatalf("variable 'data' is not a array, but %T", got)
		}
		bytes := make([]byte, len(gotSlice.Elements))
		for i, el := range gotSlice.Elements {
			intVal, ok := el.(*object.Integer)
			if !ok {
				t.Fatalf("element in slice is not an integer, but %T", el)
			}
			bytes[i] = byte(intVal.Value)
		}
		if diff := cmp.Diff(want, string(bytes)); diff != "" {
			t.Errorf("mismatched data (-want +got):\n%s", diff)
		}
	}

	// check p2
	{
		p2, ok := env.Get("p2")
		if !ok {
			t.Fatalf("variable 'p2' not found")
		}
		p2Struct, ok := p2.(*object.StructInstance)
		if !ok {
			t.Fatalf("variable 'p2' is not a struct, but %T", p2)
		}

		// check p2.X
		xVal, ok := p2Struct.Fields["X"]
		if !ok {
			t.Fatalf("field 'X' not found in p2")
		}
		xInt, ok := xVal.(*object.Integer)
		if !ok {
			t.Fatalf("field 'X' is not an integer, but %T", xVal)
		}
		if xInt.Value != 10 {
			t.Errorf("p2.X is not 10, got %d", xInt.Value)
		}

		// check p2.Y
		yVal, ok := p2Struct.Fields["Y"]
		if !ok {
			t.Fatalf("field 'Y' not found in p2")
		}
		yInt, ok := yVal.(*object.Integer)
		if !ok {
			t.Fatalf("field 'Y' is not an integer, but %T", yVal)
		}
		if yInt.Value != 20 {
			t.Errorf("p2.Y is not 20, got %d", yInt.Value)
		}
	}
}

func TestStdlib_json_with_tags(t *testing.T) {
	script := "package main\n" +
		`import "encoding/json"` + "\n" +
		"type Point struct {\n" +
		`	X int ` + "`json:\"x_coord\"`" + "\n" +
		`	Y int ` + "`json:\"y_coord,omitempty\"`" + "\n" +
		`	Z int ` + "`json:\"-\"`" + "\n" +
		"}\n" +
		"var p1 = Point{X: 10, Y: 0, Z: 30}\n" +
		"var data, err = json.Marshal(p1)\n"

	interp := newTestInterpreter(t)
	stdjson.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()
	{
		got, ok := env.Get("err")
		if !ok {
			t.Fatalf("variable 'err' not found")
		}
		if got != object.NIL {
			t.Errorf("variable 'err' is not nil, but %#v", got)
		}
	}
	{
		want := `{"x_coord":10}` // Y should be omitted, Z should be ignored
		got, ok := env.Get("data")
		if !ok {
			t.Fatalf("variable 'data' not found")
		}
		gotSlice, ok := got.(*object.Array)
		if !ok {
			t.Fatalf("variable 'data' is not a array, but %T", got)
		}
		bytes := make([]byte, len(gotSlice.Elements))
		for i, el := range gotSlice.Elements {
			intVal, ok := el.(*object.Integer)
			if !ok {
				t.Fatalf("element in slice is not an integer, but %T", el)
			}
			bytes[i] = byte(intVal.Value)
		}
		if diff := cmp.Diff(want, string(bytes)); diff != "" {
			t.Errorf("mismatched data (-want +got):\n%s", diff)
		}
	}
}

func TestStdlib_strconv(t *testing.T) {
	script := `
package main
import (
	"strconv"
)
var i, err_ok = strconv.Atoi("123")
var s = strconv.Itoa(456)
var r_atoi_ng, err_ng = strconv.Atoi("abc")
var r_fb_t = strconv.FormatBool(true)
var r_fb_f = strconv.FormatBool(false)
`
	interp := newTestInterpreter(t)
	stdstrconv.Install(interp)
	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()
	{
		want := int64(123)
		got, ok := env.Get("i")
		if !ok {
			t.Fatalf("variable 'i' not found")
		}
		gotInt, ok := got.(*object.Integer)
		if !ok {
			t.Fatalf("variable 'i' is not an integer, but %T", got)
		}
		if diff := cmp.Diff(want, gotInt.Value); diff != "" {
			t.Errorf("mismatched i (-want +got):\n%s", diff)
		}
	}
	{
		got, ok := env.Get("err_ok")
		if !ok {
			t.Fatalf("variable 'err_ok' not found")
		}
		if got != object.NIL {
			t.Errorf("variable 'err_ok' is not nil, but %T", got)
		}
	}
	{
		want := "456"
		got, ok := env.Get("s")
		if !ok {
			t.Fatalf("variable 's' not found")
		}
		gotStr, ok := got.(*object.String)
		if !ok {
			t.Fatalf("variable 's' is not a string, but %T", got)
		}
		if diff := cmp.Diff(want, gotStr.Value); diff != "" {
			t.Errorf("mismatched s (-want +got):\n%s", diff)
		}
	}

	// Check error case from Atoi("abc")
	{
		got, ok := env.Get("r_atoi_ng")
		if !ok {
			t.Fatalf("variable 'r_atoi_ng' not found")
		}
		if got.(*object.Integer).Value != 0 {
			t.Errorf("expected 'r_atoi_ng' to be 0 on error, got %d", got.(*object.Integer).Value)
		}
		err_ng, ok := env.Get("err_ng")
		if !ok || err_ng == object.NIL {
			t.Fatalf("expected 'err_ng' to be a non-nil error")
		}
		if _, ok := err_ng.(*object.GoValue); !ok {
			t.Errorf("expected error to be a GoValue, got %T", err_ng)
		}
	}

	// Check FormatBool
	{
		got, _ := env.Get("r_fb_t")
		if got.Type() != object.STRING_OBJ {
			t.Errorf("expected 'r_fb_t' to be a STRING, but got %s", got.Type())
		} else if got.Inspect() != "true" {
			t.Errorf("expected 'r_fb_t' to have value \"true\", got %q", got.Inspect())
		}

		got, _ = env.Get("r_fb_f")
		if got.Type() != object.STRING_OBJ {
			t.Errorf("expected 'r_fb_f' to be a STRING, but got %s", got.Type())
		} else if got.Inspect() != "false" {
			t.Errorf("expected 'r_fb_f' to have value \"false\", got %q", got.Inspect())
		}
	}
}

func TestStdlib(t *testing.T) {
	script := `
package main

import (
	"fmt"
	"strings"
)

var s = "  hello world  "
var trimmed = strings.TrimSpace(s)
var upper = strings.ToUpper(trimmed)
var message = fmt.Sprintf("Message: %s", upper)
`

	interp := newTestInterpreter(t)

	// Install the generated bindings
	stdstrings.Install(interp)
	stdfmt.Install(interp)

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}

	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()
	want := "Message: HELLO WORLD"
	got, ok := env.Get("message")
	if !ok {
		t.Fatalf("variable 'message' not found in global scope")
	}

	gotStr, ok := got.(*object.String)
	if !ok {
		t.Fatalf("variable 'message' is not a string, but %T", got)
	}

	if diff := cmp.Diff(want, gotStr.Value); diff != "" {
		t.Errorf("mismatched message (-want +got):\n%s", diff)
	}
}

func TestStdlib_json_nested(t *testing.T) {
	jsonData := `{"Name":"parent","Child":{"Value":"child"}}`
	script := `
package main
import "encoding/json"
type Inner struct {
	Value string
}
type Outer struct {
	Name  string
	Child Inner
}
var result Outer
var err = json.Unmarshal(data, &result)
`
	interp := newTestInterpreter(t)
	stdjson.Install(interp)

	// Inject the data variable
	dataBytes := []byte(jsonData)
	elements := make([]object.Object, len(dataBytes))
	for i, b := range dataBytes {
		elements[i] = &object.Integer{Value: int64(b)}
	}
	interp.GlobalEnvForTest().Set("data", &object.Array{Elements: elements})

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()
	if err, _ := env.Get("err"); err != object.NIL {
		t.Fatalf("expected err to be nil, but got: %#v", err)
	}

	result, _ := env.Get("result")
	outerStruct, ok := result.(*object.StructInstance)
	if !ok {
		t.Fatalf("result is not a struct instance: %T", result)
	}

	if name, _ := outerStruct.Fields["Name"].(*object.String); name.Value != "parent" {
		t.Errorf("expected Name to be 'parent', got %q", name.Value)
	}

	child, ok := outerStruct.Fields["Child"].(*object.StructInstance)
	if !ok {
		t.Fatalf("Child is not a struct instance: %T", outerStruct.Fields["Child"])
	}
	if val, _ := child.Fields["Value"].(*object.String); val.Value != "child" {
		t.Errorf("expected Child.Value to be 'child', got %q", val.Value)
	}
}

func TestStdlib_json_recursive(t *testing.T) {
	jsonData := `{"Name":"Worker", "Manager": {"Name":"Boss", "Manager":null}}`
	script := `
package main
import "encoding/json"
type Employee struct {
	Name    string
	Manager *Employee
}
var result Employee
var err = json.Unmarshal(data, &result)
`
	interp := newTestInterpreter(t)
	stdjson.Install(interp)

	// Inject the data variable
	dataBytes := []byte(jsonData)
	elements := make([]object.Object, len(dataBytes))
	for i, b := range dataBytes {
		elements[i] = &object.Integer{Value: int64(b)}
	}
	interp.GlobalEnvForTest().Set("data", &object.Array{Elements: elements})

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()
	if err, _ := env.Get("err"); err != object.NIL {
		t.Fatalf("expected err to be nil, but got: %#v", err)
	}

	result, _ := env.Get("result")
	worker, ok := result.(*object.StructInstance)
	if !ok {
		t.Fatalf("result is not a struct instance: %T", result)
	}

	if name, _ := worker.Fields["Name"].(*object.String); name.Value != "Worker" {
		t.Errorf("expected Name to be 'Worker', got %q", name.Value)
	}

	managerPtr, ok := worker.Fields["Manager"].(*object.Pointer)
	if !ok {
		t.Fatalf("Manager is not a pointer: %T", worker.Fields["Manager"])
	}
	boss, ok := (*managerPtr.Element).(*object.StructInstance)
	if !ok {
		t.Fatalf("Manager pointer does not point to a struct instance: %T", *managerPtr.Element)
	}
	if name, _ := boss.Fields["Name"].(*object.String); name.Value != "Boss" {
		t.Errorf("expected Manager.Name to be 'Boss', got %q", name.Value)
	}

	if boss.Fields["Manager"] != object.NIL {
		t.Errorf("expected Manager.Manager to be nil, but got: %#v", boss.Fields["Manager"])
	}
}

func TestStdlib_json_crosspackage(t *testing.T) {
	jsonData := `{"Name":"John Doe","Age":42}`
	script := `
package main
import "encoding/json"
import "github.com/podhmo/go-scan/minigo/testdata/jsonstructs"
var result jsonstructs.Person
var err = json.Unmarshal(data, &result)
`
	interp := newTestInterpreter(t)
	stdjson.Install(interp)

	// Inject the data variable
	dataBytes := []byte(jsonData)
	elements := make([]object.Object, len(dataBytes))
	for i, b := range dataBytes {
		elements[i] = &object.Integer{Value: int64(b)}
	}
	interp.GlobalEnvForTest().Set("data", &object.Array{Elements: elements})

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

	env := interp.GlobalEnvForTest()
	if err, _ := env.Get("err"); err != object.NIL {
		t.Fatalf("expected err to be nil, but got: %#v", err)
	}

	result, _ := env.Get("result")
	person, ok := result.(*object.StructInstance)
	if !ok {
		t.Fatalf("result is not a struct instance: %T", result)
	}

	if name, _ := person.Fields["Name"].(*object.String); name.Value != "John Doe" {
		t.Errorf("expected Name to be 'John Doe', got %q", name.Value)
	}
	if age, _ := person.Fields["Age"].(*object.Integer); age.Value != 42 {
		t.Errorf("expected Age to be 42, got %d", age.Value)
	}
}
