format:
	go run golang.org/x/tools/cmd/goimports@latest -w $(shell find . -name '*.go')
.PHONY: format	