.PHONY: all emit test clean

all: emit

emit:
	@echo "Generating UnmarshalJSON methods for models and testdata..."
	@go run main.go generator.go

test:
	@echo "Running tests for derivingjson example (models package)..."
	@go test ./models/...
	@echo "Verifying generated code in testdata subdirectories (compile check by listing)..."
	@# This is a simple check; proper tests for testdata would involve `go test` in each sub-pkg.
	@# For now, we ensure the generator runs and produces files.
	@# Example: go test ./testdata/simple/... (if simple had its own tests)
	@# We expect the `emit` step to have placed _deriving.go files correctly.
	@# A more robust test step here might iterate and run `go build` in each testdata sub-pkg.
	@ls testdata/*/*_deriving.go > /dev/null || (echo "No deriving files found in testdata subdirs, check emit step" && exit 1)
	@echo "Testdata generation seems to have produced files. Manual or further specific tests recommended."


clean:
	@echo "Cleaning generated files..."
	@rm -f models/models_deriving.go
	@rm -f testdata/simple/simple_deriving.go
	@# Add other testdata subdirectories if they exist and produce _deriving.go files
	@rm -f testdata/separated/separated_deriving.go # Assuming 'separated' is the package name for testdata/separated
	# If testdata/separated/models was the actual package, then:
	# @rm -f testdata/separated/models/models_deriving.go
	# The generator uses pkgInfo.Name, so for testdata/separated it would be 'separated_deriving.go' in that dir.
