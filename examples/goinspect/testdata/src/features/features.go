package features

import (
	"fmt"

	"github.com/podhmo/go-scan/examples/goinspect/testdata/src/another"
)

// Data represents a simple data structure.
type Data struct {
	id   string
	name string
}

// GetID is a classic getter.
func (d *Data) GetID() string {
	return d.id
}

// SetName is a classic setter.
func (d *Data) SetName(name string) {
	d.name = name
}

// ComplexLogic performs some logic.
func (d *Data) ComplexLogic() {
	fmt.Printf("Processing data for %s\n", d.GetID())
	another.Helper()
}

// Execute takes a function and executes it.
func Execute(action func()) {
	fmt.Println("Before action")
	action()
	fmt.Println("After action")
}

// Main is the entry point for this feature set.
func Main() {
	d := &Data{id: "xyz-123", name: "test"}
	d.SetName("new-name")
	d.ComplexLogic()

	Execute(func() {
		fmt.Println("...action executed...")
	})
}