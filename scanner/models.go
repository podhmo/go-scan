package scanner

import (
	"context"
	"fmt"
	"go/ast"
	"go/token" // Added
	"reflect"  // Added for reflect.StructTag
	"strings"  // Added for strings.Builder and strings.Fields
	"sync"
)

// TypeParamInfo stores information about a single type parameter.
type TypeParamInfo struct {
	Name       string     `json:"name"`
	Constraint *FieldType `json:"constraint,omitempty"`
}

// Kind defines the category of a type definition.
type Kind int

const (
	StructKind Kind = iota
	AliasKind
	FuncKind
	InterfaceKind
)

// PackageResolver is an interface that can resolve an import path to a package definition.
// It is implemented by the top-level typescanner.Scanner to enable lazy, cached lookups.
type PackageResolver interface {
	ScanPackageByImport(ctx context.Context, importPath string) (*PackageInfo, error)
}

// PackageInfo holds all the extracted information from a single package.
type PackageInfo struct {
	Name       string
	Path       string
	ImportPath string // Added: Canonical import path of the package
	Files      []string
	Types      []*TypeInfo
	Constants  []*ConstantInfo
	Functions  []*FunctionInfo
	Fset       *token.FileSet       // Added: Fileset for position information
	AstFiles   map[string]*ast.File // Added: Parsed AST for each file

	lookupOnce sync.Once
	lookup     map[string]*TypeInfo
}

// Lookup finds a type by name in the package.
func (p *PackageInfo) Lookup(name string) *TypeInfo {
	p.lookupOnce.Do(func() {
		p.lookup = make(map[string]*TypeInfo, len(p.Types))
		for _, t := range p.Types {
			p.lookup[t.Name] = t
		}
	})
	return p.lookup[name]
}

// ExternalTypeOverride defines a mapping from a fully qualified type name
// (e.g., "time.Time") to a pre-defined TypeInfo struct.
// This allows users to provide a "synthetic" type definition for certain types,
// bypassing the need for the scanner to parse them from source. This is particularly
// useful for standard library types that can cause issues when scanned from
// within a test binary, or for any type where manual definition is preferred.
// The key is the fully qualified type name (ImportPath + "." + TypeName).
// The value is a pointer to a scanner.TypeInfo struct that defines the type.
type ExternalTypeOverride map[string]*TypeInfo

// Overlay provides a way to replace the contents of a file with alternative content.
// The key is either a project-relative path (from the module root) or a
// Go package path concatenated with a file name.
type Overlay map[string][]byte

// TypeInfo represents a single type declaration (`type T ...`).
type TypeInfo struct {
	Name       string           `json:"name"`
	PkgPath    string           `json:"pkgPath"`
	FilePath   string           `json:"filePath"`
	Doc        string           `json:"doc,omitempty"`
	Kind       Kind             `json:"kind"`
	TypeParams []*TypeParamInfo `json:"typeParams,omitempty"` // For generic types
	Node       ast.Node         `json:"-"`                    // Avoid cyclic JSON with Node itself.
	Struct     *StructInfo      `json:"struct,omitempty"`
	Func       *FunctionInfo    `json:"func,omitempty"` // For type alias to func type
	Interface  *InterfaceInfo   `json:"interface,omitempty"`
	Underlying *FieldType       `json:"underlying,omitempty"` // For alias types

	// --- Fields for Enum-like patterns ---
	IsEnum      bool            `json:"isEnum,omitempty"`      // True if this type is identified as an enum
	EnumMembers []*ConstantInfo `json:"enumMembers,omitempty"` // List of constants belonging to this enum type
}

// Annotation extracts the value of a specific annotation from the TypeInfo's Doc string.
// Annotations are expected to be in the format "@<name>[:<value>]".
// For example, if Doc contains "@deriving:unmarshall", Annotation("deriving") returns "unmarshall", true.
// If Doc contains "@myannotation", Annotation("myannotation") returns "", true (value is optional).
// If the annotation is not found, it returns "", false.
func (ti *TypeInfo) Annotation(name string) (value string, ok bool) {
	if ti.Doc == "" {
		return "", false
	}
	lines := strings.Split(ti.Doc, "\n")
	prefix := "@" + name
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, prefix) {
			// Found the annotation
			ok = true
			// Check if there's a value after the annotation name
			rest := strings.TrimSpace(strings.TrimPrefix(trimmedLine, prefix))
			if strings.HasPrefix(rest, ":") {
				value = strings.TrimSpace(strings.TrimPrefix(rest, ":"))
			} else if rest == "" {
				// Annotation exists without a value part, e.g. @myannotation
				value = ""
			} else {
				// This case handles annotations like "@deriving:binding in:"body""
				// where the "value" is everything after "@name "
				value = rest
			}
			// Further parsing for specific formats like `in:"body"` can be done by the caller
			// if the raw value after the colon is needed.
			// For `@deriving:binding in:"body"`, this will return `in:"body"` as value for `binding` annotation.
			return value, ok
		}
	}
	return "", false
}

