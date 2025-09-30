package evaluator

import (
	"testing"
)

func TestGuessPackageNameFromImportPath(t *testing.T) {
	testcases := []struct {
		msg        string
		importPath string
		want       string
	}{
		{
			msg:        "standard library",
			importPath: "fmt",
			want:       "fmt",
		},
		{
			msg:        "standard library subpackage",
			importPath: "net/http",
			want:       "http",
		},
		{
			msg:        "github, no version",
			importPath: "github.com/pkg/errors",
			want:       "errors",
		},
		{
			msg:        "github, with .git suffix (unusual but possible)",
			importPath: "github.com/pkg/errors.git",
			want:       "errors",
		},
		{
			msg:        "version suffix vN",
			importPath: "github.com/go-chi/chi/v5",
			want:       "chi",
		},
		{
			msg:        "version suffix vN with subpackage",
			importPath: "github.com/go-chi/chi/v5/middleware",
			want:       "middleware",
		},
		{
			msg:        "gopkg.in style version",
			importPath: "gopkg.in/yaml.v2",
			want:       "yaml",
		},
		{
			msg:        "go- prefix",
			importPath: "github.com/mattn/go-colorable",
			want:       "colorable",
		},
		{
			msg:        "go- prefix and hyphen",
			importPath: "github.com/mattn/go-isatty",
			want:       "isatty",
		},
		{
			msg:        "hyphenated name",
			importPath: "github.com/some/pkg-name",
			want:       "pkgname",
		},
		{
			msg:        "empty path",
			importPath: "",
			want:       "",
		},
		{
			msg:        "single element path",
			importPath: "foobar",
			want:       "foobar",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.msg, func(t *testing.T) {
			got := guessPackageNameFromImportPath(tc.importPath)
			if got != tc.want {
				t.Errorf("guessPackageNameFromImportPath(%q) = %q, want %q", tc.importPath, got, tc.want)
			}
		})
	}
}