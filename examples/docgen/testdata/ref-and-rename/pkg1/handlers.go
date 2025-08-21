package pkg1

import (
	"encoding/json"
	"net/http"

	"ref-and-rename/shared"
)

// ListItems returns a list of shared.Payload.
// The schema for shared.Payload should be a $ref.
func ListItems(w http.ResponseWriter, r *http.Request) {
	items := []shared.Payload{
		{ID: "1", Name: "foo"},
		{ID: "2", Name: "bar"},
	}
	json.NewEncoder(w).Encode(items)
}

// GetItem returns a single shared.Payload.
// The schema for shared.Payload should also be a $ref, pointing to the same component.
func GetItem(w http.ResponseWriter, r *http.Request) {
	item := shared.Payload{
		ID:   "1",
		Name: "foo",
	}
	json.NewEncoder(w).Encode(item)
}
