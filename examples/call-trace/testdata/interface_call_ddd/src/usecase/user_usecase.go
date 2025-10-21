package usecase

import "github.com/podhmo/go-scan/examples/call-trace/testdata/interface_call_ddd/src/domain"

// UserUsecase provides user related usecases.
type UserUsecase struct {
	Repo domain.UserRepository
}

// GetUserByID gets a user by id.
func (u *UserUsecase) GetUserByID(id string) (string, error) {
	return u.Repo.Find(id)
}
