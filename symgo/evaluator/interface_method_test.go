package evaluator

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_ExternalInterfaceMethodCall(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"iface/iface.go": `
package iface
type Writer interface {
	Write(p []byte) (n int, err error)
}`,
		"main.go": `
package main
import "example.com/me/iface"
func Do(w iface.Writer) {
	w.Write(nil)
}
func main() {
	Do(nil)
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var writeCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		if mainPkg.Name != "main" {
			mainPkg = findPkg(pkgs, "main")
		}
		eval := New(s, s.Logger, nil, nil)

		key := "(example.com/me/iface.Writer).Write"
		eval.RegisterIntrinsic(key, func(ctx context.Context, args ...object.Object) object.Object {
			writeCalled = true
			return nil
		})

		for _, file := range mainPkg.AstFiles {
			eval.Eval(ctx, file, nil, mainPkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		result := eval.Apply(ctx, mainFunc, []object.Object{}, mainPkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed: %s", err.Message)
		}
		return nil
	}

	// Let scantest.Run create and configure the scanner.
	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if !writeCalled {
		t.Errorf("intrinsic for external interface method was not called")
	}
}

func TestEval_InterfaceMethodCall(t *testing.T) {
	code := `
package main

type Writer interface {
	Write(p []byte) (n int, err error)
}

func Do(w Writer) {
	w.Write(nil)
}

func main() {
	Do(nil)
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var writeCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		key := fmt.Sprintf("(%s.Writer).Write", pkg.ImportPath)
		eval.RegisterIntrinsic(key, func(ctx context.Context, args ...object.Object) object.Object {
			writeCalled = true
			return nil
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed: %s", err.Message)
		}
		return nil
	}

	// Let scantest.Run create and configure the scanner.
	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if !writeCalled {
		t.Errorf("intrinsic for (main.Writer).Write was not called")
	}
}

func TestEval_InterfaceMethodCall_OnConcreteType(t *testing.T) {
	code := `
package main

type Speaker interface {
	Speak() string
}

type Dog struct {}
func (d *Dog) Speak() string { return "woof" }

func main() {
	var s Speaker
	s = &Dog{}
	s.Speak()
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var placeholderCalled bool
	var concreteFuncCalled bool

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		eval.RegisterDefaultIntrinsic(func(ctx context.Context, args ...object.Object) object.Object {
			if len(args) == 0 {
				return nil
			}
			fnObj := args[0]
			switch fn := fnObj.(type) {
			case *object.SymbolicPlaceholder:
				if fn.UnderlyingFunc != nil && fn.UnderlyingFunc.Name == "Speak" {
					placeholderCalled = true
				}
			case *object.Function:
				// We want to ensure the concrete method is NOT called directly by the intrinsic system
				// when the variable's static type is an interface.
				if fn.Name.Name == "Speak" {
					concreteFuncCalled = true
				}
			}
			return nil
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed: %s", err.Message)
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if !placeholderCalled {
		t.Errorf("expected SymbolicPlaceholder to be created for interface call on concrete type, but it was not")
	}
	if concreteFuncCalled {
		t.Errorf("expected interface call to NOT resolve to concrete function in intrinsic, but it did")
	}
}

func TestEval_InterfaceMethodCall_AcrossControlFlow(t *testing.T) {
	code := `
package main

var someCondition bool // This will be symbolic

type Speaker interface {
	Speak() string
}

type Dog struct{}
func (d *Dog) Speak() string { return "woof" }

type Cat struct{}
func (c *Cat) Speak() string { return "meow" }

func main() {
	var s Speaker
	if someCondition {
		s = &Dog{}
	} else {
		s = &Cat{}
	}
	s.Speak()
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var speakPlaceholder *object.SymbolicPlaceholder

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		eval.RegisterDefaultIntrinsic(func(ctx context.Context, args ...object.Object) object.Object {
			if len(args) == 0 {
				return nil
			}
			if p, ok := args[0].(*object.SymbolicPlaceholder); ok {
				if p.UnderlyingFunc != nil && p.UnderlyingFunc.Name == "Speak" {
					speakPlaceholder = p
				}
			}
			return nil
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed: %s", err.Message)
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if speakPlaceholder == nil {
		t.Fatalf("SymbolicPlaceholder for Speak method was not captured")
	}

	receiverVar, ok := speakPlaceholder.Receiver.(*object.Variable)
	if !ok {
		t.Fatalf("placeholder receiver is not a *object.Variable, but %T", speakPlaceholder.Receiver)
	}

	if len(receiverVar.PossibleTypes) != 2 {
		t.Errorf("expected 2 possible concrete types, but got %d", len(receiverVar.PossibleTypes))
		for pt := range receiverVar.PossibleTypes {
			t.Logf("  possible type: %s", pt)
		}
	}

	foundTypes := make(map[string]bool)
	for pt := range receiverVar.PossibleTypes {
		foundTypes[pt] = true
	}

	if !foundTypes["example.com/me.*Dog"] {
		t.Errorf("did not find *Dog in possible concrete types")
	}
	if !foundTypes["example.com/me.*Cat"] {
		t.Errorf("did not find *Cat in possible concrete types")
	}
}

// findPkg is a helper to find a package by name.
func findPkg(pkgs []*goscan.Package, name string) *goscan.Package {
	for _, p := range pkgs {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func TestEval_InterfaceMethodCall_UndefinedButAllowed(t *testing.T) {
	code := `
package main

// sideEffect is a function that will be called as an argument
// to an undefined interface method. We use it to check that arguments
// are still evaluated.
func sideEffect() int {
	return 1
}

// Runner has a Run method, but not a Stop method.
type Runner interface {
	Run()
}

// Do calls both a defined (Run) and an undefined (Stop) method.
func Do(r Runner) {
	r.Run()
	r.Stop(sideEffect()) // This would cause an error without the patch.
}

func main() {
	Do(nil)
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var sideEffectCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		// Create an evaluator with a policy that allows scanning the current package.
		eval := New(s, s.Logger, nil, func(path string) bool {
			return path == pkg.ImportPath
		})

		// Register an intrinsic for the sideEffect function to track its call.
		sideEffectKey := fmt.Sprintf("%s.sideEffect", pkg.ImportPath)
		eval.RegisterIntrinsic(sideEffectKey, func(ctx context.Context, args ...object.Object) object.Object {
			sideEffectCalled = true
			return &object.Integer{Value: 1} // Return a value consistent with the function signature
		})

		// Evaluate the whole file to populate functions etc.
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		// Get the main function to start evaluation from.
		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc := mainFuncObj.(*object.Function)

		// Apply the main function.
		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			// We expect no evaluation error. The original code would produce one.
			return fmt.Errorf("evaluation failed unexpectedly: %s", err.Error())
		}
		return nil
	}

	// Run the test.
	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if !sideEffectCalled {
		t.Errorf("expected sideEffect() to be called, but it was not")
	}
}

func TestEval_InterfaceMethodCall_UndefinedWithIntrinsic(t *testing.T) {
	code := `
package main

// Runner has a Run method, but not a Stop method.
type Runner interface {
	Run()
}

// Do calls both a defined (Run) and an undefined (Stop) method.
func Do(r Runner) {
	r.Run()
	r.Stop()
}

func main() {
	Do(nil)
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var stopIntrinsicCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, func(path string) bool {
			return path == pkg.ImportPath
		})

		// Register an intrinsic for the UNDEFINED method.
		// The key format is `(pkgpath.TypeName).MethodName`
		stopKey := fmt.Sprintf("(%s.Runner).Stop", pkg.ImportPath)
		eval.RegisterIntrinsic(stopKey, func(ctx context.Context, args ...object.Object) object.Object {
			stopIntrinsicCalled = true
			return nil
		})

		// Evaluate the whole file to populate functions etc.
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		// Get the main function to start evaluation from.
		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc := mainFuncObj.(*object.Function)

		// Apply the main function.
		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed unexpectedly: %s", err.Error())
		}
		return nil
	}

	// Run the test.
	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if !stopIntrinsicCalled {
		t.Errorf("expected intrinsic for undefined method Stop() to be called, but it was not")
	}
}

func TestEval_InterfaceMethodCall_UndefinedAndContinue(t *testing.T) {
	code := `
package main

func cont() {} // This function call should be reached.

// Runner has a Run method, but not a Stop method.
type Runner interface {
	Run()
}

// Do calls an undefined method and then a defined function.
func Do(r Runner) {
	r.Stop()
	cont()
}

func main() {
	Do(nil)
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var contIntrinsicCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, func(path string) bool {
			return path == pkg.ImportPath
		})

		// Register an intrinsic for the `cont` function to see if it's called.
		contKey := fmt.Sprintf("%s.cont", pkg.ImportPath)
		eval.RegisterIntrinsic(contKey, func(ctx context.Context, args ...object.Object) object.Object {
			contIntrinsicCalled = true
			return nil
		})

		// Evaluate the whole file to populate functions etc.
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		// Get the main function to start evaluation from.
		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc := mainFuncObj.(*object.Function)

		// Apply the main function.
		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed unexpectedly: %s", err.Error())
		}
		return nil
	}

	// Run the test.
	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	if !contIntrinsicCalled {
		t.Errorf("expected intrinsic for cont() to be called, but it was not")
	}
}

// countingHandler is a simple slog.Handler that counts log records matching a specific message and level.
type countingHandler struct {
	mu      sync.Mutex
	count   int
	msg     string
	level   slog.Level
	handler slog.Handler
}

func newCountingHandler(msg string, level slog.Level, underlying slog.Handler) *countingHandler {
	return &countingHandler{
		msg:     msg,
		level:   level,
		handler: underlying,
	}
}

func (h *countingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *countingHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level == h.level && r.Message == h.msg {
		h.mu.Lock()
		h.count++
		h.mu.Unlock()
	}
	return h.handler.Handle(ctx, r)
}

func (h *countingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &countingHandler{
		handler: h.handler.WithAttrs(attrs),
		msg:     h.msg,
		level:   h.level,
		mu:      sync.Mutex{},
	}
}

func (h *countingHandler) WithGroup(name string) slog.Handler {
	return &countingHandler{
		handler: h.handler.WithGroup(name),
		msg:     h.msg,
		level:   h.level,
		mu:      sync.Mutex{},
	}
}

func (h *countingHandler) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count
}

func TestEval_InterfaceMethodCall_SyntheticMethodIsCached(t *testing.T) {
	code := `
package main

type Runner interface {
	Run()
}

// Do calls an undefined method twice.
func Do(r Runner) {
	r.Stop()
	r.Stop()
}

func main() {
	Do(nil)
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": code,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// 1. Set up the counting logger
	handler := newCountingHandler(
		"undefined method on interface, creating synthetic method",
		slog.LevelWarn,
		slog.NewTextHandler(io.Discard, nil), // Use io.Discard to not print logs during tests
	)
	logger := slog.New(handler)

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		// 2. Use the logger with the counting handler
		eval := New(s, logger, nil, func(path string) bool {
			return path == pkg.ImportPath
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc := mainFuncObj.(*object.Function)

		result := eval.Apply(ctx, mainFunc, []object.Object{}, pkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("evaluation failed unexpectedly: %s", err.Error())
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}

	// 3. Assert that the synthetic method was created only once.
	if count := handler.Count(); count != 1 {
		t.Errorf("expected synthetic method to be created once, but it was created %d times", count)
	}
}
