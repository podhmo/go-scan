package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"testing"

	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

const sampleApiCode = `
package sampleapi

import (
	"encoding/json"
	"net/http"
)

// User represents a user in the system.
type User struct {
	ID   int    "json:\"id\""
	Name string "json:\"name\""
}

// listUsers handles the GET /users endpoint.
// It returns a list of all users.
func listUsers(w http.ResponseWriter, r *http.Request) {
	users := []User{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// RegisterHandlers registers all the handlers for this sample API.
func RegisterHandlers() {
	http.HandleFunc("/users", listUsers)
}
`

func TestIntegration_HttpDocgen(t *testing.T) {
	// Setup: Scanner with in-memory file overlay
	s := scantest.NewScanner(t,
		scantest.WithOverlay(map[string]string{
			"github.com/podhmo/go-scan/examples/docgen/sampleapi/api.go": sampleApiCode,
		}),
	)
	evaluator := symgo.New(s)
	ctx := context.Background()

	// Define the intrinsic for http.HandleFunc
	httpPkg, err := s.ScanPackageByImport(ctx, "net/http")
	if err != nil {
		t.Fatalf("could not load net/http package: %v", err)
	}
	handleFuncObj := s.LookupFunc(httpPkg, "HandleFunc")
	if handleFuncObj == nil {
		t.Fatalf("http.HandleFunc not found")
	}

	var foundPath string
	var foundHandler symgo.Function

	evaluator.Intrinsics.Register(handleFuncObj, func(eval *symgo.Evaluator, call *ast.CallExpr, scope *symgo.Scope) symgo.Object {
		if len(call.Args) != 2 {
			return object.NewError(fmt.Errorf("expected 2 arguments to http.HandleFunc, got %d", len(call.Args)))
		}
		pathValue := eval.EvalExpr(call.Args[0], scope)
		path, ok := pathValue.Value.(string)
		if !ok {
			return object.NewError(fmt.Errorf("expected string literal for path, got %T", pathValue.Value))
		}
		handlerIdent, ok := call.Args[1].(*ast.Ident)
		if !ok {
			return object.NewError(fmt.Errorf("expected identifier for handler, got %T", call.Args[1]))
		}
		handlerSym := scope.Lookup(handlerIdent.Name)
		if handlerSym == nil {
			return object.NewError(fmt.Errorf("could not resolve handler function %q", handlerIdent.Name))
		}
		handlerFunc, ok := handlerSym.Object.(symgo.Function)
		if !ok {
			return object.NewError(fmt.Errorf("handler %q is not a function", handlerIdent.Name))
		}

		// Store the found path and handler
		foundPath = path
		foundHandler = handlerFunc
		return object.Void
	})


	// Run analysis
	pkg, err := s.ScanPackageByImport(ctx, "github.com/podhmo/go-scan/examples/docgen/sampleapi")
	if err != nil {
		t.Fatalf("failed to load sample API package: %v", err)
	}
	registerHandlers := s.LookupFunc(pkg, "RegisterHandlers")
	if registerHandlers == nil {
		t.Fatalf("RegisterHandlers function not found in package")
	}
	if _, err := evaluator.Eval(ctx, registerHandlers); err != nil {
		t.Fatalf("failed to evaluate RegisterHandlers: %v", err)
	}

	// Assertions will be in the next step
	if foundPath != "/users" {
		t.Errorf("expected path to be /users, but got %q", foundPath)
	}
	if foundHandler == nil {
		t.Fatal("handler was not found")
	}
	if foundHandler.Name() != "listUsers" {
		t.Errorf("expected handler to be listUsers, but got %q", foundHandler.Name())
	}
}
