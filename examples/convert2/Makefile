# Makefile for the convert2 tool and its tests

APP_NAME=convert2
CLI_PATH=./$(APP_NAME)

# Testdata paths
TESTDATA_DIR=./testdata/simple
TEST_OUTPUT_DIR=$(TESTDATA_DIR)
TEST_GEN_FILE=$(TEST_OUTPUT_DIR)/simple_gen.go

# Default target
.PHONY: all
all: test

# Generate Go code using 'go run'
.PHONY: generate_test_code_gorun
generate_test_code_gorun:
	@echo "Generating Go code for tests in $(TESTDATA_DIR) using go run..."
	go run . -input $(TESTDATA_DIR) -output $(TEST_OUTPUT_DIR)
	@echo "Generated code: $(TEST_GEN_FILE)"
	@echo "Formatting generated code $(TEST_GEN_FILE)..."
	@go run golang.org/x/tools/cmd/goimports@latest -w $(TEST_GEN_FILE)

# Run tests in testdata/simple
# Depends on the generated code being present.
.PHONY: test
test: generate_test_code_gorun
	@echo "Running tests in $(TEST_OUTPUT_DIR)..."
	cd $(TEST_OUTPUT_DIR) && go test -v -tags=convert2_test_target .
	@echo "Tests complete."

# Clean up generated files and build artifacts
.PHONY: clean
clean:
	@echo "Cleaning up generated files and build artifacts..."
	rm -f $(TEST_OUTPUT_DIR)/*_gen.go
	rm -f $(TEST_OUTPUT_DIR)/*.test # Go test coverage/profile files if any
	@echo "Cleanup complete."

# Help target to display available commands
.PHONY: help
help:
	@echo "Available commands:"
	@echo "  make all          - Generate code and run tests (default)"
	@echo "  make generate_test_code_gorun - Generate Go code for tests (using go run)"
	@echo "  make test         - Generate code and run tests in $(TESTDATA_DIR)"
	@echo "  make clean        - Remove generated files and build artifacts"

# Explicitly state that these are not file targets
.PHONY: all generate_test_code_gorun test clean help
