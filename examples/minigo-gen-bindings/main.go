package main

import (
	"context"
	"flag"
	"fmt"
	"go/ast"
	"log"

	"github.com/podhmo/go-scan"
)

func main() {
	var (
		pkgPath = flag.String("pkg", "", "target package path")
	)
	flag.Parse()

	if *pkgPath == "" {
		log.Fatal("-pkg flag is required")
	}

	pkg, err := run(*pkgPath)
	if err != nil {
		log.Fatalf("failed to run: %+v", err)
	}

	fmt.Println("Exported Functions:")
	for _, fn := range pkg.Functions {
		if ast.IsExported(fn.Name) {
			fmt.Println(fn.Name)
		}
	}

	fmt.Println("\nExported Constants:")
	for _, c := range pkg.Constants {
		if ast.IsExported(c.Name) {
			fmt.Println(c.Name)
		}
	}
}

func run(pkgPath string) (*goscan.Package, error) {
	s, err := goscan.New(goscan.WithGoModuleResolver())
	if err != nil {
		return nil, fmt.Errorf("failed to create scanner: %w", err)
	}

	pkg, err := s.ScanPackageByImport(context.Background(), pkgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to scan package: %w", err)
	}

	if pkg == nil {
		return nil, fmt.Errorf("package info is nil")
	}
	return pkg, nil
}
