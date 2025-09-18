package evaluator_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
)

func TestMemoization_IsConfigurableAndWorksAsExpected(t *testing.T) {
	ctx := context.Background()
	source := `
package main

func internal() {}

// Shared is called by two different functions.
func Shared() {
	internal()
}

// CallerA is the first entry point.
func CallerA() {
	Shared()
}

// CallerB is the second entry point.
func CallerB() {
	Shared()
}
`
	runTest := func(t *testing.T, memoize bool) int {
		f, s := scantest.NewScanned(t, "main", source)

		var interpOpts []symgo.Option
		// Only enable memoization if the test case requires it.
		if memoize {
			interpOpts = append(interpOpts, symgo.WithMemoization(true))
		}

		interp, err := symgo.New(s.Scanner, interpOpts...)
		if err != nil {
			t.Fatalf("failed to create interpreter: %+v", err)
		}

		internalExecutionCount := 0
		interp.RegisterIntrinsic("example.com/main.internal", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			internalExecutionCount++
			return nil
		})

		if _, err := interp.Eval(ctx, f.File, f.PackageInfo); err != nil {
			t.Fatalf("failed to eval package: %+v", err)
		}

		mainPkg := f.PackageInfo
		if mainPkg == nil {
			t.Fatal("main package info is nil")
		}

		// Analyze both callers
		callerA, _ := interp.FindObjectInPackage(ctx, "example.com/main", "CallerA")
		interp.Apply(ctx, callerA, nil, mainPkg)

		callerB, _ := interp.FindObjectInPackage(ctx, "example.com/main", "CallerB")
		interp.Apply(ctx, callerB, nil, mainPkg)

		return internalExecutionCount
	}

	t.Run("memoization disabled (default)", func(t *testing.T) {
		count := runTest(t, false)
		// Without memoization, Shared() is evaluated twice, so internal() is called twice.
		if count != 2 {
			t.Errorf("expected internal() to be executed 2 times, but was %d", count)
		}
	})

	t.Run("memoization enabled", func(t *testing.T) {
		count := runTest(t, true)
		// With memoization, the second call to Shared() is skipped, so internal() is only called once.
		if count != 1 {
			t.Errorf("expected internal() to be executed 1 time, but was %d", count)
		}
	})
}
