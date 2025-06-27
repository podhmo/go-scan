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

// valuesFromSource retrieves all values for a given key from the specified source.
// For Query and Header, it can return multiple values if the key is repeated (Query)
// or if the header value itself is a list (though standard header practice is often one value per key,
// or comma-separated values within a single header line).
// For Cookie and Path, it's expected to return a single value or none.
func (b *Binding) valuesFromSource(source Source, key string) ([]string, bool) {
	switch source {
	case Query:
		if values, ok := b.req.URL.Query()[key]; ok && len(values) > 0 {
			return values, true
		}
		return nil, false
	case Header:
		canonicalKey := textproto.CanonicalMIMEHeaderKey(key)
		if values, ok := b.req.Header[canonicalKey]; ok && len(values) > 0 {
			// Headers can have multiple entries for the same key or comma-separated values.
			// Here, we return the raw values as found. Splitting comma-separated
			// values will be handled by the Slice/SlicePtr functions if needed.
			return values, true
		}
		return nil, false
	case Cookie:
		cookie, err := b.req.Cookie(key)
		if err == nil {
			return []string{cookie.Value}, true
		}
		return nil, false
	case Path:
		if b.pathValue != nil {
			val := b.pathValue(key)
			if val != "" {
				return []string{val}, true
			}
			return nil, false
		}
		return nil, false
	}
	return nil, false
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

// Slice binds values into a slice of a non-pointer type (e.g., []int, []string).
// 'dest' must be a pointer to the slice field (e.g., &r.Tags).
// It handles multiple values from Query parameters (e.g., ?tags=a&tags=b)
// and comma-separated values from Header, Cookie, or Path (e.g., X-Tags: a,b,c).
func Slice[T any](b *Binding, dest *[]T, source Source, key string, parse Parser[T], req Requirement) error {
	rawValues, ok := b.valuesFromSource(source, key)
	if !ok {
		if req == Required {
			return fmt.Errorf("binding: %s key '%s' is required", source, key)
		}
		*dest = nil
		return nil
	}

	slice := make([]T, 0)
	var errs []error

	for _, valStr := range rawValues {
		// For sources like Header, Cookie, Path, a single rawValue might contain comma-separated items.
		// For Query, each rawValue is typically a distinct item.
		// We split by comma regardless, as `strings.Split("single", ",")` yields `[]string{"single"}`.
		itemsStr := strings.Split(valStr, ",")
		for i, itemStr := range itemsStr {
			trimmed := strings.TrimSpace(itemStr)
			// Allow empty strings to be parsed if the parser handles them (e.g. to represent an empty element or a default)
			// The original code skipped empty items: `if trimmed == "" { continue }`
			// We now let the parser decide. If a parser wants to treat "" as an error, it can.
			// If it wants to treat "" as, say, a zero value or a specific marker, it can.
			// This is particularly relevant for cases like `X-Header: "1,,3"`, where the middle element is empty.

			val, err := parse(trimmed)
			if err != nil {
				errs = append(errs, fmt.Errorf("binding: failed to parse item #%d from value %q for %s key '%s': %w", i, itemStr, source, key, err))
				continue // Continue processing other items even if one fails
			}
			slice = append(slice, val)
		}
	}

	if len(errs) > 0 {
		// If parsing failed for all items and the field is optional,
		// it might be acceptable to return nil, but current behavior is to return errors.
		// If no items were successfully parsed and it was required, this is an issue.
		// However, if some items parsed and some failed, we still assign the partially filled slice.
		*dest = slice // Assign successfully parsed items
		return errors.Join(errs...)
	}

	if len(slice) == 0 && req == Required && !ok { // Should be covered by the initial !ok check
		return fmt.Errorf("binding: %s key '%s' is required, but no values found or all values were empty after split", source, key)
	}

	*dest = slice
	return nil
}

// SlicePtr binds values into a slice of a pointer type (e.g., []*int, []*string).
// 'dest' must be a pointer to the slice field (e.g., &r.CategoryIDs).
// It handles multiple values from Query parameters and comma-separated values from other sources.
// Empty strings resulting from parsing (e.g., "a,,b") will result in a nil pointer for that element if the parser succeeds for an empty string (which it typically might not, but if it did, it would be *new(T) where T is zero-valued).
// More commonly, an empty string like "" in "1,,2" will be passed to the parser. If the parser errors on "", that error is collected. If it somehow parses "" to a value, a pointer to that value is added.
func SlicePtr[T any](b *Binding, dest *[]*T, source Source, key string, parse Parser[T], req Requirement) error {
	rawValues, ok := b.valuesFromSource(source, key)
	if !ok {
		if req == Required {
			return fmt.Errorf("binding: %s key '%s' is required", source, key)
		}
		*dest = nil
		return nil
	}

	slice := make([]*T, 0)
	var errs []error

	for _, valStr := range rawValues {
		itemsStr := strings.Split(valStr, ",")
		for i, itemStr := range itemsStr {
			trimmed := strings.TrimSpace(itemStr)
			// Similar to Slice: we pass trimmed (which can be "") to the parser.
			// If parse(trimmed) is successful, we take the address of the result.
			// If `trimmed` is an empty string like in "val1,,val3", and `parse("")`
			// returns a value (e.g. zero value for T) and no error, then `&val` for that item will be `&T{}`.
			// If `parse("")` returns an error, that error is collected.

			val, err := parse(trimmed)
			if err != nil {
				errs = append(errs, fmt.Errorf("binding: failed to parse pointer item #%d from value %q for %s key '%s': %w", i, itemStr, source, key, err))
				continue
			}
			slice = append(slice, &val)
		}
	}

	if len(errs) > 0 {
		*dest = slice // Assign successfully parsed items
		return errors.Join(errs...)
	}

	if len(slice) == 0 && req == Required && !ok {
		return fmt.Errorf("binding: %s key '%s' is required, but no values found or all values were empty after split", source, key)
	}

	*dest = slice
	return nil
}
