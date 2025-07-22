package main

import (
	"context"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate(t *testing.T) {
	// 1. Setup: Create a temporary directory with the source files.
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": `module testing`,
		"models.go": "package models\n\ntype Event interface {\n\tEvent()\n}\n\n// @deriving:unmarshal\ntype EventContainer struct {\n\tEvent Event `json:\"event\"`\n}\n\ntype MessageEvent struct {\n\tMessage string `json:\"message\"`\n}\nfunc (e *MessageEvent) Event() {}\n\ntype ReactionEvent struct {\n\tReaction string `json:\"reaction\"`\n}\nfunc (e *ReactionEvent) Event() {}\n",
	})
	defer cleanup()

	// 2. Action: Run the generation logic using scantest.Run
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		for _, pkg := range pkgs {
			if err := Generate(ctx, s, pkg); err != nil {
				return err
			}
		}
		return nil
	}

	result, err := scantest.Run(t, dir, []string{"models.go"}, action)
	require.NoError(t, err, "scantest.Run should not fail")
	require.NotNil(t, result, "scantest.Run should produce a result")
	require.Contains(t, result.Outputs, "models_deriving.go", "output should contain the generated file")

	// 3. Assert: Check the content of the generated file.
	generatedCode := string(result.Outputs["models_deriving.go"])

	assert.Contains(t, generatedCode, `func (s *EventContainer) UnmarshalJSON(data []byte) error {`)
	assert.Contains(t, generatedCode, `switch discriminatorDoc.Type {`)
	assert.Contains(t, generatedCode, `case "message":`)
	assert.Contains(t, generatedCode, `var content *MessageEvent`)
	assert.Contains(t, generatedCode, `case "reaction":`)
	assert.Contains(t, generatedCode, `var content *ReactionEvent`)
}
