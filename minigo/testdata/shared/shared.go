package shared

import "github.com/podhmo/go-scan/minigo/testdata/deeper"

// Container holds a nested struct from another package.
type Container struct {
	Name    string
	Payload deeper.Payload
}
