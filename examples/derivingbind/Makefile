.PHONY: all emit-simple emit-integrationtest test clean

all: emit-simple emit-integrationtest

emit-simple:
	@echo "Generating for testdata/simple"
	@go run ./main.go ./testdata/simple

emit-integrationtest:
	@echo "Generating for integrationtest"
	@go run ./main.go ./integrationtest

test: emit-integrationtest
	@echo "Running integration tests..."
	@go test ./integrationtest/...
	@echo "Running simple tests..."
	@go test ./testdata/simple/...

clean:
	@echo "Cleaning generated files..."
	@rm -f ./testdata/simple/simple_deriving.go
	@rm -f ./integrationtest/integrationtest_deriving.go
