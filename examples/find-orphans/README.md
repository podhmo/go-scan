# Find Orphans

A static analysis tool to find orphan functions and methods in a Go module. An orphan is a function or method that is not used anywhere within the same module.

*(Note: This tool is currently under development.)*

## Usage

```sh
go run ./examples/find-orphans [flags]
```

### Flags

-   `-all`: Scan every package in the module.
-   `--include-tests`: Include usage within test files (`_test.go`).
-   `--workspace-root`: Scan all Go modules found under a given directory.
-   `-v`: Enable verbose output.
