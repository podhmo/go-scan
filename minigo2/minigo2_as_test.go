package minigo2

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo2/testdata/shared"
)

func TestResult_As(t *testing.T) {
	type Person struct {
		Name string
		Age  int
	}

	type Company struct {
		Name      string
		Employees []Person
	}

	tests := []struct {
		name       string
		script     string
		target     any // A pointer to the variable to unmarshal into
		want       any
		wantErr    bool
		checkFunc  func(t *testing.T, target any) // Optional custom check
	}{
		{
			name:   "unmarshal integer",
			script: "123",
			target: new(int),
			want:   123,
		},
		{
			name:   "unmarshal string",
			script: `"hello world"`,
			target: new(string),
			want:   "hello world",
		},
		{
			name:   "unmarshal boolean",
			script: "true",
			target: new(bool),
			want:   true,
		},
		{
			name:   "unmarshal simple struct",
			script: "package main\ntype Person struct { Name string; Age int }\nvar result = Person{Name: \"gopher\", Age: 10}",
			target: new(Person),
			want:   Person{Name: "gopher", Age: 10},
		},
		{
			name:   "unmarshal struct with case difference",
			script: "package main\ntype Person struct { name string; age int }\nvar result = Person{name: \"Gopher\", age: 10}",
			target: new(Person),
			want:   Person{Name: "Gopher", Age: 10},
		},
		{
			name:   "unmarshal slice of integers",
			script: "package main\nvar result = []int{1, 2, 3}",
			target: new([]int),
			checkFunc: func(t *testing.T, target any) {
				got, ok := target.(*[]int)
				if !ok {
					t.Fatalf("target is not *[]int, got %T", target)
				}
				want := []int{1, 2, 3}
				if !reflect.DeepEqual(*got, want) {
					t.Errorf("got %v, want %v", *got, want)
				}
			},
		},
		{
			name:   "unmarshal slice of structs",
			script: "package main\ntype Person struct { Name string; Age int }\nvar result = []Person{Person{Name: \"a\", Age: 1}, Person{Name: \"b\", Age: 2}}",
			target: new([]Person),
			want:   []Person{{Name: "a", Age: 1}, {Name: "b", Age: 2}},
		},
		{
			name:   "unmarshal map[string]int",
			script: "package main\nvar result = map[string]int{\"one\": 1, \"two\": 2}",
			target: new(map[string]int),
			want:   map[string]int{"one": 1, "two": 2},
		},
		{
			name: "unmarshal nested struct",
			script: `package main
				type Person struct { Name string; Age int }
				type Company struct { Name string; Employees []Person }
				var result = Company{
					Name: "Megacorp",
					Employees: []Person{
						Person{Name: "gopher", Age: 10},
						Person{Name: "octocat", Age: 12},
					},
				}
			`,
			target: new(Company),
			want: Company{
				Name: "Megacorp",
				Employees: []Person{
					{Name: "gopher", Age: 10},
					{Name: "octocat", Age: 12},
				},
			},
		},
		{
			name:   "unmarshal into pointer to struct",
			script: "package main\ntype Person struct { Name string }\nvar result = Person{Name: \"pointer\"}",
			target: new(*Person),
			checkFunc: func(t *testing.T, target any) {
				got, ok := target.(**Person)
				if !ok {
					t.Fatalf("target is not **Person, got %T", target)
				}
				if (*got).Name != "pointer" {
					t.Errorf("got %q, want %q", (*got).Name, "pointer")
				}
			},
		},
		{
			name:   "unmarshal null into slice",
			script: "package main\nvar result = null",
			target: new([]Person),
			checkFunc: func(t *testing.T, target any) {
				got, ok := target.(*[]Person)
				if !ok {
					t.Fatalf("target is not *[]Person, got %T", target)
				}
				if *got != nil {
					t.Errorf("got %v, want nil", *got)
				}
			},
		},
		{
			name:    "error: target not a pointer",
			script:  "package main\nvar result = 123",
			target:  0,
			wantErr: true,
		},
		{
			name:    "error: target is nil pointer",
			script:  "package main\nvar result = 123",
			target:  (*int)(nil),
			wantErr: true,
		},
		{
			name:    "error: type mismatch",
			script:  "package main\nvar result = 123",
			target:  new(string),
			wantErr: true,
		},
		{
			name: "unmarshal cross-package struct",
			script: `package main
import "github.com/podhmo/go-scan/minigo2/testdata/shared"
var result = shared.Point{X: 10, Y: 20}`,
			target: new(shared.Point),
			want:   shared.Point{X: 10, Y: 20},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interpreter, err := NewInterpreter()
			if err != nil {
				t.Fatalf("NewInterpreter() failed: %v", err)
			}

			// For simple expression tests, wrap them in a full program.
			// For tests that are already full programs, use them as is.
			var fullScript string
			if !strings.HasPrefix(tt.script, "package main") {
				fullScript = "package main\n\nvar result = " + tt.script
			} else {
				fullScript = tt.script
			}

			res, err := interpreter.Eval(context.Background(), Options{
				Source:   []byte(fullScript),
				Filename: "test.mgo",
			})
			if err != nil {
				// If we expect an error from As(), the Eval() should not fail.
				if !tt.wantErr {
					t.Fatalf("Eval() returned an error: %v", err)
				}
			}

			// For tests that expect an error from As(), Eval might have succeeded or failed.
			// We only proceed if Eval succeeded.
			if err == nil {
				val, ok := interpreter.Env.Get("result")
				if !ok {
					t.Fatalf("could not find 'result' variable in environment")
				}
				res.Value = val
			}

			err = res.As(tt.target)

			if (err != nil) != tt.wantErr {
				t.Fatalf("Result.As() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return // Don't check the value if an error was expected
			}

			if tt.checkFunc != nil {
				tt.checkFunc(t, tt.target)
			} else {
				// Use reflection to get the value from the pointer
				val := reflect.ValueOf(tt.target).Elem().Interface()
				if !reflect.DeepEqual(val, tt.want) {
					t.Errorf("Result.As() got = %#v, want %#v", val, tt.want)
				}
			}
		})
	}
}
