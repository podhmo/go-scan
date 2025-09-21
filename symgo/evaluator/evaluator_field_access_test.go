package evaluator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_FieldAccessOnSymbolicPlaceholder(t *testing.T) {
	source := `
package main

type MyType struct {
	Name string
	_    struct{}
}

func (t *MyType) GetName() string {
	return t.Name
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]

		modules := s.Modules()
		var scanPolicy object.ScanPolicyFunc
		if len(modules) > 0 {
			modulePaths := make([]string, len(modules))
			for i, m := range modules {
				modulePaths[i] = m.Path
			}
			scanPolicy = func(importPath string) bool {
				for _, modulePath := range modulePaths {
					if strings.HasPrefix(importPath, modulePath) {
						return true
					}
				}
				return false
			}
		} else {
			scanPolicy = func(importPath string) bool { return false }
		}

		eval := New(s, s.Logger, nil, scanPolicy)

		env := object.NewEnclosedEnvironment(eval.UniverseEnv)
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		var targetFuncInfo *goscan.FunctionInfo
		for _, f := range pkg.Functions {
			if f.Name == "GetName" {
				targetFuncInfo = f
				break
			}
		}
		if targetFuncInfo == nil {
			return fmt.Errorf("function 'GetName' not found in scanner results")
		}

		targetFunc := &object.Function{
			Decl:    targetFuncInfo.AstDecl,
			Body:    targetFuncInfo.AstDecl.Body,
			Package: pkg,
			Env:     env,
			Def:     targetFuncInfo,
		}

		result := eval.Apply(ctx, targetFunc, nil, pkg, env)

		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed unexpectedly: %w", err)
		}
		if ret, ok := result.(*object.ReturnValue); ok {
			if err, ok := ret.Value.(*object.Error); ok {
				return fmt.Errorf("evaluation returned an error: %w", err)
			}
			if _, ok := ret.Value.(*object.SymbolicPlaceholder); !ok {
				return fmt.Errorf("expected return value to be a SymbolicPlaceholder, but got %T", ret.Value)
			}
		} else {
			return fmt.Errorf("expected return value from Apply, but got %T", result)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
