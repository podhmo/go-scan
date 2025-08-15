package pkga

import "github.com/podhmo/go-scan/minigo/testdata/pkgb"

func FuncA() string {
	return "A says: " + pkgb.FuncB()
}
