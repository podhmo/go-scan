package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindOrphans_LargeProject(t *testing.T) {
	// Create a temporary directory for our large test project
	tmpDir, err := os.MkdirTemp("", "large-project-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a go.mod file
	goModPath := filepath.Join(tmpDir, "go.mod")
	err = os.WriteFile(goModPath, []byte("module largeproject\n\ngo 1.22"), 0644)
	require.NoError(t, err)

	const fileCount = 100
	const funcsPerFile = 5

	// Generate a large number of files
	for i := 0; i < fileCount; i++ {
		pkgName := fmt.Sprintf("pkg%d", i)
		pkgDir := filepath.Join(tmpDir, pkgName)
		require.NoError(t, os.Mkdir(pkgDir, 0755))

		var content strings.Builder
		content.WriteString(fmt.Sprintf("package %s\n\n", pkgName))
		for j := 0; j < funcsPerFile; j++ {
			content.WriteString(fmt.Sprintf("func Func%d() {}\n", j))
		}
		filePath := filepath.Join(pkgDir, "file.go")
		require.NoError(t, os.WriteFile(filePath, []byte(content.String()), 0644))
	}

	// Create a main package that uses some of the functions
	mainDir := filepath.Join(tmpDir, "cmd", "main")
	require.NoError(t, os.MkdirAll(mainDir, 0755))

	var mainContent strings.Builder
	mainContent.WriteString("package main\n\n")
	for i := 0; i < fileCount; i++ {
		// Use only the first function from each package
		mainContent.WriteString(fmt.Sprintf("import \"largeproject/pkg%d\"\n", i))
	}
	mainContent.WriteString("\nfunc main() {\n")
	for i := 0; i < fileCount; i++ {
		mainContent.WriteString(fmt.Sprintf("\tpkg%d.Func0()\n", i))
	}
	mainContent.WriteString("}\n")

	mainPath := filepath.Join(mainDir, "main.go")
	require.NoError(t, os.WriteFile(mainPath, []byte(mainContent.String()), 0644))

	// Hijack stdout to capture the output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run the analyzer
	err = run(context.Background(), false, false, tmpDir, false, true, "auto", []string{"./..."}, []string{"vendor"})
	require.NoError(t, err)

	// Restore stdout and read the output
	w.Close()
	os.Stdout = oldStdout
	var out bytes.Buffer
	_, err = out.ReadFrom(r)
	require.NoError(t, err)

	// Decode the JSON output
	var orphans []struct {
		Name string `json:"name"`
	}
	err = json.Unmarshal(out.Bytes(), &orphans)
	require.NoError(t, err, "Failed to unmarshal JSON output: %s", out.String())

	// Verify the results
	// We expect (funcsPerFile - 1) orphans from each of the fileCount packages
	expectedOrphanCount := (funcsPerFile - 1) * fileCount
	assert.Equal(t, expectedOrphanCount, len(orphans), "Incorrect number of orphans found")

	// Spot check a few orphans
	orphanSet := make(map[string]bool)
	for _, o := range orphans {
		orphanSet[o.Name] = true
	}

	assert.True(t, orphanSet["largeproject/pkg0.Func1"], "Expected Func1 from pkg0 to be an orphan")
	assert.True(t, orphanSet["largeproject/pkg50.Func4"], "Expected Func4 from pkg50 to be an orphan")
	assert.False(t, orphanSet["largeproject/pkg0.Func0"], "Func0 from pkg0 should not be an orphan")
}
