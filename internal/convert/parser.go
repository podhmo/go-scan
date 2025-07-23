package convert

import (
	"context"
	"fmt"
	"go/token"
	"regexp"

	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
)

var reDerivingConvert = regexp.MustCompile(`@derivingconvert\(([^,)]+)\)`)

// Parse parses the package and returns the parsed information.
func Parse(ctx context.Context, pkgpath string) (*ParsedInfo, error) {
	// 1. Use locator to find module info and package directory
	l, err := locator.New(".", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create locator from current directory: %w", err)
	}

	pkgDir, err := l.FindPackageDir(pkgpath)
	if err != nil {
		return nil, fmt.Errorf("could not find package directory for import path %q: %w", pkgpath, err)
	}

	// 2. Create and configure the scanner
	fset := token.NewFileSet()
	s, err := scanner.New(fset, nil, nil, l.ModulePath(), l.RootDir())
	if err != nil {
		return nil, fmt.Errorf("failed to create scanner: %w", err)
	}

	// 3. Scan the located package directory
	scannedPkg, err := s.ScanPackage(ctx, pkgDir, s)
	if err != nil {
		return nil, fmt.Errorf("failed to scan package at %q: %w", pkgDir, err)
	}

	// 4. Parse annotations
	info := &ParsedInfo{
		PackageName: scannedPkg.Name,
		PackagePath: pkgpath,
	}

	for _, t := range scannedPkg.Types {
		if t.Doc == "" {
			continue
		}
		m := reDerivingConvert.FindStringSubmatch(t.Doc)
		if m == nil {
			continue
		}
		pair := ConversionPair{
			SrcTypeName: t.Name,
			DstTypeName: m[1],
		}
		info.ConversionPairs = append(info.ConversionPairs, pair)
	}

	return info, nil
}
