.PHONY: all test format clean generate

all:
	go build ./...

format:
	go run golang.org/x/tools/cmd/goimports@latest -w $(shell find . -name '*.go')

# generate: Runs the convert tool to generate code for the sampledata.
# This serves as a CI check to ensure the generator produces valid, compilable Go code.
generate:
	@echo "--- Generating converter for examples/convert ---"
	@mkdir -p examples/convert/sampledata/generated
	go -C ./examples/convert run . \
		-pkg github.com/podhmo/go-scan/examples/convert/sampledata/source \
		-output sampledata/generated/converter.go \
		-pkgname generated
	@echo "--- Verifying generated code compiles ---"
	go -C ./examples/convert mod tidy
	go -C ./examples/convert build ./sampledata/generated/...

test: generate
	go test ./...
	go -C ./examples/derivingjson test ./...
	go -C ./examples/derivingbind test ./...
	go -C ./examples/minigo test ./...
	go -C ./examples/convert test ./...

clean:
	go clean -cache -testcache # General Go clean
	rm -rf examples/convert/sampledata/generated
	# Example-specific cleaning should be done within their respective Makefiles
	# or by explicitly calling make -C examples/<example_dir> clean
	@echo "Root clean done. For example-specific cleaning, cd into example dir and run make clean."
