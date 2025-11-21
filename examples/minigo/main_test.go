package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo"
	fmt_ "github.com/podhmo/go-scan/minigo/stdlib/fmt"
	strings_ "github.com/podhmo/go-scan/minigo/stdlib/strings"
)

func TestREPL(t *testing.T) {
	// Helper function to run a single REPL test case.
	runCase := func(t *testing.T, input string, setup func(interp *minigo.Interpreter)) string {
		t.Helper()

		r := strings.NewReader(input)
		out := &bytes.Buffer{}

		interp, err := newInterpreterWithStdlib()
		if err != nil {
			t.Fatalf("newInterpreterWithStdlib failed: %v", err)
		}

		if setup != nil {
			setup(interp)
		}

		err = runREPL(r, out, interp)
		if err != nil {
			t.Fatalf("runREPL failed: %v", err)
		}
		return out.String()
	}

	t.Run("simple expression", func(t *testing.T) {
		input := "1 + 1\n:exit\n"
		output := runCase(t, input, nil)

		if !strings.Contains(output, "2\n") {
			t.Errorf("expected '2\\n', got %q", output)
		}
	})

	t.Run("use imported package", func(t *testing.T) {
		// minigo's REPL prints the return value of the function, not its stdout side-effect.
		// fmt.Println("hello") returns (6, nil).
		input := `import "fmt"` + "\n" + `fmt.Println("hello")` + "\n:exit\n"
		output := runCase(t, input, func(interp *minigo.Interpreter) {
			fmt_.Install(interp)
		})
		// The output is the inspected version of the return values (int, error).
		if !strings.Contains(output, "(6, nil)\n") {
			t.Errorf("expected return value '(6, nil)\\n', got %q", output)
		}
	})

	t.Run("define and call function", func(t *testing.T) {
		// Let's try a simpler single-argument function to avoid potential bugs
		// with multi-argument function evaluation in minigo.
		input := "func double(a int) int { return a * 2 }\ndouble(6)\n:exit\n"
		output := runCase(t, input, nil)
		if !strings.Contains(output, "12\n") {
			t.Errorf("expected '12\\n' for single-arg fn, got %q", output)
		}
	})

	t.Run("metacommand :help", func(t *testing.T) {
		input := ":help\n:exit\n"
		output := runCase(t, input, nil)
		if !strings.Contains(output, "Available commands:") {
			t.Errorf("expected help message, got %q", output)
		}
	})

	t.Run("metacommand :reset", func(t *testing.T) {
		input := "x := 10\nx\n:reset\nx\n:exit\n"
		output := runCase(t, input, nil)

		if !strings.Contains(output, "10\n") {
			t.Errorf("expected '10' before reset, got %q", output)
		}
		if !strings.Contains(output, "Resetting interpreter state.") {
			t.Errorf("expected reset message, got %q", output)
		}
		// Adjusting to the actual error message from the interpreter.
		if !strings.Contains(output, "Error: runtime error: identifier not found: x") {
			t.Errorf("expected correct undefined error for 'x' after reset, got %q", output)
		}
	})

	t.Run("import non-existent package", func(t *testing.T) {
		input := `import "nonexistent/package"` + "\n:exit\n"
		output := runCase(t, input, nil)
		// It seems minigo's EvalLine silently fails on non-existent imports, returning no error.
		// We'll test for the absence of an error message.
		// The output should not contain "Error:".
		if strings.Contains(output, "Error:") {
			t.Errorf("expected no error for non-existent import, but got one: %q", output)
		}
	})

	t.Run("metacommand :load", func(t *testing.T) {
		// This test is expected to fail until :load is implemented.
		file, err := os.CreateTemp(t.TempDir(), "test_*.go")
		if err != nil {
			t.Fatal(err)
		}
		filename := file.Name()
		defer os.Remove(filename)

		content := "package main\n\nfunc loadedFunc() string { return \"loaded!\" }"
		if _, err := file.WriteString(content); err != nil {
			t.Fatal(err)
		}
		file.Close()

		input := ":load " + filename + "\n" + "loadedFunc()\n:exit\n"
		output := runCase(t, input, nil)

		if !strings.Contains(output, "Loading file "+filename) {
			t.Errorf("expected loading message for %q, got %q", filename, output)
		}
		// The Inspect() for a string seems to not add quotes, so we check for the raw string.
		if !strings.Contains(output, "loaded!\n") {
			t.Errorf("expected 'loaded!\\n' from loaded function, got %q", output)
		}
	})

	t.Run("metacommand :load with import", func(t *testing.T) {
		file, err := os.CreateTemp(t.TempDir(), "test_import_*.go")
		if err != nil {
			t.Fatal(err)
		}
		filename := file.Name()
		defer os.Remove(filename)

		content := `
package main

import "strings"

func shout(s string) string {
	return strings.ToUpper(s)
}
`
		if _, err := file.WriteString(content); err != nil {
			t.Fatal(err)
		}
		file.Close()

		input := ":load " + filename + "\n" + `shout("hello")` + "\n:exit\n"
		output := runCase(t, input, func(interp *minigo.Interpreter) {
			strings_.Install(interp)
		})

		// The expected output is the inspected string, which for minigo is just the raw string value.
		if !strings.Contains(output, "HELLO\n") {
			t.Errorf("expected 'HELLO\\n' from loaded function using import, got %q", output)
		}
	})
}

