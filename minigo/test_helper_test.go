package minigo_test

import (
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo"
)

func newTestInterpreter(t *testing.T, opts ...minigo.Option) *minigo.Interpreter {
	t.Helper()
	s, err := goscan.New(goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}
	interp, err := minigo.NewInterpreter(s, opts...)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %v", err)
	}
	return interp
}
