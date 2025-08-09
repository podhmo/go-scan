package minigo2

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo2/loader"
	"github.com/podhmo/go-scan/scanner"
)

// mockScanner is a mock implementation of the evaluator.Scanner interface for testing.
type mockScanner struct {
	scanCount int32
}

func (m *mockScanner) FindSymbolInPackage(ctx context.Context, pkgPath, symbolName string) (*goscan.Package, error) {
	atomic.AddInt32(&m.scanCount, 1)

	// if symbolName is empty, it's a request to load the whole package (for dot import).
	if symbolName == "" {
		return &goscan.Package{
			Name:       "fakelib",
			ImportPath: pkgPath,
			Constants: []*scanner.ConstantInfo{
				{Name: "MockConstant", Value: `"hello"`},
			},
			Functions: []*scanner.FunctionInfo{
				{Name: "DoSomething"},
			},
		}, nil
	}

	// Default mock behavior: return a simple package info for a single symbol.
	return &goscan.Package{
		Name:       "fakelib",
		ImportPath: pkgPath,
		Functions: []*scanner.FunctionInfo{
			{Name: symbolName},
		},
	}, nil
}

func TestInterpreter_PackageCache(t *testing.T) {
	// This script simulates two different "files" or scopes that both import and use the same package.
	script := `
package main

import "example.com/fakelib"
import "example.com/fakelib" // a second time

var x = fakelib.DoSomething()
var y = fakelib.DoSomething()
`

	// 1. Setup the interpreter with our mock scanner.
	interpreter, err := NewInterpreter()
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}

	mock := &mockScanner{}
	interpreter.loader = loader.New(mock, interpreter.packages)

	// 2. Execute the script.
	_, err = interpreter.Eval(context.Background(), Options{
		Source:   []byte(script),
		Filename: "test.mgo",
	})
	if err != nil {
		t.Fatalf("Eval() returned an unexpected error: %v", err)
	}

	// 3. Assert that the scanner was only called once.
	// The PackageCache in the interpreter should have cached the "example.com/fakelib"
	// package after the first access (`fakelib.DoSomething()`), so the second access
	// should not trigger another scan.
	scanCalls := atomic.LoadInt32(&mock.scanCount)
	if scanCalls != 1 {
		t.Errorf("Expected scanner to be called once, but it was called %d times", scanCalls)
	}
}
