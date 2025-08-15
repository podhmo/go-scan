package pkge

import "github.com/podhmo/go-scan/minigo/testdata/pkgf"

func FuncE1() string {
	return "E1 says: " + pkgf.FuncF()
}
