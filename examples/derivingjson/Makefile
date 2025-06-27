.PHONY: all emit test clean

all: emit

emit:
	go run ./ testdata/simple
	go run ./ testdata/separated/models
	go run ./ ./integrationtest

test:
	go test ./...

clean:
	find . -name '*_deriving.go' -delete
