package main

import (
	"context"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
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
	if err != nil {
		t.Fatalf("scantest.Run should not fail: %v", err)
	}
	if result == nil {
		t.Fatal("scantest.Run should produce a result")
	}
	if _, ok := result.Outputs["models_deriving.go"]; !ok {
		t.Fatal("output should contain the generated file")
	}

	// 3. Assert: Check the content of the generated file.
	generatedCode := string(result.Outputs["models_deriving.go"])

	if !strings.Contains(generatedCode, `func (s *EventContainer) UnmarshalJSON(data []byte) error {`) {
		t.Errorf("expected to contain UnmarshalJSON method")
	}
	if !strings.Contains(generatedCode, `switch discriminatorDoc.Type {`) {
		t.Errorf("expected to contain switch statement")
	}
	if !strings.Contains(generatedCode, `case "message":`) {
		t.Errorf("expected to contain case for message")
	}
	if !strings.Contains(generatedCode, `var content *MessageEvent`) {
		t.Errorf("expected to contain var for MessageEvent")
	}
	if !strings.Contains(generatedCode, `case "reaction":`) {
		t.Errorf("expected to contain case for reaction")
	}
	if !strings.Contains(generatedCode, `var content *ReactionEvent`) {
		t.Errorf("expected to contain var for ReactionEvent")
	}
}
