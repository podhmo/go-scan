.PHONY: all test format clean

all:
	go build ./...

format:
	go run golang.org/x/tools/cmd/goimports@latest -w $(shell find . -name '*.go')

STDLIB_PKGS= \
	fmt \
	strings \
	encoding/json \
	strconv \
	math/rand \
	time \
	bytes \
	io \
	os \
	regexp \
	text/template \
	errors \
	net/http \
	net/url \
	path/filepath \
	sort

gen-stdlib:
	rm -rf minigo/stdlib/* # clean first
	go run ./examples/minigo-gen-bindings --output minigo/stdlib $(STDLIB_PKGS)

test:
	go test ./...
	go -C ./examples/derivingjson test ./...
	go -C ./examples/derivingbind test ./...
	go -C ./examples/deriving-all test ./...
	go -C ./examples/minigo test ./...
	go -C ./examples/convert test ./...
	go -C ./examples/convert-define test ./...
	go -C ./examples/deps-walk test ./...

test-e2e:
	make -C examples/convert e2e
	make -C examples/convert-define e2e
	make -C examples/deriving-all e2e

clean:
	go clean -cache -testcache # General Go clean
	rm -rf examples/convert/sampledata/generated
	# Example-specific cleaning should be done within their respective Makefiles
	# or by explicitly calling make -C examples/<example_dir> clean
	@echo "Root clean done. For example-specific cleaning, cd into example dir and run make clean."