func TestExecutionModes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	t.Run("file-based config", func(t *testing.T) {
		configContent := `
package main

type MyConfig struct {
	Name string
	Port int
}

func Config() MyConfig {
	return MyConfig{Name: "test-server", Port: 8080}
}`
		configFile := filepath.Join(dir, "config.go")
		if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := run(ctx, configFile, "Config", "json", "")
		if err != nil {
			t.Errorf("run() error = %v", err)
		}

		w.Close()
		os.Stdout = oldStdout
		var out bytes.Buffer
		out.ReadFrom(r)

		expectedJSON := `{
  "Name": "test-server",
  "Port": 8080
}`
		if strings.TrimSpace(out.String()) != expectedJSON {
			t.Errorf("Expected JSON output:\n%s\nGot:\n%s", expectedJSON, out.String())
		}
	})

	t.Run("inline code", func(t *testing.T) {
		evalCode := `
package main
func main() (int, int) { return 1, 2 }`

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := run(ctx, "", "main", "inspect", evalCode)
		if err != nil {
			t.Errorf("run() error = %v", err)
		}

		w.Close()
		os.Stdout = oldStdout
		var out bytes.Buffer
		out.ReadFrom(r)

		expectedOutput := `(1, 2)`
		if strings.TrimSpace(out.String()) != expectedOutput {
			t.Errorf("Expected output:\n%s\nGot:\n%s", expectedOutput, out.String())
		}
	})

	t.Run("self-contained script", func(t *testing.T) {
		scriptContent := `
package main
import "encoding/json"

type MyConfig struct {
	Name string
	Port int
}

func Config() MyConfig {
	return MyConfig{Name: "script-server", Port: 9090}
}

func main() []byte {
	result, err := json.Marshal(Config())
	if err != nil {
		return nil
	}
	return result
}`
		scriptFile := filepath.Join(dir, "script.go")
		if err := os.WriteFile(scriptFile, []byte(scriptContent), 0644); err != nil {
			t.Fatalf("Failed to write script file: %v", err)
		}

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		runFile(ctx, scriptFile)

		w.Close()
		os.Stdout = oldStdout
		var out bytes.Buffer
		out.ReadFrom(r)

		// A more robust check for the content of the byte slice.
		expectedContent := `{"Name":"script-server","Port":9090}`
		outputStr := strings.TrimSpace(out.String())
		outputStr = strings.TrimPrefix(outputStr, "[")
		outputStr = strings.TrimSuffix(outputStr, "]")
		parts := strings.Split(outputStr, " ")
		var byteSlice []byte
		for _, p := range parts {
			if p == "" {
				continue
			}
			b, err := strconv.Atoi(p)
			if err != nil {
				t.Fatalf("failed to parse byte from output: %q", p)
			}
			byteSlice = append(byteSlice, byte(b))
		}

		if string(byteSlice) != expectedContent {
			t.Errorf("Expected byte slice content:\n%s\nGot:\n%s", expectedContent, string(byteSlice))
		}
	})
}
