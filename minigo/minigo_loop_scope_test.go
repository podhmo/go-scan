package minigo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/minigo"
	stdstrconv "github.com/podhmo/go-scan/minigo/stdlib/strconv"
	stdstrings "github.com/podhmo/go-scan/minigo/stdlib/strings"
)

type loopTestcase struct {
	name       string
	input      string
	imports    []string // packages to install
	resultType string   // the type of the result variable, e.g. "[]int"
	want       string   // expected output of the last expression
}

func (tt *loopTestcase) run(t *testing.T) {
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}

	for _, pkg := range tt.imports {
		switch pkg {
		case "strings":
			stdstrings.Install(interp)
		case "strconv":
			stdstrconv.Install(interp)
		}
	}

	lines := strings.Split(strings.TrimSpace(tt.input), "\n")
	lastLine := lines[len(lines)-1]
	body := strings.Join(lines[:len(lines)-1], "\n")

	script := "package main\n"
	if len(tt.imports) > 0 {
		script += "import (\n"
		for _, pkg := range tt.imports {
			script += fmt.Sprintf("\t\"%s\"\n", pkg)
		}
		script += ")\n"
	}
	script += fmt.Sprintf("var __result__ %s\n", tt.resultType)
	script += "func main() {\n"
	script += body
	script += fmt.Sprintf("\n\t__result__ = %s\n", lastLine)
	script += "}\n"

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v\n-- script --\n%s", err, script)
	}

	_, err = interp.Eval(context.Background())
	if err != nil {
		t.Fatalf("failed to evaluate script: %+v\n-- script --\n%s", err, script)
	}

	env := interp.GlobalEnvForTest()
	val, ok := env.Get("__result__")
	if !ok {
		t.Fatalf("variable '__result__' not found")
	}

	got := val.Inspect()
	if diff := cmp.Diff(tt.want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s\n-- script --\n%s", diff, script)
	}
}

func TestForLoopScope(t *testing.T) {
	tests := []loopTestcase{
		{
			name: "for loop variable should be captured by value in closures (Go 1.22+ semantics)",
			input: `
			var funcs []func() int
			for i := 0; i < 3; i++ {
				funcs = append(funcs, func() int { return i })
			}

			var results []int
			for _, f := range funcs {
				results = append(results, f())
			}
			results
			`,
			resultType: "[]int",
			want:       `[0 1 2]`,
		},
		{
			name: "for...range loop variable (value) should be captured correctly (already correct)",
			input: `
			var funcs []func() int
			items := []int{10, 20, 30}
			for _, item := range items {
				funcs = append(funcs, func() int { return item })
			}

			var results []int
			for _, f := range funcs {
				results = append(results, f())
			}
			results
			`,
			resultType: "[]int",
			want:       `[10 20 30]`,
		},
		{
			name: "for...range loop variable (key, value) should be captured correctly (already correct)",
			input: `
			var funcs []func() string
			items := []string{"a", "b"}
			for i, item := range items {
				funcs = append(funcs, func() string {
					return strconv.Itoa(i) + ":" + item
				})
			}

			var results []string
			for _, f := range funcs {
				results = append(results, f())
			}
			results
			`,
			imports:    []string{"strconv"},
			resultType: "[]string",
			want:       `[0:a 1:b]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