// InterfaceInfo represents an interface type.
type InterfaceInfo struct {
	Methods []*MethodInfo
}

// MethodInfo represents a single method in an interface.
type MethodInfo struct {
	Name       string
	Parameters []*FieldInfo
	Results    []*FieldInfo
	// We might need position or AST node info later if generating code that needs to refer back to the source.
}

// StructInfo represents a struct type.
type StructInfo struct {
	Fields []*FieldInfo
}

// FieldInfo represents a single field in a struct or a parameter/result in a function.
type FieldInfo struct {
	Name     string
	Doc      string
	Type     *FieldType
	Tag      string
	Embedded bool
}

// TagValue extracts the value associated with the given tagName from the struct tag.
// If the tag value contains a comma (e.g., "name,omitempty"), only the part before the comma is returned.
// Returns an empty string if the tag is not present or malformed.
func (fi *FieldInfo) TagValue(tagName string) string {
	if fi.Tag == "" {
		return ""
	}
	tag := reflect.StructTag(fi.Tag)
	tagVal := tag.Get(tagName)
	if commaIdx := strings.Index(tagVal, ","); commaIdx != -1 {
		return tagVal[:commaIdx]
	}
	return tagVal
}

// FieldType represents the type of a field.
type FieldType struct {
	Name         string       `json:"name"`
	PkgName      string       `json:"pkgName,omitempty"` // e.g., "json", "models"
	MapKey       *FieldType   `json:"mapKey,omitempty"`  // For map types
	Elem         *FieldType   `json:"elem,omitempty"`    // For slice, map, pointer, array types
	IsPointer    bool         `json:"isPointer,omitempty"`
	IsSlice      bool         `json:"isSlice,omitempty"`
	IsMap        bool         `json:"isMap,omitempty"`
	IsTypeParam  bool         `json:"isTypeParam,omitempty"`  // True if this FieldType refers to a type parameter
	IsConstraint bool         `json:"isConstraint,omitempty"` // True if this FieldType represents a type constraint
	TypeArgs     []*FieldType `json:"typeArgs,omitempty"`     // For instantiated generic types, e.g., T in List[T]

	Definition         *TypeInfo `json:"-"` // Caches the resolved type definition. Avoid cyclic JSON.
	IsResolvedByConfig bool      `json:"isResolvedByConfig,omitempty"`
	IsBuiltin          bool      `json:"isBuiltin,omitempty"`

	// Resolver, FullImportPath, and TypeName are used for on-demand package scanning.
	// They are exported to allow consumers of the library to construct a resolvable
	// FieldType manually, for instance when parsing type information from an
	// annotation rather than from a Go AST node.
	Resolver       PackageResolver `json:"-"` // For lazy-loading the type definition.
	FullImportPath string          `json:"-"` // Full import path of the type, e.g., "example.com/project/models".
	TypeName       string          `json:"-"` // The name of the type within its package, e.g., "User".
}

// Resolve finds and returns the full definition of the type.
// It uses the PackageResolver to parse other packages on-demand.
// The result is cached for subsequent calls.

// String returns the Go string representation of the field type.
// e.g., "*pkgname.MyType", "[]string", "map[string]int", "MyType[string]"
func (ft *FieldType) String() string {
	if ft == nil {
		return "<nil_FieldType>"
	}
	var sb strings.Builder

	if ft.IsPointer {
		sb.WriteString("*")
		// For pointers, the current parsing for StarExpr in parseTypeExpr does:
		//   underlyingType := s.parseTypeExpr(t.X, currentTypeParams)
		//   underlyingType.IsPointer = true
		//   return underlyingType
		// This means `underlyingType` (which is `ft` here) *is* the element type, but marked as a pointer.
		// So, we write "*" and then format `ft` as if it's not a pointer for the rest of its structure.
		// ft.Elem is primarily for slice/map element types.
	}

	if ft.IsSlice {
		sb.WriteString("[]")
		if ft.Elem != nil {
			sb.WriteString(ft.Elem.String()) // Recursive call for element type
		} else {
			// This case should ideally not happen for valid Go code.
			sb.WriteString("interface{}") // Fallback
		}
		return sb.String() // Slice representation is complete
	}

	if ft.IsMap {
		sb.WriteString("map[")
		if ft.MapKey != nil {
			sb.WriteString(ft.MapKey.String())
		} else {
			sb.WriteString("interface{}") // Fallback
		}
		sb.WriteString("]")
		if ft.Elem != nil {
			sb.WriteString(ft.Elem.String())
		} else {
			sb.WriteString("interface{}") // Fallback
		}
		return sb.String() // Map representation is complete
	}

	// Named types, primitives, or type parameters
	name := ft.Name
	if ft.PkgName != "" && !ft.IsTypeParam { // Type parameters don't have package names like "pkg.T"
		// For qualified types like "pkg.MyType"
		// ft.Name might already be "pkg.MyType" if parsed from SelectorExpr.
		// Or ft.Name is "MyType" and ft.PkgName is "pkg".
		// Prefer ft.TypeName if available (set by SelectorExpr parsing for the base name).
		actualName := ft.Name
		if ft.TypeName != "" {
			actualName = ft.TypeName
		}
		name = fmt.Sprintf("%s.%s", ft.PkgName, actualName)
	}
	sb.WriteString(name)

	// Append type arguments if any, e.g., MyType[T, U]
	if len(ft.TypeArgs) > 0 {
		sb.WriteString("[")
		for i, typeArg := range ft.TypeArgs {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(typeArg.String())
		}
		sb.WriteString("]")
	}

	return sb.String()
}

