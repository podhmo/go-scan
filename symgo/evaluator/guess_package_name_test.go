package evaluator

import (
	"reflect"
	"sort"
	"testing"
)

func TestGuessPackageNameFromImportPath(t *testing.T) {
	testcases := []struct {
		msg        string
		importPath string
		want       []string
	}{
		{
			msg:        "standard library",
			importPath: "fmt",
			want:       []string{"fmt"},
		},
		{
			msg:        "standard library subpackage",
			importPath: "net/http",
			want:       []string{"http"},
		},
		{
			msg:        "github, no version",
			importPath: "github.com/pkg/errors",
			want:       []string{"errors"},
		},
		{
			msg:        "github, with .git suffix",
			importPath: "github.com/pkg/errors.git",
			want:       []string{"errors"},
		},
		{
			msg:        "version suffix vN",
			importPath: "github.com/go-chi/chi/v5",
			want:       []string{"chi"},
		},
		{
			msg:        "version suffix vN with subpackage",
			importPath: "github.com/go-chi/chi/v5/middleware",
			want:       []string{"middleware"},
		},
		{
			msg:        "gopkg.in style version",
			importPath: "gopkg.in/yaml.v2",
			want:       []string{"yaml"},
		},
		{
			msg:        "go- prefix and hyphen",
			importPath: "github.com/mattn/go-isatty",
			want:       []string{"goisatty", "isatty"},
		},
		{
			msg:        "go- prefix only",
			importPath: "github.com/podhmo/go-scan",
			want:       []string{"goscan", "scan"},
		},
		{
			msg:        "hyphenated name",
			importPath: "github.com/some/pkg-name",
			want:       []string{"pkgname"},
		},
		{
			msg:        "empty path",
			importPath: "",
			want:       nil,
		},
		{
			msg:        "single element path",
			importPath: "foobar",
			want:       []string{"foobar"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.msg, func(t *testing.T) {
			got := guessPackageNameFromImportPath(tc.importPath)
			// Sort both slices for stable comparison
			sort.Strings(got)
			sort.Strings(tc.want)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("guessPackageNameFromImportPath(%q) = %q, want %q", tc.importPath, got, tc.want)
			}
		})
	}
}