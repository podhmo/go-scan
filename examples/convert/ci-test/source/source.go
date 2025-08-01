package source

import (
	dest "ci-test/destination"
)

// @derivingconvert("dest.Dst")
type Src struct {
	ID   string
	Name string
}