func (ft *FieldType) Resolve(ctx context.Context, resolving map[string]struct{}) (*TypeInfo, error) {
	// If the definition is already cached (e.g. by an override), return it immediately.
	if ft.Definition != nil {
		return ft.Definition, nil
	}
	if ft.IsResolvedByConfig {
		// This type was marked as resolved by config, but has no pre-supplied definition.
		// This can happen for overrides that map to primitives.
		return nil, nil
	}
	if ft.IsBuiltin {
		// Built-in types are considered resolved without a specific TypeInfo.
		return nil, nil
	}

	// Cannot resolve types without a resolver or if not a built-in.
	if ft.Resolver == nil {
		// For local types (no FullImportPath), we can't resolve further without a resolver.
		// This can happen for non-exported types or if the package context wasn't fully available.
		return nil, fmt.Errorf("type %q cannot be resolved: no resolver available", ft.Name)
	}
	if ft.FullImportPath == "" {
		// This is likely a type from the same package being scanned.
		// Its definition should have been found during the initial scan.
		// If we are here, it means it's a forward declaration or a scenario
		// that doesn't require cross-package resolution.
		return nil, nil // Not an error, just can't resolve further.
	}

	typeIdentifier := ft.FullImportPath + "." + ft.TypeName
	if _, ok := resolving[typeIdentifier]; ok {
		// Cycle detected. Attempt to return the already-allocated TypeInfo pointer
		// to allow the graph to be linked correctly.
		if pkgInfo, err := ft.Resolver.ScanPackageByImport(ctx, ft.FullImportPath); err == nil && pkgInfo != nil {
			if t := pkgInfo.Lookup(ft.TypeName); t != nil {
				ft.Definition = t // Cache the result even in the cycle case
				return t, nil     // Return the existing, partially resolved TypeInfo
			}
		}
		// If we can't find it, returning nil is the last resort, but ideally, the TypeInfo is already in the map.
		return nil, nil
	}
	resolving[typeIdentifier] = struct{}{}
	defer delete(resolving, typeIdentifier)

	pkgInfo, err := ft.Resolver.ScanPackageByImport(ctx, ft.FullImportPath)
	if err != nil {
		return nil, fmt.Errorf("failed to scan package %q for type %q: %w", ft.FullImportPath, ft.TypeName, err)
	}

	// The lookup now uses the efficient map.
	if t := pkgInfo.Lookup(ft.TypeName); t != nil {
		ft.Definition = t // Cache the result
		return t, nil
	}

	return nil, fmt.Errorf("type %q not found in package %q", ft.TypeName, ft.FullImportPath)
}

// ConstantInfo represents a single top-level constant declaration.
type ConstantInfo struct {
	Name       string
	FilePath   string // Added: Absolute path to the file where this const is defined
	Doc        string
	Type       *FieldType // Changed from string to *FieldType
	Value      string
	IsExported bool     // Added to indicate if the constant is exported
	Node       ast.Node // Added: AST node for position, if needed, though FilePath is primary
}

// FunctionInfo represents a single top-level function or method declaration.
type FunctionInfo struct {
	Name       string           `json:"name"`
	FilePath   string           `json:"filePath"`
	Doc        string           `json:"doc,omitempty"`
	Receiver   *FieldInfo       `json:"receiver,omitempty"`
	TypeParams []*TypeParamInfo `json:"typeParams,omitempty"` // For generic functions
	Parameters []*FieldInfo     `json:"parameters,omitempty"`
	Results    []*FieldInfo     `json:"results,omitempty"`
	IsVariadic bool             `json:"isVariadic,omitempty"`
	AstDecl    *ast.FuncDecl    `json:"-"` // Avoid cyclic JSON.
}

// SetResolver is a test helper to overwrite the internal resolver.
func (ft *FieldType) SetResolver(r PackageResolver) {
	ft.Resolver = r
}

// var _ = strings.Builder{} // This helper is no longer needed as "strings" is directly imported.
