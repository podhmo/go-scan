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
func Parse(pkgInfo *scanner.PackageInfo, s *scanner.Scanner) ([]ConversionPair, error) {
	var pairs []ConversionPair

	for _, t := range pkgInfo.Types {
		if t.Doc == "" {
			continue
		}

		matches := reDerivingConvert.FindStringSubmatch(t.Doc)
		if len(matches) < 2 {
			continue
		}

		// The matched string is the full import path and type, e.g., `"example.com/convert/models/destination.DstUser"`
		fullDstTypeName := strings.Trim(strings.TrimSpace(matches[1]), `"`)

		// Use the scanner to find the destination type by its fully qualified name
		dstType, err := s.LookupType(fullDstTypeName)
		if err != nil {
			return nil, fmt.Errorf("could not resolve destination type %q for source type %q: %w", fullDstTypeName, t.Name, err)
		}

		pairs = append(pairs, ConversionPair{
			SrcType: t,
			DstType: dstType,
		})
	}

	return pairs, nil
}
