format:
	go run golang.org/x/tools/cmd/goimports@latest -w $(shell find . -name '*.go')
.PHONY: format

test:
	go test ./...
	( cd examples/derivingjson && go test ./... )
.PHONY: test
