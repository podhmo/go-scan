.PHONY: all test format clean

all:
	go build ./...

format:
	go run golang.org/x/tools/cmd/goimports@latest -w $(shell find . -name '*.go')

test:
	go test ./...
	go -C ./examples/derivingjson test ./...
	go -C ./examples/derivingbind test ./...
	go -C ./examples/minigo test ./...
	go -C ./examples/convert test ./...
	go -C ./examples/convert build -o /dev/null

clean:
	go clean -cache -testcache # General Go clean
	# Example-specific cleaning should be done within their respective Makefiles
	# or by explicitly calling make -C examples/<example_dir> clean
	@echo "Root clean done. For example-specific cleaning, cd into example dir and run make clean."
