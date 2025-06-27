// Package binding provides a type-safe, reflect-free, and expression-oriented
// way to bind data from HTTP requests to Go structs.
package binding

import (
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"strings"
)

// Source represents the source of a value in an HTTP request.
type Source string

const (
	Query  Source = "query"
	Header Source = "header"
	Cookie Source = "cookie"
	Path   Source = "path"
)

// Requirement specifies whether a value is required or optional.
type Requirement bool

const (
	Required Requirement = true
	Optional Requirement = false
)

// Parser is a generic function that parses a string into a value of type T.
// It returns an error if parsing fails.
type Parser[T any] func(string) (T, error)

// Binding holds the context for a binding operation, including the HTTP request
// and a function to retrieve path parameters.
type Binding struct {
	req       *http.Request
	pathValue func(string) string
}

// New creates a new Binding instance from an *http.Request and a function to retrieve path parameters.
// The pathValue function is typically provided by a routing library (e.g., chi, gorilla/mux).
func New(req *http.Request, pathValue func(string) string) *Binding {
	return &Binding{req: req, pathValue: pathValue}
}

// Lookup is an internal method that retrieves a value and its existence from a given source.
// It abstracts away the differences in how each source (query, header, etc.) is accessed.
func (b *Binding) Lookup(source Source, key string) (string, bool) {
	switch source {
	case Query:
		// .Query().Has() is available in Go 1.17+ and checks for key presence.
		if b.req.URL.Query().Has(key) {
			return b.req.URL.Query().Get(key), true
		}
		return "", false
	case Header:
		// Header keys are case-insensitive.
		canonicalKey := textproto.CanonicalMIMEHeaderKey(key)
		if vals, ok := b.req.Header[canonicalKey]; ok {
			// A key's presence, even with an empty value, is considered "exists".
			if len(vals) > 0 {
				return vals[0], true
			}
			return "", true // e.g., "X-Custom-Header:"
		}
		return "", false
	case Cookie:
		cookie, err := b.req.Cookie(key)
		if err == nil { // If err is nil, the cookie exists.
			return cookie.Value, true
		}
		return "", false // e.g., http.ErrNoCookie
	case Path:
		if b.pathValue != nil {
			val := b.pathValue(key)
			if val != "" {
				return val, true
			}
			return "", false
		}
		return "", false
	}
	return "", false
}

// One binds a single value of a non-pointer type (e.g., int, string).
// 'dest' must be a pointer to the field where the value will be stored (e.g., &r.ID).
func One[T any](b *Binding, dest *T, source Source, key string, parse Parser[T], req Requirement) error {
	valStr, ok := b.Lookup(source, key)
	if !ok {
		if req == Required {
			return fmt.Errorf("binding: %s key '%s' is required", source, key)
		}
		return nil // Optional and not present is a success.
	}

	// For optional fields, an empty value is often treated as "not provided".
	// If you need to handle empty but present values, the logic can be adjusted here.
	if !ok && req == Optional {
		return nil
	}

	val, err := parse(valStr)
	if err != nil {
		return fmt.Errorf("binding: failed to parse %s key '%s' with value %q: %w", source, key, valStr, err)
	}

	*dest = val
	return nil
}

// OnePtr binds a single value of a pointer type (e.g., *int, *string).
// 'dest' must be a pointer to the pointer field (e.g., &r.Name).
// If the value is optional and not present, the destination pointer is set to nil.
func OnePtr[T any](b *Binding, dest **T, source Source, key string, parse Parser[T], req Requirement) error {
	valStr, ok := b.Lookup(source, key)
	if !ok {
		if req == Required {
			return fmt.Errorf("binding: %s key '%s' is required", source, key)
		}
		*dest = nil // Optional and not present: set field to nil.
		return nil
	}

	val, err := parse(valStr)
	if err != nil {
		return fmt.Errorf("binding: failed to parse %s key '%s' with value %q: %w", source, key, valStr, err)
	}

	*dest = &val // Store the address of the parsed value.
	return nil
}

// Slice binds a comma-separated string into a slice of a non-pointer type (e.g., []int, []string).
// 'dest' must be a pointer to the slice field (e.g., &r.Tags).
func Slice[T any](b *Binding, dest *[]T, source Source, key string, parse Parser[T], req Requirement) error {
	valStr, ok := b.Lookup(source, key)
	if !ok {
		if req == Required {
			return fmt.Errorf("binding: %s key '%s' is required", source, key)
		}
		*dest = nil
		return nil
	}

	itemsStr := strings.Split(valStr, ",")
	slice := make([]T, 0, len(itemsStr))
	var errs []error

	for i, itemStr := range itemsStr {
		trimmed := strings.TrimSpace(itemStr)
		if trimmed == "" {
			continue // Skip empty items, e.g., "a,,b"
		}
		val, err := parse(trimmed)
		if err != nil {
			errs = append(errs, fmt.Errorf("binding: failed to parse item #%d for %s key '%s' with value %q: %w", i, source, key, itemStr, err))
			continue
		}
		slice = append(slice, val)
	}

	*dest = slice
	return errors.Join(errs...)
}

// SlicePtr binds a comma-separated string into a slice of a pointer type (e.g., []*int, []*string).
// 'dest' must be a pointer to the slice field (e.g., &r.CategoryIDs).
func SlicePtr[T any](b *Binding, dest *[]*T, source Source, key string, parse Parser[T], req Requirement) error {
	valStr, ok := b.Lookup(source, key)
	if !ok {
		if req == Required {
			return fmt.Errorf("binding: %s key '%s' is required", source, key)
		}
		*dest = nil
		return nil
	}

	itemsStr := strings.Split(valStr, ",")
	slice := make([]*T, 0, len(itemsStr))
	var errs []error

	for i, itemStr := range itemsStr {
		trimmed := strings.TrimSpace(itemStr)
		if trimmed == "" {
			continue
		}
		val, err := parse(trimmed)
		if err != nil {
			errs = append(errs, fmt.Errorf("binding: failed to parse item #%d for %s key '%s' with value %q: %w", i, source, key, itemStr, err))
			continue
		}
		slice = append(slice, &val)
	}

	*dest = slice
	return errors.Join(errs...)
}
