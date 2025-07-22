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
		"models.go": "package models\n\n// @derivng:binding\ntype Input struct {\n\tName string `in:\"query\" query:\"name\"`\n\tAge  int    `in:\"query\" query:\"age\" required:\"true\"`\n}\n\n// @derivng:binding in:\"body\"\ntype Person struct {\n\tName string `json:\"name\"`\n\tAge  int    `json:\"age\"`\n}\n",
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

	// Check imports
	assert.Contains(t, generatedCode, `import (`, "should have an import block")
	assert.Contains(t, generatedCode, `"net/http"`, "should import net/http")
	assert.Contains(t, generatedCode, `"github.com/podhmo/go-scan/examples/derivingbind/parser"`, "should import parser")
	assert.Contains(t, generatedCode, `"github.com/podhmo/go-scan/examples/derivingbind/binding"`, "should import binding")
	assert.Contains(t, generatedCode, `"errors"`, "should import errors")

	// Check Input struct Bind method
	assert.Contains(t, generatedCode, `func (s *Input) Bind(req *http.Request, pathVar func(string) string) error {`, "should define Bind method for Input")
	assert.Contains(t, generatedCode, `binding.One(b, &s.Age, binding.Query, "age", parser.Int, binding.Required)`, "should parse age from query")
	assert.Contains(t, generatedCode, `binding.One(b, &s.Name, binding.Query, "name", parser.String, binding.Optional)`, "should parse name from query")

	// Check Person struct Bind method
	assert.Contains(t, generatedCode, `func (s *Person) Bind(req *http.Request, pathVar func(string) string) error {`, "should define Bind method for Person")
	assert.Contains(t, generatedCode, `if decErr := json.NewDecoder(req.Body).Decode(s); decErr != nil {`, "should decode body for Person")

	t.Logf("Generated code:\n%s", generatedCode)
}
