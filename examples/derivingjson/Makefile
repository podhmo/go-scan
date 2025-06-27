.PHONY: all emit test clean

all: emit

emit:
	go run ./ $(shell find testdata/simple -maxdepth 1 -name '*.go' ! -name '*_test.go' ! -name '*_deriving.go')
	go run ./ $(shell find testdata/separated/models -maxdepth 1 -name '*.go' ! -name '*_test.go' ! -name '*_deriving.go')
	go run ./ $(shell find ./integrationtest -maxdepth 1 -name '*.go' ! -name '*_test.go' ! -name '*_deriving.go' | paste -sd " ")

test:
	go test ./...

clean:
	find . -name '*_deriving.go' -delete
	find . -name '*_derivingjson.go' -delete
