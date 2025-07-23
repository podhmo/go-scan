package parser

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)

// ConversionPair defines a top-level conversion between two types.
type ConversionPair struct {
	SrcType          *scanner.TypeInfo
	DstType          *scanner.TypeInfo
	DstPkgImportPath string // The import path of the destination type's package.
}

var reDerivingConvert = regexp.MustCompile(`@derivingconvert\(([^)]+)\)`)

// Parse scans the package for `@derivingconvert` annotations and returns a list of conversion pairs.
func Parse(ctx context.Context, pkgInfo *scanner.PackageInfo, s *goscan.Scanner) ([]ConversionPair, error) {
	var pairs []ConversionPair

	for _, t := range pkgInfo.Types {
		if t.Doc == "" {
			continue
		}

		matches := reDerivingConvert.FindStringSubmatch(t.Doc)
		if len(matches) < 2 {
			continue
		}

		fullDstTypeName := strings.Trim(strings.TrimSpace(matches[1]), `"`)

		lastDotIndex := strings.LastIndex(fullDstTypeName, ".")
		if lastDotIndex == -1 {
			return nil, fmt.Errorf("invalid destination type format in @derivingconvert for source type %q: expected 'path/to/pkg.TypeName', got %s", t.Name, fullDstTypeName)
		}

		pkgPath := fullDstTypeName[:lastDotIndex]
		typeName := fullDstTypeName[lastDotIndex+1:]

		dstPkgInfo, err := s.ScanPackageByImport(ctx, pkgPath)
		if err != nil {
			return nil, fmt.Errorf("could not scan package %q for destination type %q: %w", pkgPath, typeName, err)
		}

		var dstType *scanner.TypeInfo
		for _, dstT := range dstPkgInfo.Types {
			if dstT.Name == typeName {
				dstType = dstT
				break
			}
		}

		if dstType == nil {
			return nil, fmt.Errorf("destination type %q not found in package %q for source type %q", typeName, pkgPath, t.Name)
		}

		pairs = append(pairs, ConversionPair{
			SrcType:          t,
			DstType:          dstType,
			DstPkgImportPath: dstPkgInfo.ImportPath,
		})
	}

	return pairs, nil
}
