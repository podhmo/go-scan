package parser

import (
	"context"
	"os"
	"testing"

	goscan "github.com/podhmo/go-scan"
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
			s, err := goscan.New()
			if err != nil {
				t.Fatalf("failed to create scanner: %v", err)
			}
			got, err := Parse(ctx, s, tt.pkgPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil {
				if len(got) != len(tt.want.ConversionPairs) {
					t.Fatalf("Parse() got %d pairs, want %d", len(got), len(tt.want.ConversionPairs))
				}
				for i := range got {
					wantPair := tt.want.ConversionPairs[i]
					gotPair := got[i]
					if wantPair.SrcTypeName != gotPair.SrcTypeName {
						t.Errorf("pair %d: SrcTypeName mismatch: got %q, want %q", i, gotPair.SrcTypeName, wantPair.SrcTypeName)
					}
					if wantPair.DstTypeName != gotPair.DstTypeName {
						t.Errorf("pair %d: DstTypeName mismatch: got %q, want %q", i, gotPair.DstTypeName, wantPair.DstTypeName)
					}
					if gotPair.SrcInfo == nil {
						t.Errorf("pair %d: got nil SrcInfo", i)
					}
					if gotPair.DstInfo == nil {
						t.Errorf("pair %d: got nil DstInfo", i)
					}
				}
			} else if len(got) != 0 {
				t.Errorf("Parse() got non-empty pairs, want nil")
			}
		})
	}
}
