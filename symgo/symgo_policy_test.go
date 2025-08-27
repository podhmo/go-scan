package symgo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	scan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"

	"github.com/podhmo/go-scan/symgo/object"
)

func TestScanPolicyForMethodResolution(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{
		"app/go.mod": "module app\n",
		"app/main.go": `
package main
import "dep/pkg"
func main() {
	c := pkg.NewComponent()
	c.DoSomething()
}`,
		"dep/go.mod": "module dep\n",
		"dep/pkg/component.go": `
package pkg
type Component struct{}
func NewComponent() *Component { return &Component{} }
func (c *Component) DoSomething() {}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	appDir := filepath.Join(dir, "app")
	depDir := filepath.Join(dir, "dep")

	// Create a scanner that knows about both modules
	s, err := scan.New(
		scan.WithModuleDirs([]string{appDir, depDir}),
		scan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("scanner creation failed: %v", err)
	}

	// Define a policy that ONLY allows scanning the 'app' module
	scanPolicy := func(importPath string) bool {
		return strings.HasPrefix(importPath, "app")
	}

	interp, err := symgo.NewInterpreter(s, symgo.WithScanPolicy(scanPolicy))
	if err != nil {
		t.Fatalf("interpreter creation failed: %v", err)
	}

	// 1. Evaluate the main package of the app
	mainPkg, err := s.ScanPackageByImport(ctx, "app")
	if err != nil {
		t.Fatalf("ScanPackageByImport(app) failed: %v", err)
	}
	mainFile := FindFile(t, mainPkg, "main.go")
	if _, err := interp.Eval(ctx, mainFile, mainPkg); err != nil {
		t.Fatalf("Eval(main.go) failed: %v", err)
	}

	// 2. Find the main function
	mainObj, ok := interp.FindObject("main")
	if !ok {
		t.Fatal("main function not found")
	}
	mainFunc, ok := mainObj.(*object.Function)
	if !ok {
		t.Fatalf("main object is not a function, but %T", mainObj)
	}

	// 3. Apply the main function to start symbolic execution
	_, err = interp.Apply(ctx, mainFunc, nil, mainPkg)

	// With the fix, the interpreter should no longer error when encountering a
	// method call on a variable whose type is from a non-scanned module.
	// It should treat the call as symbolic and continue without failing.
	if err != nil {
		t.Fatalf("expected no error, but got: %v", err)
	}
}
