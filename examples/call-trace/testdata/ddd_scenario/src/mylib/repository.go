package mylib

// Thing is an example interface.
type Thing interface {
	Do()
}

type thing struct{}

// Do calls the target helper function.
func (t *thing) Do() {
	Helper()
}

// NewThing creates a new Thing.
func NewThing() Thing {
	return &thing{}
}

// Helper is our target function.
func Helper() {}
