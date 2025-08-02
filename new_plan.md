1. *Add a build tag to `examples/convert/generated_test/generated_test.go`.*
   - I will add `//go:build e2e` to the top of the file.
2. *Modify `examples/convert/Makefile` to run the `generated_test` with the `e2e` tag.*
   - I will add a new command to the `e2e` target to run `go test -tags e2e ./...`.
3. *Submit the changes.*
   - I will submit the changes with a descriptive commit message.
