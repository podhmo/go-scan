package id

import (
	"fmt"

	"github.com/podhmo/go-scan/scanner"
)

// FromFunc returns a unique identifier for a function or method.
func FromFunc(pkg *scanner.PackageInfo, fn *scanner.FunctionInfo) string {
	if fn.Receiver != nil {
		// Use the String() method on FieldType which is designed for this.
		recvTypeStr := fn.Receiver.Type.String()
		return fmt.Sprintf("(%s.%s).%s", pkg.ImportPath, recvTypeStr, fn.Name)
	}
	return fmt.Sprintf("%s.%s", pkg.ImportPath, fn.Name)
}

// FromType returns a unique identifier for a type.
func FromType(t *scanner.TypeInfo) string {
	return fmt.Sprintf("%s.%s", t.PkgPath, t.Name)
}
