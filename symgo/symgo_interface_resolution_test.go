package symgo

import (
	"context"
	"fmt"
	"strings"
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

	perms := [][]string{
		{"example.com/me/def", "example.com/me/impl", "example.com/me"},
		{"example.com/me/def", "example.com/me", "example.com/me/impl"},
		{"example.com/me/impl", "example.com/me/def", "example.com/me"},
		{"example.com/me/impl", "example.com/me", "example.com/me/def"},
		{"example.com/me", "example.com/me/def", "example.com/me/impl"},
		{"example.com/me", "example.com/me/impl", "example.com/me/def"},
	}

	for _, p := range perms {
		t.Run(strings.Join(p, ">"), func(t *testing.T) {
			s, err := goscan.New(
				goscan.WithWorkDir(dir),
				goscan.WithGoModuleResolver(),
			)
			if err != nil {
				t.Fatalf("failed to create scanner: %v", err)
			}

			interp, err := NewInterpreter(s, WithLogger(s.Logger))
			if err != nil {
				t.Fatalf("failed to create interpreter: %v", err)
			}

			var dogSpeakCalled bool
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

			ctx := context.Background()
			for _, pkgPath := range p {
				if _, err := s.ScanPackageByImport(ctx, pkgPath); err != nil {
					t.Fatalf("could not scan package %s: %v", pkgPath, err)
				}
			}

			mainPkgInfo, err := s.ScanPackageByImport(ctx, "example.com/me")
			if err != nil {
				t.Fatalf("could not get main package: %v", err)
			}
			if _, err := interp.Eval(ctx, mainPkgInfo.AstFiles[mainPkgInfo.Files[0]], mainPkgInfo); err != nil {
				t.Fatalf("evaluation of main pkg failed: %v", err)
			}

			mainFunc, ok := interp.FindObjectInPackage(ctx, "example.com/me", "main")
			if !ok {
				t.Fatalf("could not find main function in interpreter")
			}
			if _, err := interp.Apply(ctx, mainFunc, []object.Object{}, mainPkgInfo); err != nil {
				t.Fatalf("error applying main function: %v", err)
			}

			interp.Finalize(ctx)

			if !dogSpeakCalled {
				t.Errorf("expected (*Dog).Speak to be called via interface resolution, but it was not")
			}
		})
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

	perms := [][]string{
		{"example.com/me/def", "example.com/me/impl", "example.com/me"},
		{"example.com/me/def", "example.com/me", "example.com/me/impl"},
		{"example.com/me/impl", "example.com/me/def", "example.com/me"},
		{"example.com/me/impl", "example.com/me", "example.com/me/def"},
		{"example.com/me", "example.com/me/def", "example.com/me/impl"},
		{"example.com/me", "example.com/me/impl", "example.com/me/def"},
	}

	for _, p := range perms {
		t.Run(strings.Join(p, ">"), func(t *testing.T) {
			s, err := goscan.New(
				goscan.WithWorkDir(dir),
				goscan.WithGoModuleResolver(),
			)
			if err != nil {
				t.Fatalf("failed to create scanner: %v", err)
			}

			interp, err := NewInterpreter(s, WithLogger(s.Logger))
			if err != nil {
				t.Fatalf("failed to create interpreter: %v", err)
			}

			var personGreetCalled bool
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

			ctx := context.Background()
			for _, pkgPath := range p {
				if _, err := s.ScanPackageByImport(ctx, pkgPath); err != nil {
					t.Fatalf("could not scan package %s: %v", pkgPath, err)
				}
			}

			mainPkgInfo, err := s.ScanPackageByImport(ctx, "example.com/me")
			if err != nil {
				t.Fatalf("could not get main package: %v", err)
			}

			if _, err := interp.Eval(ctx, mainPkgInfo.AstFiles[mainPkgInfo.Files[0]], mainPkgInfo); err != nil {
				t.Fatalf("evaluation of main pkg failed: %v", err)
			}

			mainFunc, ok := interp.FindObjectInPackage(ctx, "example.com/me", "main")
			if !ok {
				t.Fatalf("could not find main function in interpreter")
			}
			if _, err := interp.Apply(ctx, mainFunc, []object.Object{}, mainPkgInfo); err != nil {
				t.Fatalf("error applying main function: %v", err)
			}

			interp.Finalize(ctx)

			if !personGreetCalled {
				t.Errorf("expected (*Person).Greet to be called via interface resolution, but it was not")
			}
		})
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

	perms := [][]string{
		{"example.com/me/def", "example.com/me/impl", "example.com/me"},
		{"example.com/me/def", "example.com/me", "example.com/me/impl"},
		{"example.com/me/impl", "example.com/me/def", "example.com/me"},
		{"example.com/me/impl", "example.com/me", "example.com/me/def"},
		{"example.com/me", "example.com/me/def", "example.com/me/impl"},
		{"example.com/me", "example.com/me/impl", "example.com/me/def"},
	}

	for _, p := range perms {
		t.Run(strings.Join(p, ">"), func(t *testing.T) {
			s, err := goscan.New(
				goscan.WithWorkDir(dir),
				goscan.WithGoModuleResolver(),
			)
			if err != nil {
				t.Fatalf("failed to create scanner: %v", err)
			}

			interp, err := NewInterpreter(s, WithLogger(s.Logger))
			if err != nil {
				t.Fatalf("failed to create interpreter: %v", err)
			}

			var emailSendCalled bool
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

			ctx := context.Background()
			for _, pkgPath := range p {
				if _, err := s.ScanPackageByImport(ctx, pkgPath); err != nil {
					t.Fatalf("could not scan package %s: %v", pkgPath, err)
				}
			}

			mainPkgInfo, err := s.ScanPackageByImport(ctx, "example.com/me")
			if err != nil {
				t.Fatalf("could not get main package: %v", err)
			}

			if _, err := interp.Eval(ctx, mainPkgInfo.AstFiles[mainPkgInfo.Files[0]], mainPkgInfo); err != nil {
				t.Fatalf("evaluation of main pkg failed: %v", err)
			}

			mainFunc, ok := interp.FindObjectInPackage(ctx, "example.com/me", "main")
			if !ok {
				t.Fatalf("could not find main function in interpreter")
			}
			if _, err := interp.Apply(ctx, mainFunc, []object.Object{}, mainPkgInfo); err != nil {
				t.Fatalf("error applying main function: %v", err)
			}

			interp.Finalize(ctx)

			if !emailSendCalled {
				t.Errorf("expected (Email).Send to be called via interface resolution, but it was not")
			}
		})
	}
}
