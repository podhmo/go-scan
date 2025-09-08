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


const (
	moduleDefForPointerReceiverTest = `
module example.com/me
go 1.21
`
	defPkgForPointerReceiverTest = `
package def
type Greeter interface {
	Greet() string
}
`
	implPkgForPointerReceiverTest = `
package impl
import "example.com/me/def"
type Person struct{ Name string }
func (p *Person) Greet() string {
	return "hello " + p.Name
}
var _ def.Greeter = (*Person)(nil)
`
	mainPkgForPointerReceiverTest = `
package main
import (
	"example.com/me/def"
	"example.com/me/impl"
)
func doGreet(g def.Greeter) {
	g.Greet()
}
func main() {
	p := &impl.Person{Name: "world"}
	doGreet(p)
}
`
)

func TestInterfaceResolutionWithPointerReceiver(t *testing.T) {
	files := map[string]string{
		"go.mod":       moduleDefForPointerReceiverTest,
		"def/def.go":   defPkgForPointerReceiverTest,
		"impl/impl.go": implPkgForPointerReceiverTest,
		"main.go":      mainPkgForPointerReceiverTest,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var personGreetCalled bool

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

				if key == "(*example.com/me/impl.Person).Greet" {
					personGreetCalled = true
				}
			}
			return nil
		})

		mainPkgInfo, err := s.ScanPackageByImport(ctx, "example.com/me")
		if err != nil {
			return fmt.Errorf("could not scan main package: %w", err)
		}

		if _, err := interp.Eval(ctx, mainPkgInfo.AstFiles[mainPkgInfo.Files[0]], mainPkgInfo); err != nil {
			return fmt.Errorf("evaluation of main pkg failed: %w", err)
		}

		mainFunc, ok := interp.FindObject("main")
		if !ok {
			return fmt.Errorf("could not find main function in interpreter")
		}
		if _, err := interp.Apply(ctx, mainFunc, []object.Object{}, mainPkgInfo); err != nil {
			return fmt.Errorf("error applying main function: %w", err)
		}

		interp.Finalize(ctx)

		if !personGreetCalled {
			return fmt.Errorf("expected (*Person).Greet to be called via interface resolution, but it was not")
		}

		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}

const (
	moduleDefForValueReceiverTest = `
module example.com/me
go 1.21
`
	defPkgForValueReceiverTest = `
package def
type Messenger interface {
	Send() string
}
`
	implPkgForValueReceiverTest = `
package impl
import "example.com/me/def"
type Email struct{}
func (e Email) Send() string {
	return "sending email"
}
var _ def.Messenger = Email{}
`
	mainPkgForValueReceiverTest = `
package main
import (
	"example.com/me/def"
	"example.com/me/impl"
)
func doSend(m def.Messenger) {
	m.Send()
}
func main() {
	e := impl.Email{}
	doSend(e)
}
`
)

func TestInterfaceResolutionWithValueReceiver(t *testing.T) {
	files := map[string]string{
		"go.mod":       moduleDefForValueReceiverTest,
		"def/def.go":   defPkgForValueReceiverTest,
		"impl/impl.go": implPkgForValueReceiverTest,
		"main.go":      mainPkgForValueReceiverTest,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var emailSendCalled bool

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

				if key == "(example.com/me/impl.Email).Send" {
					emailSendCalled = true
				}
			}
			return nil
		})

		mainPkgInfo, err := s.ScanPackageByImport(ctx, "example.com/me")
		if err != nil {
			return fmt.Errorf("could not scan main package: %w", err)
		}

		if _, err := interp.Eval(ctx, mainPkgInfo.AstFiles[mainPkgInfo.Files[0]], mainPkgInfo); err != nil {
			return fmt.Errorf("evaluation of main pkg failed: %w", err)
		}

		mainFunc, ok := interp.FindObject("main")
		if !ok {
			return fmt.Errorf("could not find main function in interpreter")
		}
		if _, err := interp.Apply(ctx, mainFunc, []object.Object{}, mainPkgInfo); err != nil {
			return fmt.Errorf("error applying main function: %w", err)
		}

		interp.Finalize(ctx)

		if !emailSendCalled {
			return fmt.Errorf("expected (Email).Send to be called via interface resolution, but it was not")
		}

		return nil
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}
