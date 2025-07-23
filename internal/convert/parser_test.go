package convert

import (
	"context"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/scantest"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		pkgPath string
		want    *ParsedInfo
		wantErr bool
	}{
		{
			name: "simple pair",
			files: map[string]string{
				"go.mod": "module myapp",
				"models/models.go": `
package models

// @derivingconvert(Dst)
type Src struct {}

type Dst struct {}
`,
			},
			pkgPath: "myapp/models",
			want: &ParsedInfo{
				PackageName: "models",
				PackagePath: "myapp/models",
				ConversionPairs: []ConversionPair{
					{SrcTypeName: "Src", DstTypeName: "Dst"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, cleanup := scantest.WriteFiles(t, tt.files)
			defer cleanup()

			cwd, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get current working directory: %v", err)
			}
			if err := os.Chdir(dir); err != nil {
				t.Fatalf("failed to change directory to %s: %v", dir, err)
			}
			defer os.Chdir(cwd)

			ctx := context.Background()
			got, err := Parse(ctx, tt.pkgPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
