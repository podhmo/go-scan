.PHONY: all generate-simple integrationtest-emit clean test

all: generate-simple integrationtest-emit

# Renamed emit-simple to generate-simple for clarity
generate-simple:
	@echo "Generating for testdata/simple"
	@go run main.go ./testdata/simple

integrationtest-emit:
	@echo "Generating for integrationtest"
	@go run main.go ./integrationtest

test: generate-simple integrationtest-emit
	@echo "Running main tests..."
	@go test ./...
	@echo "Testing testdata/simple..."
	@(cd testdata/simple && go test .)
	@echo "Testing integrationtest..."
	@(cd integrationtest && go test .)

clean:
	@echo "Cleaning generated files..."
	@rm -f testdata/simple/simple_deriving.go
	@rm -f integrationtest/integrationtest_deriving.go
