package parser

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/podhmo/go-scan/scanner"
)

// ConversionPair defines a top-level conversion between two types.
// Corresponds to: @derivingconvert(<DstType>, [option=value, ...])
type ConversionPair struct {
	SrcType *scanner.TypeInfo
	DstType *scanner.TypeInfo
	// TODO: Add options like MaxErrors
}

var reDerivingConvert = regexp.MustCompile(`@derivingconvert\(([^)]+)\)`)

// Parse scans the package for `@derivingconvert` annotations and returns a list of conversion pairs.
func Parse(pkgInfo *scanner.PackageInfo) ([]ConversionPair, error) {
	var pairs []ConversionPair

	// A map to quickly find types by name
	typeMap := make(map[string]*scanner.TypeInfo)
	for _, t := range pkgInfo.Types {
		typeMap[t.Name] = t
	}

	for _, t := range pkgInfo.Types {
		if t.Doc == "" {
			continue
		}

		matches := reDerivingConvert.FindStringSubmatch(t.Doc)
		if len(matches) < 2 {
			continue
		}

		// TODO: Parse options as well, for now just the DstType
		dstTypeName := strings.TrimSpace(matches[1])

		dstType, ok := typeMap[dstTypeName]
		if !ok {
			return nil, fmt.Errorf("destination type %q for source type %q not found in package %s", dstTypeName, t.Name, pkgInfo.Name)
		}

		pairs = append(pairs, ConversionPair{
			SrcType: t,
			DstType: dstType,
		})
	}

	return pairs, nil
}
