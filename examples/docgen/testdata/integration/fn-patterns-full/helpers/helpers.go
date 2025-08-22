package helpers

import "net/http"

// RespondJSON is a sample helper function that we want to create a pattern for.
func RespondJSON(w http.ResponseWriter, data any) {
	// In a real app, this would marshal the data to JSON and write it.
	// For the test, the implementation doesn't matter.
}
