package symgo

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

const (
	moduleDefForInterfaceTest = `
module example.com/me
go 1.21
`
	defPkgForInterfaceTest = `
package def
type Speaker interface {
	Speak() string
}
`
	implPkgForInterfaceTest = `
package impl
import "example.com/me/def"
type Dog struct{}
func (d *Dog) Speak() string {
	return "woof"
}
var _ def.Speaker = (*Dog)(nil)
`
	mainPkgForInterfaceTest = `
package main
import (
	"example.com/me/def"
	"example.com/me/impl"
)
func doSpeak(s def.Speaker) {
	s.Speak()
}
func main() {
	d := &impl.Dog{}
	doSpeak(d)
}
`
)

func TestInterfaceResolution(t *testing.T) {
	files := map[string]string{
		"go.mod":       moduleDefForInterfaceTest,
		"def/def.go":   defPkgForInterfaceTest,
		"impl/impl.go": implPkgForInterfaceTest,
		"main.go":      mainPkgForInterfaceTest,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var dogSpeakCalled bool

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		interp, err := NewInterpreter(s, WithLogger(s.Logger))
		if err != nil {
			return fmt.Errorf("failed to create interpreter: %w", err)
		}

		interp.RegisterDefaultIntrinsic(func(i *Interpreter, args []object.Object) object.Object {
			if len(args) == 0 {
				return nil
			}
			fn, ok := args[0].(*object.Function)
			if !ok {
				return nil
			}

			if fn.Receiver != nil && fn.Receiver.TypeInfo() != nil && fn.Name != nil {
				receiverTypeInfo := fn.Receiver.TypeInfo()
				key := ""
				if ft := fn.Receiver.FieldType(); ft != nil && ft.IsPointer {
					key = fmt.Sprintf("(*%s.%s).%s", receiverTypeInfo.PkgPath, receiverTypeInfo.Name, fn.Name.Name)
				} else {
					key = fmt.Sprintf("(%s.%s).%s", receiverTypeInfo.PkgPath, receiverTypeInfo.Name, fn.Name.Name)
				}

				if key == "(*example.com/me/impl.Dog).Speak" {
					dogSpeakCalled = true
				}
			}
			return nil
		})

		mainPkgInfo, err := s.ScanPackageByImport(ctx, "example.com/me")
		if err != nil {
			return fmt.Errorf("could not scan main package: %w", err)
		}
		// Eval the file to define all symbols
		if _, err := interp.Eval(ctx, mainPkgInfo.AstFiles[mainPkgInfo.Files[0]], mainPkgInfo); err != nil {
			return fmt.Errorf("evaluation of main pkg failed: %w", err)
		}

		// Find the main function and apply it to start the symbolic execution.
		mainFunc, ok := interp.FindObject("main")
		if !ok {
			return fmt.Errorf("could not find main function in interpreter")
		}
		if _, err := interp.Apply(ctx, mainFunc, []object.Object{}, mainPkgInfo); err != nil {
			return fmt.Errorf("error applying main function: %w", err)
		}

		// Now that execution is done, we can call Finalize.
		interp.Finalize(ctx)

		if !dogSpeakCalled {
			return fmt.Errorf("expected (*Dog).Speak to be called via interface resolution, but it was not")
		}

		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}
