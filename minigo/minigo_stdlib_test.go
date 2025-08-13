package minigo_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
	stdfmt "github.com/podhmo/go-scan/minigo/stdlib/fmt"
	stdstrings "github.com/podhmo/go-scan/minigo/stdlib/strings"
)

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
