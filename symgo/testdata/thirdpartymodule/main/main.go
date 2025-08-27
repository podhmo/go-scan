package main

import (
	"fmt"

	"github.com/google/uuid"
)

func PrintUUID(id uuid.UUID) {
	fmt.Println(id.String())
}

func main() {
	// not used
}
