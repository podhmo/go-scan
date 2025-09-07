package symgo_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrossPackageInterfaceImplementation(t *testing.T) {
	const moduleName = "example.com/testmodule"
	baseFiles := map[string]string{
		"go.mod":    fmt.Sprintf("module %s\n\ngo 1.22", moduleName),
		"pkga/a.go": `package pkga; type Greeter interface { Greet() string }`,
		"pkgb/b.go": `package pkgb; import "fmt"; type MyGreeter struct{}; func (g *MyGreeter) Greet() string { return fmt.Sprintf("hello from MyGreeter") }`,
		"pkgc/c.go": fmt.Sprintf(`package pkgc; import ("fmt"; "%s/pkga"; "%s/pkgb"); func UseGreeter() { var i pkga.Greeter; i = &pkgb.MyGreeter{}; fmt.Println(i.Greet()) }`, moduleName, moduleName),
	}

	discoveryOrders := [][]string{
		{fmt.Sprintf("%s/pkga", moduleName), fmt.Sprintf("%s/pkgb", moduleName), fmt.Sprintf("%s/pkgc", moduleName)},
		{fmt.Sprintf("%s/pkga", moduleName), fmt.Sprintf("%s/pkgc", moduleName), fmt.Sprintf("%s/pkgb", moduleName)},
		{fmt.Sprintf("%s/pkgb", moduleName), fmt.Sprintf("%s/pkga", moduleName), fmt.Sprintf("%s/pkgc", moduleName)},
		{fmt.Sprintf("%s/pkgb", moduleName), fmt.Sprintf("%s/pkgc", moduleName), fmt.Sprintf("%s/pkga", moduleName)},
		{fmt.Sprintf("%s/pkgc", moduleName), fmt.Sprintf("%s/pkga", moduleName), fmt.Sprintf("%s/pkgb", moduleName)},
		{fmt.Sprintf("%s/pkgc", moduleName), fmt.Sprintf("%s/pkgb", moduleName), fmt.Sprintf("%s/pkga", moduleName)},
	}

	dir, cleanup := scantest.WriteFiles(t, baseFiles)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// Create a map of import paths to packages for easy lookup.
		pkgMap := make(map[string]*goscan.Package, len(pkgs))
		for _, pkg := range pkgs {
			pkgMap[pkg.ImportPath] = pkg
		}

		for _, order := range discoveryOrders {
			t.Run(strings.Join(order, " -> "), func(t *testing.T) {
				var calledMethods []string
				// We create a new interpreter for each sub-test to ensure a clean state.
				interp, err := symgo.NewInterpreter(s, symgo.WithPrimaryAnalysisScope(fmt.Sprintf("%s/...", moduleName)))
				require.NoError(t, err)

				interp.RegisterDefaultIntrinsic(func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
					if len(args) > 0 {
						if fn, ok := args[0].(*object.Function); ok && fn.Def != nil && fn.Package != nil {
							// Only record calls within our test module
							if !strings.HasPrefix(fn.Package.ImportPath, moduleName) {
								return nil
							}
							var recv string
							if fn.Def.Receiver != nil {
								recv = fn.Def.Receiver.Type.String()
							}
							fqn := fmt.Sprintf("%s:%s.%s", fn.Package.ImportPath, recv, fn.Def.Name)
							calledMethods = append(calledMethods, fqn)
						}
					}
					return nil
				})

				// Evaluate packages in the specified order, using the packages scanned by scantest.Run
				for _, pkgImportPath := range order {
					pkg, ok := pkgMap[pkgImportPath]
					require.True(t, ok, "package %s not found in scanned packages", pkgImportPath)
					for _, file := range pkg.AstFiles {
						_, err := interp.Eval(ctx, file, pkg)
						require.NoError(t, err, "Eval in sub-test failed for pkg %s", pkg.ImportPath)
					}
				}

				entryFunc, ok := interp.FindObject("UseGreeter")
				require.True(t, ok, "could not find entry point function UseGreeter")

				entryPkgPath := fmt.Sprintf("%s/pkgc", moduleName)
				entryPkg, ok := pkgMap[entryPkgPath]
				require.True(t, ok, "entry package %s not found", entryPkgPath)

				_, err = interp.Apply(ctx, entryFunc, nil, entryPkg)
				require.NoError(t, err)

				sort.Strings(calledMethods)
				expectedMethod := fmt.Sprintf("%s/pkgb:*MyGreeter.Greet", moduleName)
				assert.Contains(t, calledMethods, expectedMethod)
			})
		}
		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"./..."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
