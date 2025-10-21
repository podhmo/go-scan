package app

import (
	"ddd_scenario/pkg/domain"
	"ddd_scenario/pkg/infra"
)

func Run() {
	repo := infra.NewRepository()
	uc := NewUseCase(repo)
	uc.Execute()
}

type UseCase struct {
	repo domain.Repository
}

func NewUseCase(repo domain.Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (uc *UseCase) Execute() {
	uc.repo.Save("data")
}
