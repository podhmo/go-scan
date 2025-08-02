package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/scantest"

	goscan "github.com/podhmo/go-scan"
	bindgen "github.com/podhmo/go-scan/examples/derivingbind/gen"
	jsongen "github.com/podhmo/go-scan/examples/derivingjson/gen"
	"github.com/podhmo/go-scan/scanner"
)

func TestUnifiedGenerator(t *testing.T) {
	testCases := []struct {
		name              string
		input             map[string]string
		expectedToGenerate bool
		mustContain       []string
		mustNotContain    []string
	}{
		{
			name: "struct with both annotations",
			input: map[string]string{
				"go.mod": "module mytest",
				"models.go": `
package models
import "time"
// @deriving:unmarshal
// @derivng:binding in:"body"
type Event struct {
	ID        string    ` + "`json:\"id\"`" + `
	CreatedAt time.Time ` + "`json:\"createdAt\"`" + `
	Data      EventData ` + "`json:\"data\"`" + `
}
type EventData interface { isEventData() }
type UserCreated struct { UserID string }
func (UserCreated) isEventData() {}
`,
			},
			expectedToGenerate: true,
			mustContain: []string{
				"func (s *Event) UnmarshalJSON(data []byte) error",
				"func (s *Event) Bind(req *http.Request, pathVar func(string) string) error",
				"package models",
				`"encoding/json"`,
				`"net/http"`,
			},
		},
		{
			name: "struct with only json unmarshal annotation",
			input: map[string]string{
				"go.mod": "module mytest",
				"models.go": `
package models
import "time"
// @deriving:unmarshal
type Event struct {
	Data EventData ` + "`json:\"data\"`" + `
}
type EventData interface { isEventData() }
type UserCreated struct { UserID string }
func (UserCreated) isEventData() {}
`,
			},
			expectedToGenerate: true,
			mustContain: []string{
				"func (s *Event) UnmarshalJSON(data []byte) error",
			},
			mustNotContain: []string{
				"func (s *Event) Bind(req *http.Request, pathVar func(string) string) error",
			},
		},
		{
			name: "struct with only binding annotation",
			input: map[string]string{
				"go.mod": "module mytest",
				"models.go": `
package models
import "time"
// @derivng:binding in:"body"
type Event struct {
	Name string ` + "`json:\"name\"`" + `
}`,
			},
			expectedToGenerate: true,
			mustContain: []string{
				"func (s *Event) Bind(req *http.Request, pathVar func(string) string) error",
			},
			mustNotContain: []string{
				"func (s *Event) UnmarshalJSON(data []byte) error",
			},
		},
		{
			name: "struct with no annotations",
			input: map[string]string{
				"go.mod": "module mytest",
				"models.go": `
package models
type User struct {
	Name string ` + "`json:\"name\"`" + `
}`,
			},
			expectedToGenerate: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup: scantest creates a temporary directory with the input files
			dir, cleanup := scantest.WriteFiles(t, tc.input)
			defer cleanup()

			// Action: Define the generation logic to be run by scantest
			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*scanner.PackageInfo) error {
				if len(pkgs) != 1 {
					return fmt.Errorf("expected 1 package, got %d", len(pkgs))
				}
				pkgInfo := pkgs[0]

				generators := []GeneratorFunc{
					jsongen.Generate,
					bindgen.Generate,
				}

				importManager := goscan.NewImportManager(pkgInfo)
				var masterCode bytes.Buffer

				for _, generate := range generators {
					code, err := generate(ctx, s, pkgInfo, importManager)
					if err != nil {
						return fmt.Errorf("generator failed: %w", err)
					}
					if len(code) > 0 {
						masterCode.Write(code)
						masterCode.WriteString("\n\n")
					}
				}

				if masterCode.Len() == 0 {
					return nil // Nothing to generate
				}

				outputDir := goscan.NewPackageDirectory(pkgInfo.Path, pkgInfo.Name)
				goFile := goscan.GoFile{
					PackageName: pkgInfo.Name,
					Imports:     importManager.Imports(),
					CodeSet:     masterCode.String(),
				}

				outputFilename := fmt.Sprintf("%s_deriving.go", strings.ToLower(pkgInfo.Name))
				// scantest intercepts this call and captures the output in memory
				return outputDir.SaveGoFile(ctx, goFile, outputFilename)
			}

			// Execution: Run the test
			result, err := scantest.Run(t, dir, []string{"."}, action)
			if err != nil {
				t.Fatalf("scantest.Run failed: %v", err)
			}

			// Assertion
			generatedFileName := "models_deriving.go"
			if !tc.expectedToGenerate {
				if result != nil {
					if _, exists := result.Outputs[generatedFileName]; exists {
						t.Errorf("expected no file to be generated, but %s was created", generatedFileName)
					}
				}
				return // Test passed
			}

			if _, exists := result.Outputs[generatedFileName]; !exists {
				t.Fatalf("expected file %s to be generated, but it was not", generatedFileName)
			}

			generatedCode := string(result.Outputs[generatedFileName])
			for _, substr := range tc.mustContain {
				if !strings.Contains(generatedCode, substr) {
					t.Errorf("generated code does not contain expected string:\n---EXPECTED---\n%s\n---ACTUAL CODE---\n%s", substr, generatedCode)
				}
			}
			for _, substr := range tc.mustNotContain {
				if strings.Contains(generatedCode, substr) {
					t.Errorf("generated code contains unexpected string:\n---UNEXPECTED---\n%s\n---ACTUAL CODE---\n%s", substr, generatedCode)
				}
			}
		})
	}
}
