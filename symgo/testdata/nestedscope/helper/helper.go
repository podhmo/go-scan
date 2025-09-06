package helper

import "fmt"

func unexportedFunc() error {
	fmt.Println("hello from unexported")
	return nil
}

func PublicFunc() {
	if err := unexportedFunc(); err != nil {
		// do nothing
	}
}
