# Package resolver

The `resolver` package provides a cached, on-demand mechanism for loading and scanning Go packages. It acts as a layer on top of the `goscan.Scanner` to ensure that a package is only scanned once, improving performance in applications that may request the same package multiple times.

The primary type is `Resolver`, which is safe for concurrent use.

## Usage

First, create a `goscan.Scanner` instance. Then, use it to initialize a new `resolver.Resolver`.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/resolver"
)

func main() {
	// 1. Create a standard go-scan Scanner
	scanner, err := goscan.New(
		// It's recommended to use the module resolver for most use cases.
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		log.Fatalf("failed to create scanner: %+v", err)
	}

	// 2. Create a new Resolver from the scanner
	r := resolver.New(scanner)

	// 3. Use the resolver to load package info.
	// The first call will scan the "fmt" package.
	fmtPkg, err := r.Resolve(context.Background(), "fmt")
	if err != nil {
		log.Fatalf("failed to resolve package 'fmt': %+v", err)
	}
	fmt.Printf("Successfully loaded package: %s\n", fmtPkg.Name)
	fmt.Printf("  Import Path: %s\n", fmtPkg.ImportPath)
	fmt.Printf("  Functions: %d\n", len(fmtPkg.Functions))

	// The second call for the same package will hit the cache and return
	// the same information without re-scanning the source files.
	fmtPkgFromCache, err := r.Resolve(context.Background(), "fmt")
	if err != nil {
		log.Fatalf("failed to resolve package 'fmt' from cache: %+v", err)
	}

	// The returned pointers will be the same instance
	if fmtPkg == fmtPkgFromCache {
		fmt.Println("\nSuccessfully retrieved package from cache.")
	}
}
```
