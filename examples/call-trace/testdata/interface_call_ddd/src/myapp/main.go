package main

import (
	"fmt"
	"log"

	"github.com/podhmo/go-scan/examples/call-trace/testdata/interface_call_ddd/src/infrastructure"
	"github.com/podhmo/go-scan/examples/call-trace/testdata/interface_call_ddd/src/usecase"
)

func main() {
	repo := infrastructure.NewUserRepository()
	usecase := &usecase.UserUsecase{Repo: repo}

	user, err := usecase.GetUserByID("123")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(user)
}
