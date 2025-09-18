package evaluator_test

import (
	"context"
	"sync/atomic"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
)

func TestMemoization_WithScantest(t *testing.T) {
	// This test sets up a scenario where two functions (UseA, UseB) both call
	// a factory function (NewService). We want to verify that when memoization
	// is enabled, the body of NewService is only executed once.
	//
	// To do this without interfering with the memoization of NewService itself,
	// we add a Tally() function that is called from within NewService. We then
	// register an intrinsic on Tally() to count how many times it's called.
	sourceFiles := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main

// Tally is a hook for our test to count function executions.
func Tally() {}

type Service struct{}
func (s *Service) DoA() {}
func (s *Service) DoB() {}

// NewService is the factory function we expect to be memoized.
func NewService() *Service {
	Tally() // This call will be intercepted by our intrinsic.
	return &Service{}
}

func UseA() {
	service := NewService()
	service.DoA()
}

func UseB() {
	service := NewService()
	service.DoB()
}
`,
	}

	var callCount atomic.Int64

	t.Run("memoization enabled", func(t *testing.T) {
		callCount.Store(0)
		dir, cleanup := scantest.WriteFiles(t, sourceFiles)
		defer cleanup()

		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			interpreter, err := symgo.NewInterpreter(s,
				symgo.WithPrimaryAnalysisScope("example.com/me"),
				symgo.WithMemoization(true),
			)
			if err != nil {
				return err
			}

			// Register an intrinsic on Tally() to count calls.
			interpreter.RegisterIntrinsic("example.com/me.Tally", func(ctx context.Context, eval *symgo.Interpreter, args []symgo.Object) symgo.Object {
				callCount.Add(1)
				return nil // Tally returns nothing.
			})

			// Analyze both entry points.
			entrypoints := []string{"UseA", "UseB"}
			for _, name := range entrypoints {
				fn, ok := interpreter.FindObjectInPackage(ctx, "example.com/me", name)
				if !ok {
					t.Errorf("could not find entry point %q", name)
					continue
				}
				if _, err := interpreter.Apply(ctx, fn, nil, nil); err != nil {
					t.Errorf("error analyzing entry point %q: %v", name, err)
				}
			}
			return nil
		}

		if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
			t.Fatalf("scantest.Run failed: %v", err)
		}

		if got := callCount.Load(); got != 1 {
			t.Errorf("expected Tally to be called once with memoization, but got %d", got)
		}
	})

	t.Run("memoization disabled", func(t *testing.T) {
		callCount.Store(0)
		dir, cleanup := scantest.WriteFiles(t, sourceFiles)
		defer cleanup()

		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			interpreter, err := symgo.NewInterpreter(s,
				symgo.WithPrimaryAnalysisScope("example.com/me"),
				symgo.WithMemoization(false), // Explicitly disabled
			)
			if err != nil {
				return err
			}

			// Register an intrinsic on Tally() to count calls.
			interpreter.RegisterIntrinsic("example.com/me.Tally", func(ctx context.Context, eval *symgo.Interpreter, args []symgo.Object) symgo.Object {
				callCount.Add(1)
				return nil
			})

			// Analyze both entry points.
			entrypoints := []string{"UseA", "UseB"}
			for _, name := range entrypoints {
				fn, ok := interpreter.FindObjectInPackage(ctx, "example.com/me", name)
				if !ok {
					t.Errorf("could not find entry point %q", name)
					continue
				}
				if _, err := interpreter.Apply(ctx, fn, nil, nil); err != nil {
					t.Errorf("error analyzing entry point %q: %v", name, err)
				}
			}
			return nil
		}

		if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
			t.Fatalf("scantest.Run failed: %v", err)
		}

		if got := callCount.Load(); got != 2 {
			t.Errorf("expected Tally to be called twice without memoization, but got %d", got)
		}
	})
}
