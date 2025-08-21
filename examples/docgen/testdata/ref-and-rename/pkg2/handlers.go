package pkg2

import (
	"encoding/json"
	"net/http"
)

// ListItems has the same name as a handler in pkg1, but it's in a different package.
// It should have a unique operationId.
func ListItems(w http.ResponseWriter, r *http.Request) {
	items := []Payload{
		{Value: "value1"},
		{Value: "value2"},
	}
	json.NewEncoder(w).Encode(items)
}
