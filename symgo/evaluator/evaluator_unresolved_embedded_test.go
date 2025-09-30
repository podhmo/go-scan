package evaluator_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

// splitQualifiedName splits a name like "pkg/path.Name" into "pkg/path" and "Name".
func splitQualifiedName(name string) (pkgPath, typeName string) {
	lastDot := strings.LastIndex(name, ".")
	if lastDot == -1 {
		return "", name
	}
	return name[:lastDot], name[lastDot+1:]
}

func TestUnresolvedEmbedded(t *testing.T) {
	cases := []struct {
		msg         string
		source      map[string]string
		entrypoint  string
		wantLogs    []string
	}{
		{
			msg: "access field on embedded struct from out-of-policy package",
			source: map[string]string{
				"go.mod": "module example.com/m",
				"app/app.go": `
package app
import "lib"
type Application struct {
	*lib.CLI
}
func NewApp() *Application {
	app := &Application{}
	_ = app.Name // access embedded field
	return app
}
`,
				"lib/lib.go": `
package lib
type CLI struct {
	Name string
}
`,
			},
			entrypoint:  "example.com/m/app.NewApp",
			wantLogs: []string{
				`"msg":"assuming field exists on out-of-policy embedded type","field_name":"Name","type_name":"lib.CLI"`,
			},
		},
		{
			msg: "access method on embedded struct from out-of-policy package",
			source: map[string]string{
				"go.mod": "module example.com/m",
				"app/app.go": `
package app
import "lib"
type Application struct {
	*lib.CLI
}
func NewApp() *Application {
	app := &Application{}
	app.Run() // access embedded method
	return app
}
`,
				"lib/lib.go": `
package lib
type CLI struct {}
func (c *CLI) Run() {}
`,
			},
			entrypoint:  "example.com/m/app.NewApp",
			wantLogs: []string{
				`"msg":"assuming method exists on out-of-policy embedded type","method_name":"Run","type_name":"lib.CLI"`,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.msg, func(t *testing.T) {
			dir, cleanup := scantest.WriteFiles(t, c.source)
			defer cleanup()

			var logBuf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

			scanPolicy := func(path string) bool {
				return path == "example.com/m/app"
			}

			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
				pkgPath, fnName := splitQualifiedName(c.entrypoint)
				if pkgPath == "" {
					return fmt.Errorf("invalid entrypoint: %s", c.entrypoint)
				}

				interp, err := symgo.NewInterpreter(s, symgo.WithLogger(logger), symgo.WithScanPolicy(scanPolicy))
				if err != nil {
					return fmt.Errorf("NewInterpreter failed: %w", err)
				}

				fnObj, ok := interp.FindObjectInPackage(ctx, pkgPath, fnName)
				if !ok {
					return fmt.Errorf("entry point function %q not found in package %q", fnName, pkgPath)
				}

				var entryPointPkg *goscan.Package
				for _, p := range pkgs {
					if p.ImportPath == pkgPath {
						entryPointPkg = p
						break
					}
				}
				if entryPointPkg == nil {
					return fmt.Errorf("entry point package %q not found", pkgPath)
				}

				result := interp.EvaluatorForTest().Apply(ctx, fnObj, nil, entryPointPkg)

				if result != nil {
					if err, ok := result.(*object.Error); ok {
						t.Errorf("got unexpected error, but want success: %v", err)
					}
					if ret, ok := result.(*object.ReturnValue); ok {
						if err, ok := ret.Value.(*object.Error); ok {
							t.Errorf("got unexpected error, but want success: %v", err)
						}
					}
				}

				logs := logBuf.String()
				for _, want := range c.wantLogs {
					if !strings.Contains(logs, want) {
						t.Errorf("did not find log entry\n  want: %q\n  logs:\n%s", want, logs)
					}
				}
				return nil
			}

			_, err := scantest.Run(t, context.Background(), dir, []string{"./..."}, action)
			if err != nil {
				t.Fatalf("scantest.Run() failed unexpectedly: %+v", err)
			}
		})
	}
}