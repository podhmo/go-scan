package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

func (s *UserCreated) MarshalJSON() ([]byte, error) {
	type Alias UserCreated
	return json.Marshal(&struct {
		*Alias
		Type string `json:"type"`
	}{
		Alias: (*Alias)(s),
		Type:  "usercreated",
	})
}

func (s *MessagePosted) MarshalJSON() ([]byte, error) {
	type Alias MessagePosted
	return json.Marshal(&struct {
		*Alias
		Type string `json:"type"`
	}{
		Alias: (*Alias)(s),
		Type:  "messageposted",
	})
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
