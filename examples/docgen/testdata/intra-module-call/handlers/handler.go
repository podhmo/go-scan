package handlers

import (
	"encoding/json"
	"net/http"

	"example.com/docgen_intra_module/helpers"
)

// GetUserHandler handles requests to get a user.
// It calls a helper function from another package in the same module.
func GetUserHandler(w http.ResponseWriter, r *http.Request) {
	user := helpers.GetUser()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(user)
}
