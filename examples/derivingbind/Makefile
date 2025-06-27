.PHONY: all generate-simple clean test

all: generate-simple

# Renamed emit-simple to generate-simple for clarity
generate-simple:
	@echo "Generating for testdata/simple"
	@go run main.go testdata/simple

test: generate-simple
	go test ./...
	@echo "Testing testdata/simple..."
	@(cd testdata/simple && go test .)

clean:
	@echo "Cleaning generated files..."
	@rm -f testdata/simple/simple_deriving.go
