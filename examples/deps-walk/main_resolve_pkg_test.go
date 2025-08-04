package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/scantest"
)

func TestResolvePkgPath(t *testing.T) {
	ctx := context.Background()

	// Setup a temporary directory structure
	// <tmpdir>/
	//   go.mod (module my/app)
	//   main.go
	//   internal/
	//     api/
	//       handler.go
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module my/app\n",
		"main.go": `package main

func main() {}
`,
		"internal/api/handler.go": `package api`,
	})
	defer cleanup()

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get working directory: %v", err)
	}
	if err := os.Chdir(tmpdir); err != nil {
		t.Fatalf("could not change to tmpdir: %v", err)
	}
	defer os.Chdir(originalWD)

	cases := []struct {
		name      string
		startPath string
		want      string
		wantErr   bool
	}{
		{
			name:      "current directory",
			startPath: ".",
			want:      "my/app",
			wantErr:   false,
		},
		{
			name:      "subdirectory",
			startPath: "./internal/api",
			want:      "my/app/internal/api",
			wantErr:   false,
		},
		{
			name:      "file in subdirectory",
			startPath: "./internal/api/handler.go",
			want:      "my/app/internal/api/handler.go",
			wantErr:   false,
		},
		{
			name:      "absolute path to subdirectory",
			startPath: filepath.Join(tmpdir, "internal/api"),
			want:      "my/app/internal/api",
			wantErr:   false,
		},
		{
			name:      "already a package path",
			startPath: "github.com/foo/bar",
			want:      "github.com/foo/bar",
			wantErr:   false,
		},
		{
			name:      "path does not exist",
			startPath: "./nonexistent",
			wantErr:   true,
		},
		{
			name: "path outside module",
			// This test requires creating a directory outside the module.
			// We'll create a sibling directory to tmpdir.
			startPath: func() string {
				parentDir := filepath.Dir(tmpdir)
				outsideDir := filepath.Join(parentDir, "outside_module")
				os.Mkdir(outsideDir, 0755)
				// Defer cleanup of this new directory
				t.Cleanup(func() { os.RemoveAll(outsideDir) })
				return outsideDir
			}(),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolvePkgPath(ctx, tc.startPath)

			if (err != nil) != tc.wantErr {
				t.Fatalf("resolvePkgPath() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("resolvePkgPath() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
