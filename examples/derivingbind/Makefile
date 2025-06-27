.PHONY: all emit-simple clean test

all: emit-simple

emit-simple:
	@echo "Generating for testdata/simple"
	@go run ./ ./testdata/simple

test:
	go test ./...

clean:
	@echo "Cleaning generated files..."
	@rm -f testdata/simple/simple_deriving.go
