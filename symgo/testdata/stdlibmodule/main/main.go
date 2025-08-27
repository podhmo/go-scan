package main

import (
	"encoding/json"
	"io"
)

func GetEncoder(w io.Writer) *json.Encoder {
	return json.NewEncoder(w)
}

func main() {
	// We don't actually run this, we just analyze GetEncoder.
}
