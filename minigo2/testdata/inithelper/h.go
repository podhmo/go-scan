package inithelper

import "github.com/podhmo/go-scan/minigo2/testdata/inittable"

func Get() int { return inittable.Lookup() }
