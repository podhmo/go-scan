package integrationtest

// ptr returns a pointer to the given value.
// This is a helper function for creating pointers to literal values in tests.
func ptr[T any](v T) *T {
	return &v
}
