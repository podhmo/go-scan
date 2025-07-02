package scanner

import (
	"context"
	"fmt"
	"go/ast"
	"go/token" // Added
	"reflect"  // Added for reflect.StructTag
	"strings"  // Added for strings.Builder and strings.Fields
)

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
	Fset       *token.FileSet // Added: Fileset for position information
	AstFiles   map[string]*ast.File // Added: Parsed AST for each file
}

// ExternalTypeOverride defines a mapping from a fully qualified type name
// (e.g., "github.com/google/uuid.UUID") to a target Go type string (e.g., "string").
// This allows users to specify how types from external packages (or even internal ones)
// should be interpreted by the scanner, overriding the default parsing behavior.
// For instance, if you want all instances of `uuid.UUID` to be treated as `string`
// in your scanned output, you would provide a mapping like:
// {"github.com/google/uuid.UUID": "string"}
// The key is the fully qualified type name (ImportPath + "." + TypeName).
// The value is the desired Go type string.
type ExternalTypeOverride map[string]string

// TypeInfo represents a single type declaration (`type T ...`).
type TypeInfo struct {
	Name       string
	FilePath   string // Added: Absolute path to the file where this type is defined
	Doc        string
	Kind       Kind
	Node       ast.Node
	Struct     *StructInfo
	Func       *FunctionInfo
	Interface  *InterfaceInfo // Added for interface types
	Underlying *FieldType
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
				// This case handles annotations like "@derivng:binding in:"body""
				// where the "value" is everything after "@name "
				value = rest
			}
			// Further parsing for specific formats like `in:"body"` can be done by the caller
			// if the raw value after the colon is needed.
			// For `@derivng:binding in:"body"`, this will return `in:"body"` as value for `binding` annotation.
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
	Name               string
	PkgName            string
	MapKey             *FieldType
	Elem               *FieldType
	IsPointer          bool
	IsSlice            bool
	IsMap              bool
	Definition         *TypeInfo // Caches the resolved type definition.
	IsResolvedByConfig bool      // True if this type was resolved using ExternalTypeOverrides
	IsBuiltin          bool      // True if this type is a Go built-in type

	resolver       PackageResolver // For lazy-loading the type definition.
	fullImportPath string          // Full import path of the type, e.g., "example.com/project/models".
	typeName       string          // The name of the type within its package, e.g., "User".
}

// FullImportPath returns the fully qualified import path if this type is from an external package.
// Returns an empty string if the type is local or not from a qualified package.
func (ft *FieldType) FullImportPath() string {
	return ft.fullImportPath
}

// Resolve finds and returns the full definition of the type.
// It uses the PackageResolver to parse other packages on-demand.
// The result is cached for subsequent calls.

// String returns the Go string representation of the field type.
// e.g., "*pkgname.MyType", "[]string", "map[string]int"
func (ft *FieldType) String() string {
	if ft == nil {
		return "<nil_FieldType>"
	}
	var sb strings.Builder

	if ft.IsPointer {
		sb.WriteString("*")
		if ft.Elem != nil {
			// If Elem exists, it represents the pointed-to type.
			// Recursively call String on Elem.
			// However, current FieldType for pointer stores base type in Name and IsPointer=true. Elem might be for base type's own Elem if it's slice/map.
			// Let's assume ft.Name is the base name if IsPointer is true.
			// The String() method should ideally be called on the Elem if it represents the next part of type.
			// Current structure: For *T, Name="T", IsPointer=true. For []*T, Name="slice", IsPointer=false, IsSlice=true, Elem points to *T.
			// This means for a simple pointer, we write "*" and then handle the ft.Name or ft.Elem based on what ft represents.

			// If ft.Name is already qualified or is a primitive, use it.
			// This part needs careful handling of how PkgName/fullImportPath interact with Name for pointers.
			// For now, let's assume if IsPointer is true, ft.Name is the base type name.
			// A more robust way: if ft.Elem is non-nil and represents the pointed-to type structure, call ft.Elem.String().
			// But `parseTypeExpr` for StarExpr does: `underlyingType := s.parseTypeExpr(t.X); underlyingType.IsPointer = true; return underlyingType;`
			// This implies Name is from t.X.
			// So, if IsPointer, we add "*" and then proceed to format the rest of ft as if it were not a pointer.
		}
		// Fallthrough to handle the base type (slice, map, or named type)
	}

	if ft.IsSlice {
		sb.WriteString("[]")
		if ft.Elem != nil {
			sb.WriteString(ft.Elem.String()) // Recursive call for element type
		} else {
			sb.WriteString("interface{}") // Fallback, should ideally not happen for valid code
		}
		return sb.String()
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
		return sb.String()
	}

	// If not a pointer (already handled for the prefix), slice, or map, it's a named type or primitive.
	// Prepend PkgName if it exists (for qualified types).
	name := ft.Name
	if ft.PkgName != "" { // PkgName is the local identifier (e.g. "json" or "m" in "m.MyType")
		// To get the canonical import path for prefixing, we'd ideally use fullImportPath's last segment
		// if PkgName is just an alias or could be ambiguous.
		// For now, assume PkgName is sufficient for qualification as used by the parser.
		name = fmt.Sprintf("%s.%s", ft.PkgName, ft.typeName) // ft.typeName is the Name within the package
	}
	sb.WriteString(name)
	return sb.String()
}

func (ft *FieldType) Resolve(ctx context.Context) (*TypeInfo, error) {
	if ft.IsResolvedByConfig {
		// This type was resolved by an external configuration (e.g. to a primitive like "string").
		// There's no further TypeInfo definition to resolve in the Go source.
		// Returning nil, nil indicates that it's "resolved" as far as the config is concerned,
		// and no error occurred, but there isn't a deeper TypeInfo struct.
		// Callers should check IsResolvedByConfig if they need to distinguish this case.
		return nil, nil
	}
	if ft.IsBuiltin {
		// Built-in types are considered resolved without a specific TypeInfo.
		return nil, nil
	}
	if ft.Definition != nil {
		return ft.Definition, nil
	}

	// Cannot resolve types from the same package without a resolver or if not a built-in.
	if ft.resolver == nil || ft.fullImportPath == "" {
		return nil, fmt.Errorf("type %q cannot be resolved: no resolver or import path available", ft.Name)
	}

	pkgInfo, err := ft.resolver.ScanPackageByImport(ctx, ft.fullImportPath)
	if err != nil {
		return nil, fmt.Errorf("failed to scan package %q for type %q: %w", ft.fullImportPath, ft.typeName, err)
	}

	for _, t := range pkgInfo.Types {
		if t.Name == ft.typeName {
			ft.Definition = t // Cache the result
			return t, nil
		}
	}

	return nil, fmt.Errorf("type %q not found in package %q", ft.typeName, ft.fullImportPath)
}

// ConstantInfo represents a single top-level constant declaration.
type ConstantInfo struct {
	Name     string
	FilePath string // Added: Absolute path to the file where this const is defined
	Doc      string
	Type     string
	Value    string
	Node     ast.Node // Added: AST node for position, if needed, though FilePath is primary
}

// FunctionInfo represents a single top-level function or method declaration.
type FunctionInfo struct {
	Name       string
	FilePath   string // Added: Absolute path to the file where this func is defined
	Doc        string
	Receiver   *FieldInfo
	Parameters []*FieldInfo
	Results    []*FieldInfo
	Node       *ast.FuncDecl // Added: AST node for the function declaration
}

// var _ = strings.Builder{} // This helper is no longer needed as "strings" is directly imported.
