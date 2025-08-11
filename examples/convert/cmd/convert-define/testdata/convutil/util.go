package convutil

import "time"

// TimeToString is a mock for testing.
func TimeToString(t time.Time) string {
	return "mock-time"
}

// PtrTimeToString is a mock for testing.
func PtrTimeToString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return "mock-ptr-time"
}
