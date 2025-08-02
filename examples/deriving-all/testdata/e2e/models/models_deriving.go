package models

import (
	json "encoding/json"
	errors "errors"
	fmt "fmt"
	io "io"
	http "net/http"
)

func (s *Event) UnmarshalJSON(data []byte) error {
	// Define an alias type to prevent infinite recursion with UnmarshalJSON.
	type Alias Event
	aux := &struct {
		Data json.RawMessage `json:"data"`

		// All other fields will be handled by the standard unmarshaler via the Alias.
		*Alias
	}{
		Alias: (*Alias)(s),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal into aux struct for Event: %w", err)
	}

	// Process Data
	if aux.Data != nil && string(aux.Data) != "null" {
		var discriminatorDoc struct {
			Type string `json:"type"` // Discriminator field
		}
		if err := json.Unmarshal(aux.Data, &discriminatorDoc); err != nil {
			return fmt.Errorf("could not detect type from field 'data' (content: %s): %w", string(aux.Data), err)
		}

		switch discriminatorDoc.Type {

		case "usercreated":
			var content *UserCreated
			if err := json.Unmarshal(aux.Data, &content); err != nil {
				return fmt.Errorf("failed to unmarshal 'data' as *UserCreated for type 'usercreated' (content: %s): %w", string(aux.Data), err)
			}
			s.Data = content

		case "messageposted":
			var content *MessagePosted
			if err := json.Unmarshal(aux.Data, &content); err != nil {
				return fmt.Errorf("failed to unmarshal 'data' as *MessagePosted for type 'messageposted' (content: %s): %w", string(aux.Data), err)
			}
			s.Data = content

		default:
			if discriminatorDoc.Type == "" {
				return fmt.Errorf("discriminator field 'type' missing or empty in 'data' (content: %s)", string(aux.Data))
			}
			return fmt.Errorf("unknown data type '%s' for field 'data' (content: %s)", discriminatorDoc.Type, string(aux.Data))
		}
	} else {
		s.Data = nil // Explicitly set to nil if null or empty
	}

	return nil
}

func (s *User) Bind(req *http.Request, pathVar func(string) string) error {
	var errs []error

	bodyErr := func() error { // Anonymous function to handle body binding logic
		if req.Body != nil && req.Body != http.NoBody {
			var bodyHandledBySpecificField = false

			// If no specific field was designated 'in:"body"', decode into the struct 's' itself.
			if !bodyHandledBySpecificField {
				if decErr := json.NewDecoder(req.Body).Decode(s); decErr != nil {
					if decErr != io.EOF { // EOF might be acceptable if body is optional and empty
						return fmt.Errorf("binding: failed to decode request body into struct User: %w", decErr)
					}
				}
			}
			return nil // Body processed (or EOF ignored)
		} else {
			// Check if body was required.
			isStructOrFieldBodyRequired := false

			if isStructOrFieldBodyRequired {
				return errors.New("binding: request body is required but was not provided or was empty")
			}
		}
		return nil // No body or body not required
	}()
	if bodyErr != nil {
		errs = append(errs, bodyErr)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
