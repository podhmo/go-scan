package main

import "example.com/external"

func main() {
	// This tests `new(unresolved.Type)`
	_ = new(external.ExtType)

	// This tests dereferencing a pointer to an unresolved type.
	var p *external.ExtType
	_ = *p
}
