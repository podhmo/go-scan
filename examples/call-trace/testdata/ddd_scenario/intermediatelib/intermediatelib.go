package intermediatelib

import "##WORKDIR##/examples/call-trace/testdata/ddd_scenario/mylib"

type Usecase struct {
	Repo mylib.Repository
}

func (u *Usecase) Run(id string) string {
	return u.Repo.Get(id)
}
