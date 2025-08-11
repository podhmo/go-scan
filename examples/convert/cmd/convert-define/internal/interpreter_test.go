package internal

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
)

func TestRunner_Success(t *testing.T) {
	// 1. Setup a temporary directory representing a Go module.
	tmpdir, err := os.MkdirTemp("", "interpreter-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Create subdirectories for packages
	modelsDir := filepath.Join(tmpdir, "models")
	convutilDir := filepath.Join(tmpdir, "convutil")
	defineDir := filepath.Join(tmpdir, "define") // Mock define package
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(convutilDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(defineDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 2. Write the necessary Go files.
	write(t, filepath.Join(tmpdir, "go.mod"), "module example.com/test\ngo 1.22")
	write(t, filepath.Join(modelsDir, "user.go"), `
package models
type User struct {
	ID int
	Name string
}`)
	write(t, filepath.Join(convutilDir, "util.go"), `
package convutil
import "time"
func TimeToString(t time.Time) string { return t.String() }`)

	// The actual `define` package has an `any` type, which is fine for the script
	// to be valid Go, but our interpreter doesn't need the real implementation.
	// We just need the package to exist so the import resolves.
	write(t, filepath.Join(defineDir, "define.go"), `
package define
type Mapping struct{}
func Convert(src, dst any, m Mapping) {}
func Rule(fn any) {}
func Mapping(fn any) Mapping { return Mapping{} }`)

	// The main script to be parsed.
	defineScript := `
package main
import (
	"example.com/test/convutil"
	"example.com/test/define"
	"example.com/test/models"
)
func main() {
	define.Rule(convutil.TimeToString)
	define.Convert(models.User{}, models.User{}, define.Mapping(nil))
}`
	scriptPath := filepath.Join(tmpdir, "define.go")
	write(t, scriptPath, defineScript)

	// 3. Capture log output.
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// 4. Create and run the interpreter runner.
	runner, err := NewRunner(goscan.WithWorkDir(tmpdir))
	if err != nil {
		t.Fatalf("NewRunner() failed: %v", err)
	}

	// Override the path to our mock define package
	runner.interp.RegisterSpecial("example.com/test/define.Convert", runner.handleConvert)
	runner.interp.RegisterSpecial("example.com/test/define.Rule", runner.handleRule)
	runner.interp.RegisterSpecial("example.com/test/define.Mapping", runner.handleMapping)

	if err := runner.Run(context.Background(), scriptPath); err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	// 5. Assert that the correct information was extracted.
	logOutput := logBuf.String()
	t.Logf("Log output:\n%s", logOutput)

	// Check Rule handling
	if !strings.Contains(logOutput, `msg="resolving rule function" pkg=example.com/test/convutil func=TimeToString`) {
		t.Error("did not resolve rule function correctly")
	}
	if !strings.Contains(logOutput, `msg="found rule" src=time.Time dst=string`) {
		t.Error("did not find correct signature for rule function")
	}

	// Check Convert handling
	if !strings.Contains(logOutput, `msg="found conversion pair" src=User dst=User`) {
		t.Error("did not find correct conversion pair")
	}
	if !strings.Contains(logOutput, `msg="  - src field" name=ID type=int`) {
		t.Error("did not find ID field in source struct")
	}
	if !strings.Contains(logOutput, `msg="  - src field" name=Name type=string`) {
		t.Error("did not find Name field in source struct")
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}
