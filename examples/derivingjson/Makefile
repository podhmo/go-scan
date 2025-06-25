.PHONY: all emit-simple emit-separated clean

all: emit-simple emit-separated

emit-simple:
	@echo "Generating for testdata/simple"
	@go run $(filter-out generator_test.go, $(wildcard *.go)) ./testdata/simple

emit-separated:
	@echo "Generating for testdata/separated/models"
	@go run $(filter-out generator_test.go, $(wildcard *.go)) ./testdata/separated/models

clean:
	@echo "Cleaning generated files..."
	@rm -f testdata/simple/simple_deriving.go
	@rm -f testdata/separated/models/models_deriving.go
