package symgo_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestGenericCallWithUnionInterfaceConstraint(t *testing.T) {
	source := `
package main

type Foo struct {
	Name string
}

func (f *Foo) Login() string {
	return "foo logged in: " + f.Name
}

type Bar struct {
	ID int
}

func (b Bar) Login() string {
	return "bar logged in"
}

// Loginable is a union interface that also requires a method.
type Loginable interface {
	*Foo | Bar
	Login() string
}

// WithLogin is a generic function that uses the union interface as a constraint.
func WithLogin[T Loginable](loginable T) {
	loginable.Login()
}

func main() {
	var f *Foo
	WithLogin(f) // Type args can be inferred

	var b Bar
	WithLogin(b) // Type args can be inferred
}
`
	files := map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// Setup scanner
	s, err := goscan.New(
		goscan.WithWorkDir(dir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	// Setup interpreter
	interp, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
	if err != nil {
		t.Fatalf("failed to create interpreter: %v", err)
	}

	// Use a map to track which methods were called.
	calledMethods := make(map[string]bool)
	var mu sync.Mutex

	// Register the intrinsic BEFORE evaluation
	interp.RegisterDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
		if len(args) == 0 {
			return nil
		}
		fn, ok := args[0].(*object.Function)
		if !ok {
			return nil
		}
		if fn.Receiver == nil || fn.Receiver.TypeInfo() == nil || fn.Name == nil {
			return nil
		}

		receiverTypeInfo := fn.Receiver.TypeInfo()
		key := ""

		var receiverIsPointer bool
		if ft := fn.Receiver.FieldType(); ft != nil {
			receiverIsPointer = ft.IsPointer
		} else if _, isPtr := fn.Receiver.(*object.Pointer); isPtr {
			receiverIsPointer = true
		}

		if receiverIsPointer {
			key = fmt.Sprintf("(*%s.%s).%s", receiverTypeInfo.PkgPath, receiverTypeInfo.Name, fn.Name.Name)
		} else {
			key = fmt.Sprintf("(%s.%s).%s", receiverTypeInfo.PkgPath, receiverTypeInfo.Name, fn.Name.Name)
		}

		if strings.HasPrefix(key, "(*mymodule.") || strings.HasPrefix(key, "(mymodule.") {
			mu.Lock()
			calledMethods[key] = true
			mu.Unlock()
		}
		return nil
	})

	// Run evaluation
	ctx := context.Background()
	mainPkgInfo, err := s.ScanPackageFromImportPath(ctx, "mymodule")
	if err != nil {
		t.Fatalf("could not get main package: %v", err)
	}

	mainFunc, ok := interp.FindObjectInPackage(ctx, "mymodule", "main")
	if !ok {
		t.Fatalf("could not find main function in interpreter")
	}

	if _, err := interp.Apply(ctx, mainFunc, []object.Object{}, mainPkgInfo); err != nil {
		t.Fatalf("error applying main function: %v", err)
	}

	// Check if all expected methods were called.
	expectedMethods := []string{
		"(*mymodule.Foo).Login",
		"(mymodule.Bar).Login",
	}

	mu.Lock()
	defer mu.Unlock()

	missing := []string{}
	for _, method := range expectedMethods {
		if !calledMethods[method] {
			missing = append(missing, method)
		}
	}

	if len(missing) > 0 {
		t.Errorf("expected methods were not called:\n- %s", strings.Join(missing, "\n- "))
	}
}

func TestGenericCallWithCrossPackageUnionInterfaceConstraint(t *testing.T) {
	files := map[string]string{
		"go.mod": "module mymodule",
		"types/types.go": `
package types

type Foo struct {
	Name string
}

func (f *Foo) Login() string {
	return "foo logged in: " + f.Name
}

type Bar struct {
	ID int
}

func (b Bar) Login() string {
	return "bar logged in"
}
`,
		"iface/iface.go": `
package iface

import "mymodule/types"

// Loginable is a union interface that also requires a method.
type Loginable interface {
	*types.Foo | types.Bar
	Login() string
}

// WithLogin is a generic function that uses the union interface as a constraint.
func WithLogin[T Loginable](loginable T) {
	loginable.Login()
}
`,
		"main.go": `
package main

import (
	"mymodule/iface"
	"mymodule/types"
)

func main() {
	var f *types.Foo
	iface.WithLogin(f)

	var b types.Bar
	iface.WithLogin(b)
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// Setup scanner
	s, err := goscan.New(
		goscan.WithWorkDir(dir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	// Setup interpreter
	interp, err := symgo.NewInterpreter(s, symgo.WithLogger(s.Logger))
	if err != nil {
		t.Fatalf("failed to create interpreter: %v", err)
	}

	// Use a map to track which methods were called.
	calledMethods := make(map[string]bool)
	var mu sync.Mutex

	// Register the intrinsic BEFORE evaluation
	interp.RegisterDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
		if len(args) == 0 {
			return nil
		}
		fn, ok := args[0].(*object.Function)
		if !ok {
			return nil
		}
		if fn.Receiver == nil || fn.Receiver.TypeInfo() == nil || fn.Name == nil {
			return nil
		}

		receiverTypeInfo := fn.Receiver.TypeInfo()
		key := ""

		var receiverIsPointer bool
		if ft := fn.Receiver.FieldType(); ft != nil {
			receiverIsPointer = ft.IsPointer
		} else if _, isPtr := fn.Receiver.(*object.Pointer); isPtr {
			receiverIsPointer = true
		}

		if receiverIsPointer {
			key = fmt.Sprintf("(*%s.%s).%s", receiverTypeInfo.PkgPath, receiverTypeInfo.Name, fn.Name.Name)
		} else {
			key = fmt.Sprintf("(%s.%s).%s", receiverTypeInfo.PkgPath, receiverTypeInfo.Name, fn.Name.Name)
		}

		if strings.HasPrefix(key, "(*mymodule/types.") || strings.HasPrefix(key, "(mymodule/types.") {
			mu.Lock()
			calledMethods[key] = true
			mu.Unlock()
		}
		return nil
	})

	// Run evaluation
	ctx := context.Background()
	// Scan all packages before starting evaluation from main
	for _, pkgPath := range []string{"mymodule/types", "mymodule/iface", "mymodule"} {
		if _, err := s.ScanPackageFromImportPath(ctx, pkgPath); err != nil {
			t.Fatalf("could not scan package %s: %v", pkgPath, err)
		}
	}

	mainPkgInfo, err := s.ScanPackageFromImportPath(ctx, "mymodule")
	if err != nil {
		t.Fatalf("could not get main package: %v", err)
	}

	mainFunc, ok := interp.FindObjectInPackage(ctx, "mymodule", "main")
	if !ok {
		t.Fatalf("could not find main function in interpreter")
	}

	if _, err := interp.Apply(ctx, mainFunc, []object.Object{}, mainPkgInfo); err != nil {
		t.Fatalf("error applying main function: %v", err)
	}

	// Check if all expected methods were called.
	expectedMethods := []string{
		"(*mymodule/types.Foo).Login",
		"(mymodule/types.Bar).Login",
	}

	mu.Lock()
	defer mu.Unlock()

	missing := []string{}
	for _, method := range expectedMethods {
		if !calledMethods[method] {
			missing = append(missing, method)
		}
	}

	if len(missing) > 0 {
		t.Errorf("expected methods were not called:\n- %s", strings.Join(missing, "\n- "))
	}
}
