package domain

// UserRepository is an interface for user repository.
type UserRepository interface {
	Find(id string) (string, error)
}
