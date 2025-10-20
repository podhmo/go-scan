package intermediatelib

import (
	"github.com/podhmo/go-scan/examples/call-trace/testdata/indirect/src/mylib"
)

func CallHelper() {
	mylib.Helper()
}
