package evaluator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	// "github.com/google/go-cmp/cmp"
	// "github.com/podhmo/go-scan/symgo/object"
)

// TestRecursiveParser_InfiniteLoop is a regression test for a bug where symgo
// gets stuck in an infinite loop when analyzing a recursive parser.
//
// The test simulates a situation where:
// 1. A main analysis function `pkga.Parse` is called.
// 2. It calls a helper `pkga.ProcessPackage` to process "pkga".
// 3. `ProcessPackage` marks "pkga" as processed in a state object and then calls a function in another package, `pkgb.Process`.
// 4. `pkgb.Process` then calls back into `pkga.ProcessPackage` to process "pkga" again.
//
// The recursion should terminate because `state.Processed["pkga"]` is true.
// However, the original bug caused symgo to not see the state change in the
// symbolic map, leading to an infinite analysis loop.
//
// This test should time out and fail before the fix is applied. After the fix,
// it should pass and correctly assert the final state of the processed map.
func TestRecursiveParser_InfiniteLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // prevent test from running forever
	defer cancel()

	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module mymodule",
		"pkga/parser.go": `
package pkga
import "mymodule/pkgb"
type State struct {
	Processed map[string]bool
}
func Parse() *State {
	state := &State{Processed: make(map[string]bool)}
	ProcessPackage(state, "pkga")
	return state
}
func ProcessPackage(state *State, name string) {
	if state.Processed[name] {
		return
	}
	state.Processed[name] = true
	if name == "pkga" {
		pkgb.Process(state)
	}
}
`,
		"pkgb/process.go": `
package pkgb
import "mymodule/pkga"
func Process(state *pkga.State) {
	pkga.ProcessPackage(state, "pkga") // should be no-op
	pkga.ProcessPackage(state, "pkgb") // should run once
}
`,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("NewScanner failed: %+v", err)
	}

	// Use NewInterpreter to get the evaluator
	i, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
	if err != nil {
		t.Fatalf("NewInterpreter failed: %+v", err)
	}

	// Use the absolute path to the package directory instead of the import path
	// to avoid resolution issues when running `go test` from a subdirectory.
	pkg, err := s.ScanPackage(ctx, filepath.Join(dir, "pkga"))
	if err != nil {
		t.Fatalf("ScanPackage failed: %+v", err)
	}

	var fn *scanner.FunctionInfo
	for _, f := range pkg.Functions {
		if f.Name == "Parse" {
			fn = f
			break
		}
	}
	if fn == nil {
		t.Fatalf("function Parse not found")
	}

	// This should fail with a timeout before the fix.
	_, err = i.Eval(ctx, fn.AstDecl, pkg)
	if err != nil {
		t.Fatalf("Eval failed with unexpected error: %+v", err)
	}

	// // Assertions will be added here after the main bug is fixed.
	// // The test will check the final state of the `Processed` map.
}
