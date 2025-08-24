package evaluator_test

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/printer"
	"sort"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterfaceTypeFlow(t *testing.T) {
	ctx := context.Background()
	source := `
package main

type Speaker interface {
	Speak() string
}

type Dog struct{}
func (d *Dog) Speak() string { return "woof" }

type Cat struct{}
func (c *Cat) Speak() string { return "meow" }

func GetSpeaker(isDog bool) Speaker {
	var s Speaker
	if isDog {
		s = &Dog{}
	} else {
		s = &Cat{}
	}
	return s
}

func main() {
	s := GetSpeaker(true)
	s.Speak()
}
`
	overlay := scanner.Overlay{
		"main.go": []byte(source),
	}
	var trace strings.Builder
	s, err := goscan.New(goscan.WithOverlay(overlay))
	require.NoError(t, err)

	tracer := symgo.TracerFunc(func(node ast.Node) {
		if node == nil {
			return
		}
		var buf bytes.Buffer
		printer.Fprint(&buf, s.Fset(), node)
		fmt.Fprintf(&trace, "visiting: %T, source: %q\n", node, buf.String())
	})

	interp, err := symgo.NewInterpreter(s, symgo.WithTracer(tracer))
	require.NoError(t, err)

	var capturedPlaceholder *object.SymbolicPlaceholder
	interp.RegisterDefaultIntrinsic(func(i *symgo.Interpreter, args []object.Object) object.Object {
		if len(args) > 0 {
			fnObj := args[0]
			if p, ok := fnObj.(*object.SymbolicPlaceholder); ok {
				if p.UnderlyingMethod != nil && p.UnderlyingMethod.Name == "Speak" {
					capturedPlaceholder = p
				}
			}
		}
		return nil
	})

	// Scan the package from the overlay
	pkgs, err := s.Scan("main.go")
	require.NoError(t, err)
	require.Len(t, pkgs, 1, "expected to scan one package")
	pkg := pkgs[0]

	// Load the package's AST into the interpreter's environment
	// The key in AstFiles is the absolute path.
	require.NotEmpty(t, pkg.Files, "package should have at least one file")
	filePath := pkg.Files[0]
	astFile, ok := pkg.AstFiles[filePath]
	require.True(t, ok, "ast file not found for path: %s", filePath)
	_, err = interp.Eval(ctx, astFile, pkg)
	require.NoError(t, err)

	mainObj, ok := interp.GlobalEnv().Get("main")
	require.True(t, ok, "main function not found in global environment")
	mainFn, ok := mainObj.(*object.Function)
	require.True(t, ok, "main is not an object.Function")

	_, err = interp.Apply(ctx, mainFn, nil, mainFn.Package)
	require.NoError(t, err)

	require.NotNil(t, capturedPlaceholder, "The symbolic placeholder for s.Speak() was not captured")
	t.Log(trace.String())
	require.NotNil(t, capturedPlaceholder, "The symbolic placeholder for s.Speak() was not captured")

	require.Len(t, capturedPlaceholder.PossibleConcreteTypes, 2, "Should have found 2 possible concrete types")

	typeNames := make([]string, len(capturedPlaceholder.PossibleConcreteTypes))
	for i, ti := range capturedPlaceholder.PossibleConcreteTypes {
		typeNames[i] = fmt.Sprintf("%s.%s", ti.PkgPath, ti.Name)
	}
	sort.Strings(typeNames)

	assert.Equal(t, "main.Cat", typeNames[0], "Expected Cat type")
	assert.Equal(t, "main.Dog", typeNames[1], "Expected Dog type")
}
