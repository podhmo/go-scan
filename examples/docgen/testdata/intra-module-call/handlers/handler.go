package handlers

import (
	"encoding/json"
	"net/http"

	"example.com/docgen_intra_module/helpers"
)

// getInternalUser is an unexported helper function within the same package.
// symgo should be able to trace the call from the exported handler into this function.
func getInternalUser() helpers.User {
	return helpers.GetUser()
}

// GetUserHandler handles requests to get a user.
// It calls an unexported helper function in the same package, which in turn
// calls a helper function from another package in the same module.
func GetUserHandler(w http.ResponseWriter, r *http.Request) {
	user := getInternalUser()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(user)
}
