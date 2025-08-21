package main

import (
	"net/http"
)

// Helper functions to be recognized by custom patterns.
func GetQueryParam(r *http.Request, key string) string {
	return r.URL.Query().Get(key)
}

func GetHeader(r *http.Request, key string) string {
	return r.Header.Get(key)
}

func GetPathValue(r *http.Request, key string) string {
	// In a real app, this would parse the URL path.
	// For analysis, the implementation doesn't matter.
	return "some-value"
}

// Handler that uses all parameter types.
func GetResource(w http.ResponseWriter, r *http.Request) {
	// These calls will be detected by the custom patterns.
	_ = GetPathValue(r, "resourceId")
	_ = GetQueryParam(r, "filter")
	_ = GetHeader(r, "X-Request-ID")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func main() {
	mux := http.NewServeMux()
	// Note: The path parameter name in the route must match the one in the pattern.
	mux.HandleFunc("GET /resources/{resourceId}", GetResource)
	http.ListenAndServe(":8080", mux)
}
