package errors

import (
	"fmt"
	"strings"
)

// ErrorCollector collects errors with path tracking.
type ErrorCollector struct {
	maxErrors int
	errors    []error
	pathStack []string
}

// NewErrorCollector creates a new ErrorCollector.
// maxErrors is the maximum number of errors to collect. 0 means unlimited.
func NewErrorCollector(maxErrors int) *ErrorCollector {
	return &ErrorCollector{
		maxErrors: maxErrors,
		errors:    make([]error, 0),
		pathStack: make([]string, 0),
	}
}

// Add records an error with the current path context.
// If maxErrors is reached, subsequent errors are not added, but the function
// still indicates if an error would have been added.
// Returns true if the error limit has been reached (or was already reached).
func (ec *ErrorCollector) Add(reason string) bool {
	if ec.maxErrors > 0 && len(ec.errors) >= ec.maxErrors {
		return true // Error limit reached
	}

	fullPath := strings.Join(ec.pathStack, "")
	err := fmt.Errorf("%s: %s", fullPath, reason)
	ec.errors = append(ec.errors, err)

	return ec.maxErrors > 0 && len(ec.errors) >= ec.maxErrors
}

// Addf records a formatted error with the current path context.
// Similar to Add, it respects maxErrors.
// Returns true if the error limit has been reached.
func (ec *ErrorCollector) Addf(format string, args ...interface{}) bool {
	if ec.maxErrors > 0 && len(ec.errors) >= ec.maxErrors {
		return true
	}
	reason := fmt.Sprintf(format, args...)
	return ec.Add(reason)
}

// Enter adds a new segment to the current error path.
// Segments can be struct field names or slice/array indices (e.g., "[0]").
// If adding a field name that is not the first element in the path, a "." separator is prepended.
func (ec *ErrorCollector) Enter(segment string) {
	if len(ec.pathStack) > 0 && !strings.HasPrefix(segment, "[") && !strings.HasSuffix(ec.pathStack[len(ec.pathStack)-1], ".") {
		ec.pathStack = append(ec.pathStack, "."+segment)
	} else {
		ec.pathStack = append(ec.pathStack, segment)
	}
}

// Leave removes the last segment from the error path.
// Call this with defer after Enter.
func (ec *ErrorCollector) Leave() {
	if len(ec.pathStack) > 0 {
		ec.pathStack = ec.pathStack[:len(ec.pathStack)-1]
	}
}

// Errors returns all collected errors.
func (ec *ErrorCollector) Errors() []error {
	return ec.errors
}

// CurrentPath returns the current error path string.
func (ec *ErrorCollector) CurrentPath() string {
	return strings.Join(ec.pathStack, "")
}

// HasErrors returns true if any errors have been collected.
func (ec *ErrorCollector) HasErrors() bool {
	return len(ec.errors) > 0
}

// MaxErrorsReached returns true if the configured maximum number of errors has been reached.
func (ec *ErrorCollector) MaxErrorsReached() bool {
	return ec.maxErrors > 0 && len(ec.errors) >= ec.maxErrors
}
