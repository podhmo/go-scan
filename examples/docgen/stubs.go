package main

import "github.com/podhmo/go-scan/scanner"

func createStubOverrides() scanner.ExternalTypeOverride {
	overrides := make(scanner.ExternalTypeOverride)

	// Stub for io.Reader
	ioReaderType := &scanner.TypeInfo{
		Name:    "Reader",
		PkgPath: "io",
		Kind:    scanner.InterfaceKind,
		Interface: &scanner.InterfaceInfo{
			Methods: []*scanner.MethodInfo{
				{Name: "Read"}, // Simplified
			},
		},
	}
	overrides["io.Reader"] = ioReaderType

	// Stub for io.Closer
	ioCloserType := &scanner.TypeInfo{
		Name:    "Closer",
		PkgPath: "io",
		Kind:    scanner.InterfaceKind,
		Interface: &scanner.InterfaceInfo{
			Methods: []*scanner.MethodInfo{
				{Name: "Close"}, // Simplified
			},
		},
	}
	overrides["io.Closer"] = ioCloserType

	// Stub for io.ReadCloser
	ioReadCloserType := &scanner.TypeInfo{
		Name:    "ReadCloser",
		PkgPath: "io",
		Kind:    scanner.InterfaceKind,
		Interface: &scanner.InterfaceInfo{
			Methods: []*scanner.MethodInfo{
				{Name: "Read"},
				{Name: "Close"},
			},
		},
	}
	overrides["io.ReadCloser"] = ioReadCloserType

	// FieldType for io.ReadCloser
	ioReadCloserFieldType := &scanner.FieldType{
		Name:           "ReadCloser",
		PkgName:        "io",
		FullImportPath: "io",
		TypeName:       "ReadCloser",
		Definition:     ioReadCloserType,
	}

	// Stub for url.Values
	urlValuesType := &scanner.TypeInfo{
		Name:    "Values",
		PkgPath: "net/url",
		Kind:    scanner.AliasKind,
		Underlying: &scanner.FieldType{
			IsMap:  true,
			MapKey: &scanner.FieldType{Name: "string", IsBuiltin: true},
			Elem: &scanner.FieldType{
				IsSlice: true,
				Elem:    &scanner.FieldType{Name: "string", IsBuiltin: true},
			},
		},
	}
	overrides["net/url.Values"] = urlValuesType

	// Stub for url.URL
	urlURLType := &scanner.TypeInfo{
		Name:    "URL",
		PkgPath: "net/url",
		Kind:    scanner.StructKind,
		Struct: &scanner.StructInfo{
			Fields: []*scanner.FieldInfo{
				{
					Name: "RawQuery",
					Type: &scanner.FieldType{Name: "string", IsBuiltin: true},
				},
			},
		},
	}
	overrides["net/url.URL"] = urlURLType

	// FieldType for url.URL (as a pointer)
	urlURLFieldType := &scanner.FieldType{
		IsPointer:      true,
		Name:           "URL",
		PkgName:        "url",
		FullImportPath: "net/url",
		TypeName:       "URL",
		Elem: &scanner.FieldType{ // This is the non-pointer version
			Name:           "URL",
			PkgName:        "url",
			FullImportPath: "net/url",
			TypeName:       "URL",
			Definition:     urlURLType,
		},
	}

	// Stub for http.Request
	httpRequestType := &scanner.TypeInfo{
		Name:    "Request",
		PkgPath: "net/http",
		Kind:    scanner.StructKind,
		Struct: &scanner.StructInfo{
			Fields: []*scanner.FieldInfo{
				{
					Name: "URL",
					Type: urlURLFieldType,
				},
				{
					Name: "Body",
					Type: ioReadCloserFieldType,
				},
			},
		},
	}
	overrides["net/http.Request"] = httpRequestType

	return overrides
}
