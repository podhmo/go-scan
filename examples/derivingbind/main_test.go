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
		"models.go": "package models\n\n// @deriving:binding\ntype Input struct {\n\tName string `in:\"query\" query:\"name\"`\n\tAge  int    `in:\"query\" query:\"age\" required:\"true\"`\n}\n\n// @deriving:binding in:\"body\"\ntype Person struct {\n\tName string `json:\"name\"`\n\tAge  int    `json:\"age\"`\n}\n",
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

	// Check imports
	if !strings.Contains(generatedCode, `import (`) {
		t.Errorf("should have an import block")
	}
	if !strings.Contains(generatedCode, `"net/http"`) {
		t.Errorf("should import net/http")
	}
	if !strings.Contains(generatedCode, `"github.com/podhmo/go-scan/examples/derivingbind/parser"`) {
		t.Errorf("should import parser")
	}
	if !strings.Contains(generatedCode, `"github.com/podhmo/go-scan/examples/derivingbind/binding"`) {
		t.Errorf("should import binding")
	}
	if !strings.Contains(generatedCode, `"errors"`) {
		t.Errorf("should import errors")
	}

	// Check Input struct Bind method
	if !strings.Contains(generatedCode, `func (s *Input) Bind(req *http.Request, pathVar func(string) string) error {`) {
		t.Errorf("should define Bind method for Input")
	}
	if !strings.Contains(generatedCode, `binding.One(b, &s.Age, binding.Query, "age", parser.Int, binding.Required)`) {
		t.Errorf("should parse age from query")
	}
	if !strings.Contains(generatedCode, `binding.One(b, &s.Name, binding.Query, "name", parser.String, binding.Optional)`) {
		t.Errorf("should parse name from query")
	}

	// Check Person struct Bind method
	if !strings.Contains(generatedCode, `func (s *Person) Bind(req *http.Request, pathVar func(string) string) error {`) {
		t.Errorf("should define Bind method for Person")
	}
	if !strings.Contains(generatedCode, `if decErr := json.NewDecoder(req.Body).Decode(s); decErr != nil {`) {
		t.Errorf("should decode body for Person")
	}

	t.Logf("Generated code:\n%s", generatedCode)
}
