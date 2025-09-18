package symgo_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgotest"
)

func TestUnresolvedOps(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": `
module mytest
go 1.24
`,
			"ext/def.go": `
package ext
type ExternalType struct {
	N int
}
`,
			"usecase/usecase.go": `
package usecase
import "mytest/ext"

func UseIt(p *ext.ExternalType) {
	v := *p // dereference a pointer to an unresolved type
	_ = -v.N // unary minus on a field of the unresolved type
}
`,
		},
		EntryPoint: "mytest/usecase.UseIt",
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(func(path string) bool {
				// Only scan the usecase package, not the 'ext' package.
				return path == "mytest/usecase"
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}
		// The main check is that the execution completes without panicking or returning an error.
	}

	symgotest.Run(t, tc, action)
}

func TestUnresolvedOps_DereferenceUnresolvedType(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": `
module mytest
go 1.24
`,
			"ext/def.go": `
package ext
type ExternalType struct {
	N int
}
`,
			"usecase/usecase.go": `
package usecase
import "mytest/ext"

func UseIt() {
	// This is invalid Go, but the symbolic engine should handle it gracefully
	// instead of panicking with "invalid indirect". This simulates the case
	// found in the find-orphans example where the evaluator attempts to
	// dereference a type object from an unscanned package.
	_ = *ext.ExternalType(nil)
}
`,
		},
		EntryPoint: "mytest/usecase.UseIt",
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(func(path string) bool {
				return path == "mytest/usecase"
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestUnresolvedOps_Comprehensive(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": `
module mytest
go 1.24
`,
			"ext/def.go": `
package ext
type ExternalType struct {
	N int
	B bool
}
`,
			"usecase/usecase.go": `
package usecase
import "mytest/ext"

func UseIt(p *ext.ExternalType) {
	v := *p

	// Test case for binary operator
	b := v.N > 0

	// Test case for logical NOT operator
	c := !b

	// Test case for increment operator
	v.N++

	// Test case for field access on a pointer to a symbolic value
	ptr := &v
	_ = ptr.N
}
`,
		},
		EntryPoint: "mytest/usecase.UseIt",
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(func(path string) bool {
				// Only scan the usecase package, not the 'ext' package.
				return path == "mytest/usecase"
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}
	}

	symgotest.Run(t, tc, action)
}
