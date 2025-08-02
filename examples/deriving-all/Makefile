.PHONY: test clean e2e

# run unit tests
test:
	go test -v ./...

# run e2e tests
e2e:
	go run . ./testdata/e2e/models
	go test -v -tags=e2e ./...

# clean generated files
clean:
	rm -f testdata/e2e/models/models_deriving.go
