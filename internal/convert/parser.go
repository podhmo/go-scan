package convert

import (
	"context"
	"fmt"
	"go/token"
	"reflect"
	"regexp"
	"strings"

	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
)

var reDerivingConvert = regexp.MustCompile(`@derivingconvert\(([^,)]+)\)`)

// Parse parses the package and returns the parsed information.
func Parse(ctx context.Context, pkgpath string) (*ParsedInfo, error) {
	l, err := locator.New(".", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create locator: %w", err)
	}

	pkgDir, err := l.FindPackageDir(pkgpath)
	if err != nil {
		return nil, fmt.Errorf("find package dir %q: %w", pkgpath, err)
	}

	fset := token.NewFileSet()
	s, err := scanner.New(fset, nil, nil, l.ModulePath(), l.RootDir())
	if err != nil {
		return nil, fmt.Errorf("failed to create scanner: %w", err)
	}

	scannedPkg, err := s.ScanPackage(ctx, pkgDir, s)
	if err != nil {
		return nil, fmt.Errorf("scan package %q: %w", pkgDir, err)
	}

	info := &ParsedInfo{
		PackageName: scannedPkg.Name,
		PackagePath: pkgpath,
		Structs:     make(map[string]*StructInfo),
	}

	for _, t := range scannedPkg.Types {
		// Parse struct fields and tags
		if t.Struct != nil {
			structInfo := &StructInfo{
				Name:   t.Name,
				Node:   t,
				Fields: make([]FieldInfo, len(t.Struct.Fields)),
			}
			for i, f := range t.Struct.Fields {
				tag, err := parseConvertTag(f.Tag)
				if err != nil {
					return nil, fmt.Errorf("parsing convert tag for %s.%s: %w", t.Name, f.Name, err)
				}
				structInfo.Fields[i] = FieldInfo{
					Name: f.Name,
					Tag:  tag,
					Node: f,
				}
			}
			info.Structs[t.Name] = structInfo
		}

		// Parse @derivingconvert annotation
		if t.Doc != "" {
			m := reDerivingConvert.FindStringSubmatch(t.Doc)
			if m != nil {
				dstTypeName := m[1]
				dstInfo, err := s.FindType(ctx, pkgDir, dstTypeName)
				if err != nil {
					// could be in another package, try to resolve later
				}

				pair := ConversionPair{
					SrcTypeName: t.Name,
					DstTypeName: dstTypeName,
					SrcInfo:     t,
					DstInfo:     dstInfo,
				}
				info.ConversionPairs = append(info.ConversionPairs, pair)
			}
		}
	}

	// Resolve DstInfo for pairs if not found initially
	for i, pair := range info.ConversionPairs {
		if pair.DstInfo == nil {
			// This logic assumes DstType is in the same module but potentially different package.
			// A more robust solution might need to scan more widely.
			dstInfo, err := s.FindTypeGlobal(ctx, pair.DstTypeName, s)
			if err != nil {
				return nil, fmt.Errorf("could not resolve destination type %q for source %q: %w", pair.DstTypeName, pair.SrcTypeName, err)
			}
			info.ConversionPairs[i].DstInfo = dstInfo
		}
	}

	return info, nil
}

// parseConvertTag parses the `convert:"..."` struct tag.
func parseConvertTag(tag reflect.StructTag) (ConvertTag, error) {
	raw := tag.Get("convert")
	if raw == "" {
		return ConvertTag{}, nil
	}

	parts := strings.Split(raw, ",")
	result := ConvertTag{
		DstFieldName: parts[0],
		RawValue:     raw,
	}

	for _, part := range parts[1:] {
		switch {
		case strings.HasPrefix(part, "using="):
			result.UsingFunc = strings.TrimPrefix(part, "using=")
		case part == "required":
			result.Required = true
		default:
			return ConvertTag{}, fmt.Errorf("unknown convert tag option: %q", part)
		}
	}
	return result, nil
}
