package main

import (
	"go/ast"
	"testing"
)

func TestRun(t *testing.T) {
	pkg, err := run("github.com/podhmo/go-scan/examples/minigo-gen-bindings/testdata/mypkg")
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	var exportedFuncs []string
	for _, fn := range pkg.Functions {
		if ast.IsExported(fn.Name) {
			exportedFuncs = append(exportedFuncs, fn.Name)
		}
	}

	if len(exportedFuncs) != 1 {
		t.Errorf("expected 1 exported function, got %d", len(exportedFuncs))
	}
	if exportedFuncs[0] != "ExportedFunc" {
		t.Errorf("expected function name ExportedFunc, got %s", exportedFuncs[0])
	}

	var exportedConsts []string
	for _, c := range pkg.Constants {
		if ast.IsExported(c.Name) {
			exportedConsts = append(exportedConsts, c.Name)
		}
	}

	if len(exportedConsts) != 1 {
		t.Errorf("expected 1 exported constant, got %d", len(exportedConsts))
	}
	if exportedConsts[0] != "ExportedConst" {
		t.Errorf("expected constant name ExportedConst, got %s", exportedConsts[0])
	}
}
