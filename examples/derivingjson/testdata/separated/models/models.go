package models

import "github.com/podhmo/go-scan/examples/derivingjson/testdata/separated/shapes"

// Container holds a shape.
// @deriving:unmarshal
type Container struct {
	Content shapes.Shape `json:"content"`
}
