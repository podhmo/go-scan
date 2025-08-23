package query

import "net/http"

// Get is a dummy function for our pattern to match.
// In a real app, this would extract a query parameter.
func Get(r *http.Request, key string) string {
	return r.URL.Query().Get(key)
}
