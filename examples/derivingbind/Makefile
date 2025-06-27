.PHONY: all generate-simple integrationtest-emit clean test

all: generate-simple integrationtest-emit

# Renamed emit-simple to generate-simple for clarity
generate-simple:
	@echo "Generating for testdata/simple"
	# Use 'go run ./' to explicitly run the main package in the current directory.
	# This avoids ambiguity in how 'go run' interprets arguments, especially when
	# the program itself takes .go files or directory paths as arguments.
	# See docs/trouble.md for more context on 'go run' argument parsing.
	@go run ./ ./testdata/simple/models.go

GENERATE_TARGETS := ./integrationtest/models.go ./anotherpkg/models_another.go

integrationtest-emit: # This target name is now a bit misleading, but keeping for consistency unless asked to change.
	@echo "Generating for integrationtest and anotherpkg"
	# Use 'go run ./' for the same reasons as in the generate-simple target.
	@go run ./ $(GENERATE_TARGETS)

test: generate-simple integrationtest-emit
	@echo "Running main tests..."
	@go test ./... # This should cover all sub-packages including anotherpkg

clean:
	@echo "Cleaning generated files..."
	@rm -f ./testdata/simple/simple_deriving.go
	@rm -f ./integrationtest/integrationtest_deriving.go
	@rm -f ./anotherpkg/anotherpkg_deriving.go
