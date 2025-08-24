package evaluator_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
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
	// Create a self-contained module in a temporary directory for the test.
	tempDir := t.TempDir()
	goModContent := "module main"
	err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0644)
	if err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	overlay := scanner.Overlay{
		"main.go": []byte(source),
	}

	s, err := goscan.New(goscan.WithWorkDir(tempDir), goscan.WithOverlay(overlay))
	if err != nil {
		t.Fatalf("goscan.New() failed: %v", err)
	}

	logBuf := new(bytes.Buffer)
	var out io.Writer = logBuf
	if os.Getenv("DEBUG") != "" {
		out = os.Stderr
	}
	logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	interp, err := symgo.NewInterpreter(s, symgo.WithLogger(logger))
	if err != nil {
		t.Fatalf("symgo.NewInterpreter() failed: %v", err)
	}

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

	pkgs, err := s.Scan("main.go")
	if err != nil {
		t.Fatalf("s.Scan() failed: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected to scan one package, but got %d", len(pkgs))
	}
	pkg := pkgs[0]

	if len(pkg.Files) == 0 {
		t.Fatal("package should have at least one file")
	}
	filePath := pkg.Files[0]
	astFile, ok := pkg.AstFiles[filePath]
	if !ok {
		t.Fatalf("ast file not found for path: %s", filePath)
	}
	_, err = interp.Eval(ctx, astFile, pkg)
	if err != nil {
		t.Fatalf("interp.Eval() failed: %v", err)
	}

	mainObj, ok := interp.GlobalEnv().Get("main")
	if !ok {
		t.Fatal("main function not found in global environment")
	}
	mainFn, ok := mainObj.(*object.Function)
	if !ok {
		t.Fatal("main is not an object.Function")
	}

	_, err = interp.Apply(ctx, mainFn, nil, mainFn.Package)
	if err != nil {
		t.Fatalf("interp.Apply() failed: %v", err)
	}

	if capturedPlaceholder == nil {
		t.Log(logBuf.String())
		t.Fatal("The symbolic placeholder for s.Speak() was not captured")
	}

	if len(capturedPlaceholder.PossibleConcreteTypes) != 2 {
		t.Log(logBuf.String())
		t.Fatalf("Should have found 2 possible concrete types, but got %d", len(capturedPlaceholder.PossibleConcreteTypes))
	}

	typeNames := make([]string, len(capturedPlaceholder.PossibleConcreteTypes))
	for i, ti := range capturedPlaceholder.PossibleConcreteTypes {
		typeNames[i] = fmt.Sprintf("%s.%s", ti.PkgPath, ti.Name)
	}
	sort.Strings(typeNames)

	expected := []string{"main.Cat", "main.Dog"}
	if diff := cmp.Diff(expected, typeNames); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
