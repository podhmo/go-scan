package minigotest

import (
	"fmt"
	"go/token"

	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
	"github.com/podhmo/go-scan/scantest"
)

// Package represents a minigo package to be evaluated.
type Package struct {
	Name  string
	Files map[string]string
}

// Result provides access to the results of a script execution.
type Result struct {
	env *object.Environment
}

// Get retrieves a global variable by name from the script's environment.
func (r *Result) Get(name string) (any, bool) {
	return r.env.Get(name)
}

// Runner is a test helper for running minigo scripts in isolated module environments.
type Runner struct {
	scanner *scantest.Scanner
	fset    *token.FileSet
}

// NewRunner creates a new test runner.
func NewRunner() *Runner {
	fset := token.NewFileSet()
	return &Runner{
		scanner: scantest.NewScanner(fset, ""),
		fset:    fset,
	}
}

// AddModule creates a new Go module with the given content, making it available
// for the minigo interpreter to import.
func (r *Runner) AddModule(module, pkg, content string) error {
	return r.scanner.AddModule(module, pkg, content)
}

// Run evaluates the given minigo package and returns the global environment.
func (r *Runner) Run(pkg *Package) (*Result, error) {
	interp, err := minigo.NewInterpreter(r.scanner)
	if err != nil {
		return nil, fmt.Errorf("failed to create interpreter: %w", err)
	}

	for filename, content := range pkg.Files {
		if err := interp.LoadFile(filename, []byte(content)); err != nil {
			return nil, fmt.Errorf("failed to load file %s: %w", filename, err)
		}
	}

	if _, err := interp.Eval(nil); err != nil {
		return nil, fmt.Errorf("evaluation failed: %w", err)
	}

	return &Result{env: interp.GlobalEnvForTest()}, nil
}
