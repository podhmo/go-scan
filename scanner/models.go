package scanner

import (
	"fmt"
	"go/ast"
	"go/token" // Added
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
	ScanPackageByImport(importPath string) (*PackageInfo, error)
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
	Underlying *FieldType
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

	resolver       PackageResolver // For lazy-loading the type definition.
	fullImportPath string          // Full import path of the type, e.g., "example.com/project/models".
	typeName       string          // The name of the type within its package, e.g., "User".
}

// Resolve finds and returns the full definition of the type.
// It uses the PackageResolver to parse other packages on-demand.
// The result is cached for subsequent calls.
func (ft *FieldType) Resolve() (*TypeInfo, error) {
	if ft.IsResolvedByConfig {
		// This type was resolved by an external configuration (e.g. to a primitive like "string").
		// There's no further TypeInfo definition to resolve in the Go source.
		// Returning nil, nil indicates that it's "resolved" as far as the config is concerned,
		// and no error occurred, but there isn't a deeper TypeInfo struct.
		// Callers should check IsResolvedByConfig if they need to distinguish this case.
		return nil, nil
	}
	if ft.Definition != nil {
		return ft.Definition, nil
	}

	// Cannot resolve primitive types or types from the same package without a resolver.
	if ft.resolver == nil || ft.fullImportPath == "" {
		return nil, fmt.Errorf("type %q cannot be resolved: no resolver or import path available", ft.Name)
	}

	pkgInfo, err := ft.resolver.ScanPackageByImport(ft.fullImportPath)
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
}
