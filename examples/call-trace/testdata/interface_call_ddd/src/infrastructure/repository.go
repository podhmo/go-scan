package infrastructure

import "github.com/podhmo/go-scan/examples/call-trace/testdata/interface_call_ddd/src/domain"

// UserRepositoryImpl is a concrete implementation of UserRepository.
type UserRepositoryImpl struct{}

// Find finds a user by id.
func (r *UserRepositoryImpl) Find(id string) (string, error) {
	return "user-" + id, nil
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository() domain.UserRepository {
	return &UserRepositoryImpl{}
}
