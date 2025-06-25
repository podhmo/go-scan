package externaltypes

import "github.com/google/uuid" // This import path is used as a string key
import "example.com/somepkg"     // This import path is used as a string key

type ObjectWithUUID struct {
	ID          uuid.UUID `json:"id"`
	Description string    `json:"description"`
}

type ObjectWithCustomTime struct {
	Timestamp somepkg.Time `json:"timestamp"`
	Name      string       `json:"name"`
}
