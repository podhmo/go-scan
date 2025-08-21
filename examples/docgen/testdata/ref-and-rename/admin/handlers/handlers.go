package handlers

import (
	"encoding/json"
	"net/http"
)

// Payload is local to this package.
type Payload struct {
	AdminID string `json:"adminId"`
}

// ListItems collides in name with the function in the other 'handlers' package.
// The analyzer should generate a unique operationId for it.
func ListItems(w http.ResponseWriter, r *http.Request) {
	items := []Payload{
		{AdminID: "admin1"},
	}
	json.NewEncoder(w).Encode(items)
}
