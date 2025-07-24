package parser

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
)


var reDerivingConvert = regexp.MustCompile(`@derivingconvert\(([^)]+)\)`)

// Parse scans the package for `@derivingconvert` annotations and returns a list of conversion pairs.
func Parse(ctx context.Context, s *goscan.Scanner, pkgpath string) ([]ConversionPair, error) {
	pkgInfo, err := s.ScanPackageByImport(ctx, pkgpath)
	if err != nil {
		return nil, fmt.Errorf("failed to scan package %s: %w", pkgpath, err)
	}
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
		var pkgPath, typeName string
		if lastDotIndex == -1 {
			pkgPath = pkgpath
			typeName = fullDstTypeName
		} else {
			pkgPath = fullDstTypeName[:lastDotIndex]
			typeName = fullDstTypeName[lastDotIndex+1:]
		}

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
			SrcTypeName:      t.Name,
			DstTypeName:      dstType.Name,
			SrcInfo:          t,
			DstInfo:          dstType,
			SrcPkgImportPath: pkgInfo.ImportPath,
			DstPkgImportPath: dstPkgInfo.ImportPath,
		})
	}

	return pairs, nil
}
