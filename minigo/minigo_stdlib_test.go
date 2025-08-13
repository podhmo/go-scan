package minigo_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
	stdjson "github.com/podhmo/go-scan/minigo/stdlib/encoding/json"
	stdfmt "github.com/podhmo/go-scan/minigo/stdlib/fmt"
	stdstrconv "github.com/podhmo/go-scan/minigo/stdlib/strconv"
	stdstrings "github.com/podhmo/go-scan/minigo/stdlib/strings"
)

func TestStdlib_json(t *testing.T) {
	// t.Skip("skipping json test for now, as it is expected to fail due to runtime limitations")
	script := `
package main
import "encoding/json"
type Point struct {
	X int
	Y int
}
var p1 = Point{X: 10, Y: 20}
var data, err1 = json.Marshal(p1)
// var p2 Point
// var err2 = json.Unmarshal(data, &p2)
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
	{
		got, ok := env.Get("err1")
		if !ok {
			t.Fatalf("variable 'err1' not found")
		}
		if got != object.NIL {
			t.Errorf("variable 'err1' is not nil, but %#v", got)
		}
	}
	// {
	// 	got, ok := env.Get("err2")
	// 	if !ok {
	// 		t.Fatalf("variable 'err2' not found")
	// 	}
	// 	if got != object.NIL {
	// 		t.Errorf("variable 'err2' is not nil, but %#v", got)
	// 	}
	// }
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

	// // check p2
	// {
	// 	p2, ok := env.Get("p2")
	// 	if !ok {
	// 		t.Fatalf("variable 'p2' not found")
	// 	}
	// 	p2Struct, ok := p2.(*object.StructInstance)
	// 	if !ok {
	// 		t.Fatalf("variable 'p2' is not a struct, but %T", p2)
	// 	}

	// 	// check p2.X
	// 	xVal, ok := p2Struct.Fields["X"]
	// 	if !ok {
	// 		t.Fatalf("field 'X' not found in p2")
	// 	}
	// 	xInt, ok := xVal.(*object.Integer)
	// 	if !ok {
	// 		t.Fatalf("field 'X' is not an integer, but %T", xVal)
	// 	}
	// 	if xInt.Value != 10 {
	// 		t.Errorf("p2.X is not 10, got %d", xInt.Value)
	// 	}

	// 	// check p2.Y
	// 	yVal, ok := p2Struct.Fields["Y"]
	// 	if !ok {
	// 		t.Fatalf("field 'Y' not found in p2")
	// 	}
	// 	yInt, ok := yVal.(*object.Integer)
	// 	if !ok {
	// 		t.Fatalf("field 'Y' is not an integer, but %T", yVal)
	// 	}
	// 	if yInt.Value != 20 {
	// 		t.Errorf("p2.Y is not 20, got %d", yInt.Value)
	// 	}
	// }
}

func TestStdlib_strconv(t *testing.T) {
	script := `
package main
import (
	"strconv"
)
var i, err = strconv.Atoi("123")
var s = strconv.Itoa(456)
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}
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
		got, ok := env.Get("err")
		if !ok {
			t.Fatalf("variable 'err' not found")
		}
		if got != object.NIL {
			t.Errorf("variable 'err' is not nil, but %T", got)
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

	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}

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
