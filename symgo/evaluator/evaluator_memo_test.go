package evaluator_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
)

func TestMemoization_SkipsReanalysisOfSharedFunctions(t *testing.T) {
	ctx := context.Background()
	source := `
package main

func internal() {}

// Shared is called by two different functions.
// Due to memoization, its body should only be evaluated once.
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
	f, s := scantest.NewScanned(t, "main", source)

	// Create a single interpreter instance to be used for all analyses.
	// This simulates how find-orphans uses the interpreter.
	interp, err := symgo.New(s.Scanner)
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}

	// We'll track how many times the `internal` function (called by `Shared`)
	// is executed by registering an intrinsic.
	internalExecutionCount := 0
	interp.RegisterIntrinsic("example.com/main.internal", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		internalExecutionCount++
		return nil
	})

	// Load the package into the interpreter's context.
	// This populates the environment with function definitions.
	if _, err := interp.Eval(ctx, f.File, f.PackageInfo); err != nil {
		t.Fatalf("failed to eval package: %+v", err)
	}

	mainPkg := f.PackageInfo
	if mainPkg == nil {
		t.Fatal("main package info is nil")
	}

	// 1. Analyze the first caller. This should trigger the analysis of Shared and, subsequently, internal.
	callerA, ok := interp.FindObjectInPackage(ctx, "example.com/main", "CallerA")
	if !ok {
		t.Fatal("could not find CallerA")
	}
	if _, err := interp.Apply(ctx, callerA, nil, mainPkg); err != nil {
		t.Fatalf("error applying CallerA: %+v", err)
	}

	// After the first analysis, the internal function should have been executed once.
	if internalExecutionCount != 1 {
		t.Errorf("after analyzing CallerA, expected internal() to be executed once, but was %d", internalExecutionCount)
	}

	// 2. Analyze the second caller.
	callerB, ok := interp.FindObjectInPackage(ctx, "example.com/main", "CallerB")
	if !ok {
		t.Fatal("could not find CallerB")
	}
	if _, err := interp.Apply(ctx, callerB, nil, mainPkg); err != nil {
		t.Fatalf("error applying CallerB: %+v", err)
	}

	// 3. Assert that the internal function was NOT executed again.
	// The call to `applyFunction(Shared)` should have been memoized, preventing its body
	// from being re-evaluated and the call to `internal()` from being re-traced.
	if internalExecutionCount != 1 {
		t.Errorf("after analyzing CallerB, expected internalExecutionCount to remain 1 due to memoization, but it was %d", internalExecutionCount)
	}
}
