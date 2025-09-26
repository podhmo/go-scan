package symgo_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestSymgo_CrossPackageCallRepresentation(t *testing.T) {
	source := map[string]string{
		"go.mod": "module example.com/app\ngo 1.22\n\nrequire example.com/helper v0.0.0\n\nreplace example.com/helper => ../helper\n",
		"main.go": `
package main

import (
	"example.com/helper"
)

func main() {
	helper.Gopher()
}
`,
		"../helper/go.mod": "module example.com/helper\ngo 1.22\n",
		"../helper/greet.go": `
package helper

// Gopher is a function in another package.
func Gopher() string {
	return "gopher"
}
`,
	}

	// This map will store the fully qualified names of all functions called.
	calledFunctions := make(map[string]bool)
	var mu sync.Mutex

	// This intrinsic will be triggered for every function call.
	// It's our way of building a call graph.
	defaultIntrinsic := func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		if len(args) == 0 {
			return nil
		}
		fn := args[0]
		var key string

		switch f := fn.(type) {
		case *object.Function:
			if f.Package != nil && f.Name != nil {
				key = fmt.Sprintf("%s.%s", f.Package.ImportPath, f.Name.Name)
			}
		case *object.UnresolvedFunction:
			key = fmt.Sprintf("%s.%s", f.PkgPath, f.FuncName)
		// The object representing an external function should be an UnresolvedFunction,
		// not an UnresolvedType. By removing the case for UnresolvedType, we assert
		// that the representation is correct.
		case *object.SymbolicPlaceholder:
			// This case might be triggered for interface methods.
			if f.UnderlyingFunc != nil && f.Package != nil {
				key = fmt.Sprintf("%s.%s", f.Package.ImportPath, f.UnderlyingFunc.Name)
			}
		}

		if key != "" {
			mu.Lock()
			calledFunctions[key] = true
			mu.Unlock()
		}
		return nil
	}

	tc := symgotest.TestCase{
		WorkDir:    ".",
		Source:     source,
		EntryPoint: "example.com/app.main",
		Options: []symgotest.Option{
			symgotest.WithDefaultIntrinsic(defaultIntrinsic),
			// We explicitly DO NOT include "example.com/helper" in the scan policy
			// to ensure it's treated as an external package.
			symgotest.WithScanPolicy(func(path string) bool {
				return strings.HasPrefix(path, "example.com/app")
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symgotest.Run failed: %+v", r.Error)
		}

		mu.Lock()
		defer mu.Unlock()

		// The main assertion: was the external function call recorded?
		// According to the TODO, this is currently being omitted.
		expectedCall := "example.com/helper.Gopher"
		if !calledFunctions[expectedCall] {
			t.Errorf("expected call to %q to be represented in the call graph, but it was not found", expectedCall)
			t.Logf("Called functions found: %v", calledFunctions)
		}
	}

	symgotest.Run(t, tc, action)
}
