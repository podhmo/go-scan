package helpers

// User represents a user in the system.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// GetUser is a helper function that retrieves a user.
// docgen should be able to trace into this function to find the response type.
func GetUser() User {
	return User{ID: 1, Name: "Intra-Module User"}
}
