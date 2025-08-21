package main

import (
	"context"
	"fmt"
	"os"

	"github.com/podhmo/go-scan/minigo"
)

// Pattern defines a declarative analysis pattern loaded from a script.
type Pattern struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	ArgIndex    int    `json:"argIndex"`
	ContentType string `json:"contentType,omitempty"`
}

// LoadPatterns reads a Go script, evaluates it to define a 'Patterns' variable,
// then retrieves and parses that variable into a slice of Pattern structs.
func LoadPatterns(filepath string) ([]Pattern, error) {
	b, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read patterns file %q: %w", filepath, err)
	}

	interp, err := minigo.NewInterpreter()
	if err != nil {
		return nil, fmt.Errorf("failed to create minigo interpreter: %w", err)
	}

	// First, evaluate the file content to define the Patterns variable in the interpreter's global scope.
	if _, err := interp.EvalString(string(b)); err != nil {
		return nil, fmt.Errorf("failed to evaluate patterns definition file %q: %w", filepath, err)
	}

	// Now, evaluate an expression to retrieve the variable's value.
	obj, err := interp.EvalLine(context.Background(), "Patterns")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve Patterns variable from %q: %w", filepath, err)
	}

	result := &minigo.Result{Value: obj}
	var patterns []Pattern
	if err := result.As(&patterns); err != nil {
		return nil, fmt.Errorf("failed to decode Patterns variable from script %q: %w", filepath, err)
	}

	return patterns, nil
}
