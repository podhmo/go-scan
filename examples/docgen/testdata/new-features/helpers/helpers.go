package helpers

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is a generic error response structure.
type ErrorResponse struct {
	Error string `json:"error"`
}

// RenderError is a helper to write a JSON error response with a specific status code.
// Our custom pattern will target this function.
func RenderError(w http.ResponseWriter, r *http.Request, status int, err error) {
	w.WriteHeader(status)
	response := ErrorResponse{Error: err.Error()}
	json.NewEncoder(w).Encode(response)
}

// RenderJSON is a generic helper to write a JSON response.
// We will use this to test map and other struct responses.
func RenderJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
