package api

import (
	"net/http"

	admin_handlers "ref-and-rename/admin/handlers"
	"ref-and-rename/pkg2"
	user_handlers "ref-and-rename/actions"
)

func NewServeMux() *http.ServeMux {
	mux := http.NewServeMux()

	// This handler uses the 'shared.Payload' struct.
	mux.HandleFunc("GET /items", user_handlers.ListItems)
	// This handler also uses the 'shared.Payload' struct.
	mux.HandleFunc("GET /items/{id}", user_handlers.GetItem)

	// This handler is from a different package but with the same package name 'handlers'.
	// It should have a unique operationId.
	mux.HandleFunc("GET /admin/items", admin_handlers.ListItems)

	// This handler is in a different package ('pkg2') and uses a different
	// 'Payload' struct, which has the same name but different fields.
	// It should be a separate schema definition.
	mux.HandleFunc("GET /pkg2/items", pkg2.ListItems)

	return mux
}
