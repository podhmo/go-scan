package main

import (
	"multi_impl/pkg/iface"
	"multi_impl/pkg/impl1"
	"multi_impl/pkg/impl2"
	"os"
)

func main() {
	var s iface.Speaker
	if len(os.Args) > 1 {
		s = impl1.New()
	} else {
		s = impl2.New()
	}
	s.Speak()
}
