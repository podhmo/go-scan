package complex

import "time"

// Base is an embedded struct.
type Base struct {
	ID uint64 `json:"id"`
}

// Profile represents a user profile with various field types.
type Profile struct {
	Base
	DisplayName *string           `json:"displayName,omitempty"`
	Tags        []string          `json:"tags"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"createdAt"`
	Active      bool              `json:"active"`
}