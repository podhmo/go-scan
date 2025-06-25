package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) <= 1 {
		fmt.Fprintln(os.Stderr, "Usage: derivingjson <package_path>")
		fmt.Fprintln(os.Stderr, "Example: derivingjson examples/derivingjson/testdata/simple") // Adjusted example path
		os.Exit(1)
	}
	pkgPath := os.Args[1] // Restore command line argument

	// Ensure the package path exists and is a directory
	stat, err := os.Stat(pkgPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: Package path '%s' does not exist.\n", pkgPath)
		} else {
			fmt.Fprintf(os.Stderr, "Error accessing package path '%s': %v\n", pkgPath, err)
		}
		os.Exit(1)
	}
	if !stat.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: Package path '%s' is not a directory.\n", pkgPath)
		os.Exit(1)
	}

	fmt.Printf("Generating UnmarshalJSON for package: %s\n", pkgPath)
	if err := Generate(pkgPath); err != nil { // Generate is in the same package
		fmt.Fprintf(os.Stderr, "Error generating code: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully generated UnmarshalJSON methods for package %s\n", pkgPath)
}
