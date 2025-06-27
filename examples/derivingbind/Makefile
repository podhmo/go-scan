.PHONY: all emit-simple clean

all: emit-simple

emit-simple:
	@echo "Generating for testdata/simple"
	@go run $(filter-out generator_test.go, $(wildcard *.go)) ./testdata/simple

clean:
	@echo "Cleaning generated files..."
	@rm -f testdata/simple/simple_deriving.go
