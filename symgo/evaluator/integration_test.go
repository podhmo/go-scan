package evaluator_test

import (
	"path/filepath"
	"testing"

	"go/parser"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_ImportAndSelector(t *testing.T) {
	source := `
package main
import (
	"fmt"
	"strings"
)
func main() {
	_ = strings.ToUpper("hello")
	_ = fmt.Println("world")
}
`
	files := map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	}

	// 1. Setup a temporary directory with source files
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// 2. Create a new go-scan Scanner targeting the temp directory
	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %v", err)
	}

	// 3. Scan and parse the file
	pkgs, err := s.Scan(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	mainFileAST := pkgs[0].AstFiles[filepath.Join(dir, "main.go")]
	if mainFileAST == nil {
		t.Fatalf("main.go not found in scanned package")
	}

	// 4. Create evaluator and environment
	// We need to create a new scanner configured for the symgo evaluator's needs.
	// The symgo evaluator needs access to the internal scanner.
	// For this test, we can pass the top-level scanner, and the evaluator can use its internal scanner.
	// Let's assume the evaluator needs access to the raw `scanner.Scanner`.
	// The top-level `goscan.Scanner` doesn't expose it.
	// This suggests a design issue. For now, let's create a raw scanner for the test.
	rawScanner, err := s.ScannerForSymgo()
	if err != nil {
		t.Fatalf("s.ScannerForSymgo() failed: %v", err)
	}
	eval := evaluator.New(rawScanner)
	env := object.NewEnvironment()

	// 5. Evaluate the entire file to handle imports
	eval.Eval(mainFileAST, env)

	// 6. Find the expression `strings.ToUpper` and evaluate it.
	node, err := parser.ParseExpr(`strings.ToUpper`)
	if err != nil {
		t.Fatalf("parser.ParseExpr failed: %v", err)
	}
	obj := eval.Eval(node, env)
	if _, ok := obj.(*object.SymbolicPlaceholder); !ok {
		t.Errorf("Expected a SymbolicPlaceholder for strings.ToUpper, but got %T (%+v)", obj, obj)
	}

	// 7. Test another symbol from a different imported package
	node, err = parser.ParseExpr(`fmt.Println`)
	if err != nil {
		t.Fatalf("parser.ParseExpr failed: %v", err)
	}
	obj = eval.Eval(node, env)
	if _, ok := obj.(*object.SymbolicPlaceholder); !ok {
		t.Errorf("Expected a SymbolicPlaceholder for fmt.Println, but got %T (%+v)", obj, obj)
	}
}
