package shared

import "github.com/podhmo/go-scan/minigo2/testdata/deeper"

// Container holds a nested struct from another package.
type Container struct {
	Name    string
	Payload deeper.Payload
}
