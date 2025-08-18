package hex

import (
	"encoding/hex"

	"github.com/podhmo/go-scan/minigo"
)

// Install binds all exported symbols from the "encoding/hex" package to the interpreter.
func Install(interp *minigo.Interpreter) {
	interp.Register("encoding/hex", map[string]any{
		"EncodeToString": hex.EncodeToString,
	})
}
