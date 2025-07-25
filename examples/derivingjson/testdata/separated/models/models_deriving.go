// Code generated by go-scan for package models. DO NOT EDIT.

package models

import (
	"encoding/json"
	"fmt"

	shapes "github.com/podhmo/go-scan/examples/derivingjson/testdata/separated/shapes"
)

func (s *Container) UnmarshalJSON(data []byte) error {
	// Define an alias type to prevent infinite recursion with UnmarshalJSON.
	type Alias Container
	aux := &struct {
		Content json.RawMessage `json:"content"`

		// All other fields will be handled by the standard unmarshaler via the Alias.
		*Alias
	}{
		Alias: (*Alias)(s),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal into aux struct for Container: %w", err)
	}

	// Process Content
	if aux.Content != nil && string(aux.Content) != "null" {
		var discriminatorDoc struct {
			Type string `json:"type"` // Discriminator field
		}
		if err := json.Unmarshal(aux.Content, &discriminatorDoc); err != nil {
			return fmt.Errorf("could not detect type from field 'content' (content: %s): %w", string(aux.Content), err)
		}

		switch discriminatorDoc.Type {

		case "circle":
			var content *shapes.Circle
			if err := json.Unmarshal(aux.Content, &content); err != nil {
				return fmt.Errorf("failed to unmarshal 'content' as *shapes.Circle for type 'circle' (content: %s): %w", string(aux.Content), err)
			}
			s.Content = content

		case "rectangle":
			var content *shapes.Rectangle
			if err := json.Unmarshal(aux.Content, &content); err != nil {
				return fmt.Errorf("failed to unmarshal 'content' as *shapes.Rectangle for type 'rectangle' (content: %s): %w", string(aux.Content), err)
			}
			s.Content = content

		default:
			if discriminatorDoc.Type == "" {
				return fmt.Errorf("discriminator field 'type' missing or empty in 'content' (content: %s)", string(aux.Content))
			}
			return fmt.Errorf("unknown data type '%s' for field 'content' (content: %s)", discriminatorDoc.Type, string(aux.Content))
		}
	} else {
		s.Content = nil // Explicitly set to nil if null or empty
	}

	return nil
}
