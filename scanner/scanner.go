package scanner

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath" // Added for filepath.Join
	"strings"
)

// Scanner parses Go source files within a package.
type Scanner struct {
	fset                  *token.FileSet // FileSet to use for parsing. Must be provided.
	resolver              PackageResolver
	importLookup          map[string]string // Maps import alias/name to full import path for the current file.
	ExternalTypeOverrides ExternalTypeOverride
	Overlay               Overlay
	modulePath            string
	moduleRootDir         string
}

// New creates a new Scanner.
// The fset must be provided and is used for all parsing operations by this scanner instance.
func New(fset *token.FileSet, overrides ExternalTypeOverride, overlay Overlay, modulePath string, moduleRootDir string) (*Scanner, error) {
	if fset == nil {
		return nil, fmt.Errorf("fset cannot be nil")
	}
	if overrides == nil {
		overrides = make(ExternalTypeOverride)
	}
	if overlay == nil {
		overlay = make(Overlay)
	}
	if modulePath == "" || moduleRootDir == "" {
		return nil, fmt.Errorf("modulePath and moduleRootDir must be provided")
	}

	return &Scanner{
		fset:                  fset,
		ExternalTypeOverrides: overrides,
		Overlay:               overlay,
		modulePath:            modulePath,
		moduleRootDir:         moduleRootDir,
	}, nil
}

// ResolveType starts the type resolution process for a given field type.
// It handles circular dependencies by tracking the resolution path.
// It's the public entry point for resolving types, initializing a new resolution tracker.
func (s *Scanner) ResolveType(ctx context.Context, fieldType *FieldType) (*TypeInfo, error) {
	// The internal Resolve method is called with a new, empty map for tracking.
	return fieldType.Resolve(ctx, make(map[string]struct{}))
}

// ScanPackage parses all .go files in a given directory and returns PackageInfo.
// It now uses ScanFiles internally.
func (s *Scanner) ScanPackage(ctx context.Context, dirPath string, resolver PackageResolver) (*PackageInfo, error) {
	s.resolver = resolver // Store resolver for use by parseTypeExpr etc.

	// List all .go files in the directory, excluding _test.go files.
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	var filePaths []string
	for _, entry := range dirEntries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			filePaths = append(filePaths, filepath.Join(dirPath, entry.Name()))
		}
	}

	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no buildable Go source files in %s", dirPath)
	}

	// Delegate to ScanFiles.
	// pkgImportPath for ScanFiles could be derived or passed in.
	// For now, let ScanFiles derive it or handle it based on its needs.
	// dirPath itself serves as the package's unique path identifier for PackageInfo.Path.
	return s.ScanFiles(ctx, filePaths, dirPath, resolver)
}

// ScanFiles parses a specific list of .go files and returns PackageInfo.
// pkgDirPath is the absolute directory path for this package, used for PackageInfo.Path.
func (s *Scanner) ScanFiles(ctx context.Context, filePaths []string, pkgDirPath string, resolver PackageResolver) (*PackageInfo, error) { // Added ctx to parseFuncDecl call
	s.resolver = resolver // Ensure resolver is set for this scanning operation.

	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no files provided to scan for package at %s", pkgDirPath)
	}

	info := &PackageInfo{
		Path:     pkgDirPath, // Physical directory path
		Fset:     s.fset,     // Use the shared FileSet
		Files:    make([]string, 0, len(filePaths)),
		AstFiles: make(map[string]*ast.File), // Initialize AstFiles
	}
	var firstPackageName string

	for _, filePath := range filePaths {
		// filePath here is absolute.
		var content any
		if s.Overlay != nil {
			relPath, err := filepath.Rel(s.moduleRootDir, filePath)
			if err == nil {
				if overlayContent, ok := s.Overlay[relPath]; ok {
					content = overlayContent
				}
			}
		}

		fileAst, err := parser.ParseFile(s.fset, filePath, content, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("failed to parse file %s: %w", filePath, err)
		}

		if info.Name == "" {
			info.Name = fileAst.Name.Name
			firstPackageName = fileAst.Name.Name
		} else if fileAst.Name.Name != firstPackageName {
			return nil, fmt.Errorf("mismatched package names: %s and %s in directory %s",
				firstPackageName, fileAst.Name.Name, pkgDirPath)
		}

		info.Files = append(info.Files, filePath) // Store absolute file path
		info.AstFiles[filePath] = fileAst         // Store AST
		slog.DebugContext(ctx, "Processing file for package", slog.String("filePath", filePath), slog.String("packageName", info.Name))
		s.buildImportLookup(fileAst)
		slog.DebugContext(ctx, "Built import lookup", slog.String("filePath", filePath), slog.Any("imports", s.importLookup))
		for declIndex, decl := range fileAst.Decls {
			slog.DebugContext(ctx, "Processing declaration", slog.String("filePath", filePath), slog.Int("declIndex", declIndex), slog.String("type", fmt.Sprintf("%T", decl)))
			switch d := decl.(type) {
			case *ast.GenDecl:
				slog.DebugContext(ctx, "Processing GenDecl", slog.String("token", d.Tok.String()), slog.String("filePath", filePath), slog.Int("specs", len(d.Specs)))
				s.parseGenDecl(ctx, d, info, filePath) // Pass context
			case *ast.FuncDecl:
				slog.DebugContext(ctx, "Processing FuncDecl", slog.String("name", d.Name.Name), slog.String("filePath", filePath))
				info.Functions = append(info.Functions, s.parseFuncDecl(ctx, d, filePath, info)) // Pass ctx and pkgInfo
			}
		}
	}
	if info.Name == "" && len(filePaths) > 0 {
		// This case should ideally not be reached if ParseFile succeeds and files are valid Go files.
		return nil, fmt.Errorf("could not determine package name from scanned files in %s", pkgDirPath)
	}
	if len(info.Files) == 0 { // Should be redundant given the initial check, but as a safeguard.
		return nil, fmt.Errorf("no buildable Go source files processed in %s", pkgDirPath)
	}

	return info, nil
}

// buildImportLookup populates the importLookup map for the current file.
func (s *Scanner) buildImportLookup(file *ast.File) {
	s.importLookup = make(map[string]string)
	for _, i := range file.Imports {
		path := strings.Trim(i.Path.Value, `"`)
		if i.Name != nil {
			s.importLookup[i.Name.Name] = path
		} else {
			parts := strings.Split(path, "/")
			s.importLookup[parts[len(parts)-1]] = path
		}
	}
}

// parseGenDecl parses a general declaration (types, constants, variables).
func (s *Scanner) parseGenDecl(ctx context.Context, decl *ast.GenDecl, info *PackageInfo, absFilePath string) {
	for _, spec := range decl.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			typeInfo := s.parseTypeSpec(ctx, sp, absFilePath)
			if typeInfo.Doc == "" && decl.Doc != nil {
				typeInfo.Doc = commentText(decl.Doc)
			}
			slog.DebugContext(ctx, "Parsed TypeSpec, adding to PackageInfo", slog.String("name", typeInfo.Name), slog.Any("kind", typeInfo.Kind), slog.String("filePath", typeInfo.FilePath), slog.Int("currentTypesCount", len(info.Types)))
			info.Types = append(info.Types, typeInfo)
		case *ast.ValueSpec:
			if decl.Tok == token.CONST {
				doc := commentText(sp.Doc)
				if doc == "" && sp.Comment != nil {
					doc = commentText(sp.Comment)
				}
				if doc == "" && decl.Doc != nil {
					doc = commentText(decl.Doc)
				}
				for i, name := range sp.Names {
					var val string
					var inferredFieldType *FieldType // For type inference

					if i < len(sp.Values) {
						valueExpr := sp.Values[i]
						if lit, ok := valueExpr.(*ast.BasicLit); ok {
							val = lit.Value
							// Infer type from value if sp.Type is nil
							switch lit.Kind {
							case token.STRING:
								inferredFieldType = &FieldType{Name: "string", IsBuiltin: true}
							case token.INT:
								inferredFieldType = &FieldType{Name: "int", IsBuiltin: true}
							case token.FLOAT:
								inferredFieldType = &FieldType{Name: "float64", IsBuiltin: true}
							case token.CHAR:
								inferredFieldType = &FieldType{Name: "rune", IsBuiltin: true}
							default:
								slog.WarnContext(ctx, "Unhandled BasicLit kind for constant type inference", slog.String("kind", lit.Kind.String()), slog.String("const_name", name.Name), slog.String("filePath", absFilePath))
							}
						} else {
							slog.InfoContext(ctx, "Constant value is not a BasicLit, type inference might be limited", slog.String("const_name", name.Name), slog.String("value_type", fmt.Sprintf("%T", valueExpr)), slog.String("filePath", absFilePath))
						}
					}

					var finalFieldType *FieldType
					if sp.Type != nil { // Explicit type is present
						finalFieldType = s.parseTypeExpr(ctx, sp.Type, nil) // Pass ctx and nil for currentTypeParams
					} else { // No explicit type, use inferred type
						finalFieldType = inferredFieldType
					}

					info.Constants = append(info.Constants, &ConstantInfo{
						Name:       name.Name,
						FilePath:   absFilePath,
						Doc:        doc,
						Value:      val,
						Type:       finalFieldType, // Use the determined field type
						IsExported: name.IsExported(),
						Node:       name,
					})
				}
			}
		}
	}
}

// parseTypeSpec parses a type specification.
func (s *Scanner) parseTypeSpec(ctx context.Context, sp *ast.TypeSpec, absFilePath string) *TypeInfo {
	typeInfo := &TypeInfo{
		Name:     sp.Name.Name,
		FilePath: absFilePath,
		Doc:      commentText(sp.Doc),
		Node:     sp,
	}

	// Parse type parameters if they exist (Go 1.18+)
	if sp.TypeParams != nil {
		typeInfo.TypeParams = s.parseTypeParamList(ctx, sp.TypeParams.List)
	}

	switch t := sp.Type.(type) {
	case *ast.StructType:
		typeInfo.Kind = StructKind
		typeInfo.Struct = s.parseStructType(ctx, t, typeInfo.TypeParams) // Pass ctx and type params for context
	case *ast.InterfaceType:
		typeInfo.Kind = InterfaceKind
		// Interfaces themselves cannot have type parameters in the same way structs/funcs do,
		// but their methods might involve types that are generic or type parameters from an outer scope.
		// However, sp.TypeParams would be for the interface itself if it were allowed.
		// For now, we assume interface definitions don't have sp.TypeParams in a way that affects `s.parseInterfaceType` directly for the interface's own params.
		typeInfo.Interface = s.parseInterfaceType(ctx, t, typeInfo.TypeParams) // Pass type params for context
	case *ast.FuncType:
		typeInfo.Kind = FuncKind
		// A type alias to a func type, e.g. "type MyFunc[T any] func(T) T"
		// The type parameters for `MyFunc` are on `sp.TypeParams`.
		// The parameters/results of the func type `t` might use these type parameters.
		typeInfo.Func = s.parseFuncType(ctx, t, typeInfo.TypeParams) // Pass ctx and type params for context
		// Note: If `typeInfo.Func.TypeParams` is to be filled, it should come from `sp.TypeParams`.
		// `parseFuncType` might need to be adjusted or this assignment handled differently.
		// For a type alias `type MyFunc[T any] = func(p T) T`, `sp.TypeParams` defines `[T any]`.
		// `t` is `func(p T) T`. `parseFuncType` needs to know about `T` from `sp.TypeParams`.
		// Let's ensure `parseFuncType` correctly uses the passed `currentTypeParams`.
		// If `typeInfo.Func` is to have its own `TypeParams` field mirroring `typeInfo.TypeParams`,
		// this needs explicit assignment or `parseFuncType` needs to set it based on context.
		// For now, `typeInfo.TypeParams` holds the params for the TypeSpec.
	default:
		typeInfo.Kind = AliasKind
		// For aliases like `type MySlice[T any] []T`, `sp.TypeParams` holds `[T any]`.
		// `sp.Type` (the RHS) is `[]T`. `parseTypeExpr` needs to know about `T`.
		typeInfo.Underlying = s.parseTypeExpr(ctx, sp.Type, typeInfo.TypeParams) // Pass ctx and type params
	}
	return typeInfo
}

// parseTypeParamList parses a list of ast.Field representing type parameters.
func (s *Scanner) parseTypeParamList(ctx context.Context, typeParamFields []*ast.Field) []*TypeParamInfo {
	var params []*TypeParamInfo
	if typeParamFields == nil {
		return nil
	}
	for _, typeParamField := range typeParamFields { // Each ast.Field in TypeParams.List
		constraintExpr := typeParamField.Type
		var constraintFieldType *FieldType
		if constraintExpr != nil {
			// When parsing constraints, these type parameters are not yet in scope for themselves.
			// Pass nil or an empty list for currentTypeParams for now.
			// Outer scope type parameters could be relevant if nested generics were common,
			// but for typical Go usage, nil is safe here.
			constraintFieldType = s.parseTypeExpr(ctx, constraintExpr, nil) // Pass ctx, No currentTypeParams for the constraint itself
			if constraintFieldType != nil {
				constraintFieldType.IsConstraint = true
			}
		}
		// typeParamField.Names contains the actual type parameter names (e.g., T, K, V)
		for _, nameIdent := range typeParamField.Names {
			params = append(params, &TypeParamInfo{
				Name:       nameIdent.Name,
				Constraint: constraintFieldType,
			})
		}
	}
	return params
}

// parseInterfaceType parses an interface type.
// It now accepts currentTypeParams for resolving type parameter references in method signatures.
func (s *Scanner) parseInterfaceType(ctx context.Context, it *ast.InterfaceType, currentTypeParams []*TypeParamInfo) *InterfaceInfo {
	if it.Methods == nil || len(it.Methods.List) == 0 {
		return &InterfaceInfo{Methods: []*MethodInfo{}} // Empty interface
	}
	interfaceInfo := &InterfaceInfo{
		Methods: make([]*MethodInfo, 0, len(it.Methods.List)),
	}
	for _, field := range it.Methods.List {
		if len(field.Names) > 0 { // Method signature
			methodName := field.Names[0].Name
			funcType, ok := field.Type.(*ast.FuncType)
			if !ok {
				slog.WarnContext(ctx, "Expected FuncType for interface method, skipping", slog.String("method_name", methodName), slog.String("got_type", fmt.Sprintf("%T", field.Type)))
				continue
			}
			methodInfo := &MethodInfo{
				Name: methodName,
				// The `currentTypeParams` for `parseFuncType` here should be those of the *interface's* scope,
				// if interfaces could be generic themselves in this way.
				// Since they can't, `currentTypeParams` refers to an outer scope (e.g. a generic type embedding this interface def).
				// Or, if this interface is a constraint in a generic func/type, `currentTypeParams` are from that func/type.
				// For `type Stringer[T any] interface { String(T) string }`, `T` is from `Stringer`.
				// Go doesn't support this directly on interface decls, but this structure allows it if AST did.
				// What `parseFuncType` returns is a `FunctionInfo`, we need to extract params/results.
				// Let's make a helper or inline parsing of params/results.
			}
			parsedFuncDetails := s.parseFuncType(ctx, funcType, currentTypeParams) // Pass ctx and currentTypeParams
			methodInfo.Parameters = parsedFuncDetails.Parameters
			methodInfo.Results = parsedFuncDetails.Results

			interfaceInfo.Methods = append(interfaceInfo.Methods, methodInfo)
		} else {
			// Embedded interface or type constraint element
			// e.g. `interface { MyInterface; ~int; comparable }`
			// field.Type is the expression (Ident, SelectorExpr for MyInterface; BinaryExpr for ~int)
			embeddedType := s.parseTypeExpr(ctx, field.Type, currentTypeParams) // Pass ctx and currentTypeParams
			// TODO: Handle embedded interfaces/constraints more explicitly if needed.
			// For now, we represent it as a "method" with this type.
			// This might need a dedicated field in InterfaceInfo or MethodInfo.
			// For `derivingjson`, we primarily care about actual methods.
			// If it's a constraint element like `~int` or `comparable`, `embeddedType.IsConstraint` should be true.
			// We can add it as a special "method" or to a new list of "ConstraintElements".
			// Let's create a placeholder method for now.
			interfaceInfo.Methods = append(interfaceInfo.Methods, &MethodInfo{
				Name:       fmt.Sprintf("embedded_%s", embeddedType.String()), // Placeholder name
				Parameters: nil,                                               // Not a real method signature in the same way
				Results:    []*FieldInfo{{Type: embeddedType}},                // Store the type here
			})
			slog.InfoContext(ctx, "Embedded interface/constraint in interface definition", slog.String("type", embeddedType.String()))
		}
	}
	return interfaceInfo
}

// parseStructType parses a struct type.
// It now accepts currentTypeParams for resolving type parameter references in field types.
func (s *Scanner) parseStructType(ctx context.Context, st *ast.StructType, currentTypeParams []*TypeParamInfo) *StructInfo {
	structInfo := &StructInfo{}
	for _, field := range st.Fields.List {
		fieldType := s.parseTypeExpr(ctx, field.Type, currentTypeParams) // Pass ctx and currentTypeParams
		var tag string
		if field.Tag != nil {
			tag = strings.Trim(field.Tag.Value, "`")
		}
		doc := commentText(field.Doc)
		if doc == "" {
			doc = commentText(field.Comment)
		}
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				structInfo.Fields = append(structInfo.Fields, &FieldInfo{
					Name: name.Name,
					Doc:  doc,
					Type: fieldType,
					Tag:  tag,
				})
			}
		} else { // Embedded field
			structInfo.Fields = append(structInfo.Fields, &FieldInfo{
				Name:     fieldType.Name, // For embedded, field name is type name
				Doc:      doc,
				Type:     fieldType,
				Tag:      tag,
				Embedded: true,
			})
		}
	}
	return structInfo
}

// parseFuncDecl parses a function declaration.
func (s *Scanner) parseFuncDecl(ctx context.Context, f *ast.FuncDecl, absFilePath string, pkgInfo *PackageInfo) *FunctionInfo {
	var funcOwnTypeParams []*TypeParamInfo // Renamed from currentTypeParams to avoid confusion
	if f.Type.TypeParams != nil {
		funcOwnTypeParams = s.parseTypeParamList(ctx, f.Type.TypeParams.List) // Pass ctx
	}

	// Initial parse of func type to get its basic structure (params, results)
	// At this stage, for method parameters/results, type parameter context might be incomplete.
	// We use funcOwnTypeParams here, which are type parameters of the function/method itself.
	funcInfo := s.parseFuncType(ctx, f.Type, funcOwnTypeParams) // Use funcOwnTypeParams

	funcInfo.Name = f.Name.Name
	funcInfo.FilePath = absFilePath
	funcInfo.Doc = commentText(f.Doc)
	funcInfo.AstDecl = f
	funcInfo.TypeParams = funcOwnTypeParams // Assign parsed type parameters of the function itself

	if f.Recv != nil && len(f.Recv.List) > 0 { // This is a method
		recvField := f.Recv.List[0]
		var recvName string
		if len(recvField.Names) > 0 {
			recvName = recvField.Names[0].Name
		}

		// Attempt to find the receiver's base type information in the current package
		var receiverBaseTypeParams []*TypeParamInfo
		// First, parse the receiver expression to get its name and any explicit type arguments
		// Pass funcOwnTypeParams as current context, though receiver type args usually refer to struct's params.
		// This initial parse helps identify the structure.
		parsedRecvFieldType := s.parseTypeExpr(ctx, recvField.Type, funcOwnTypeParams)

		if parsedRecvFieldType != nil {
			baseRecvTypeName := parsedRecvFieldType.Name
			if parsedRecvFieldType.IsPointer && parsedRecvFieldType.Elem != nil {
				baseRecvTypeName = parsedRecvFieldType.Elem.Name
			}
			// Remove package qualifier if present, assuming methods are on types in the same package for now.
			if parts := strings.Split(baseRecvTypeName, "."); len(parts) > 1 {
				baseRecvTypeName = parts[len(parts)-1]
			}

			if pkgInfo != nil { // pkgInfo is passed to parseFuncDecl
				for _, ti := range pkgInfo.Types {
					if ti.Name == baseRecvTypeName {
						receiverBaseTypeParams = ti.TypeParams // These are the TPs of the struct (e.g. T from List[T])
						// Now, re-parse the receiver type with its own struct's type parameters as context
						// so that T in *List[T] can be marked as IsTypeParam.
						parsedRecvFieldType = s.parseTypeExpr(ctx, recvField.Type, receiverBaseTypeParams)
						break
					}
				}
			}
		}

		funcInfo.Receiver = &FieldInfo{
			Name: recvName,
			Type: parsedRecvFieldType, // Use the re-parsed (or initially parsed if not found) receiver field type
		}

		// For parameters and results of the method, the context includes:
		// 1. Type parameters from the receiver's base type (struct).
		// 2. Type parameters from the method itself.
		methodScopeTypeParams := append([]*TypeParamInfo{}, receiverBaseTypeParams...)
		methodScopeTypeParams = append(methodScopeTypeParams, funcOwnTypeParams...) // funcOwnTypeParams are from the method itself.

		// Re-parse params and results with the correct combined scope
		// The original f.Type (which is *ast.FuncType) is used.
		reparsedFuncSignature := s.parseFuncType(ctx, f.Type, methodScopeTypeParams)
		funcInfo.Parameters = reparsedFuncSignature.Parameters
		funcInfo.Results = reparsedFuncSignature.Results
	}
	return funcInfo
}

// parseFuncType parses a function type (signature).
// It now accepts ctx and currentTypeParams for resolving type parameter references in params/results.
func (s *Scanner) parseFuncType(ctx context.Context, ft *ast.FuncType, currentTypeParams []*TypeParamInfo) *FunctionInfo {
	funcInfo := &FunctionInfo{}
	// Type parameters for a bare func type (e.g., in `type F = func[T any](p T) T`)
	// are part of ft.TypeParams. These are distinct from type parameters of a wrapping TypeSpec or FuncDecl.
	// If `currentTypeParams` is passed from a FuncDecl, those are the ones in scope for resolving
	// types within this signature. If `ft.TypeParams` also exists (e.g. generic func type alias),
	// then `ft.TypeParams` would define new params for *this specific func type's scope*.
	// The plan is to modify `parseFuncType` to take `currentTypeParams` from the *outer* scope (FuncDecl/TypeSpec).
	// If `ft` itself has `TypeParams` (e.g. `type F = func[X any](p X) X`), these should be parsed and
	// *added* to `currentTypeParams` for the scope of `ft.Params` and `ft.Results`.
	// For now, the provided `currentTypeParams` are from the FuncDecl or TypeSpec.

	// If ft.TypeParams is not nil, it means this is a generic func type directly.
	// Example: var myFunc func[T any](t T)
	// These would be parsed here and become the `funcInfo.TypeParams`.
	// However, the current plan implies `funcInfo.TypeParams` are set by `parseFuncDecl` from `f.Type.TypeParams`.
	// Let's stick to `currentTypeParams` being those from the *enclosing* declaration for now.
	// If `ft.TypeParams` are present, they define parameters for *this* literal func type.
	// This detail needs careful handling based on how Go AST represents this vs. FuncDecl.
	// For `func MyFunc[T any](p T){}`, `f.Type.TypeParams` is `[T any]`. `f.Type` is the `*ast.FuncType`.
	// The `*ast.FuncType` node itself also has a `TypeParams` field.
	// `f.Type.TypeParams` refers to the type parameters of the function declaration.
	// `ft.TypeParams` (where ft is *ast.FuncType) are the type parameters specific to that func type node.
	// Typically, for a FuncDecl, `f.Type.TypeParams` is the source of truth.

	if ft.Params != nil {
		funcInfo.Parameters = s.parseFieldList(ctx, ft.Params.List, currentTypeParams)
		// Check for variadic parameter
		if len(ft.Params.List) > 0 {
			lastParam := ft.Params.List[len(ft.Params.List)-1]
			if _, ok := lastParam.Type.(*ast.Ellipsis); ok {
				funcInfo.IsVariadic = true
			}
		}
	}
	if ft.Results != nil {
		funcInfo.Results = s.parseFieldList(ctx, ft.Results.List, currentTypeParams)
	}
	return funcInfo
}

// parseFieldList parses a list of fields (parameters or results).
// It now accepts ctx and currentTypeParams for resolving type parameter references.
func (s *Scanner) parseFieldList(ctx context.Context, fields []*ast.Field, currentTypeParams []*TypeParamInfo) []*FieldInfo {
	var result []*FieldInfo
	for _, field := range fields {
		fieldType := s.parseTypeExpr(ctx, field.Type, currentTypeParams) // Pass ctx and currentTypeParams
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				result = append(result, &FieldInfo{Name: name.Name, Type: fieldType, Doc: commentText(field.Doc)})
			}
		} else {
			// Unnamed parameter/result, use type name if possible or leave empty
			result = append(result, &FieldInfo{Type: fieldType, Doc: commentText(field.Doc)})
		}
	}
	return result
}

// parseTypeExpr parses an expression representing a type.
// It now accepts ctx for logging.
func (s *Scanner) parseTypeExpr(ctx context.Context, expr ast.Expr, currentTypeParams []*TypeParamInfo) *FieldType {
	ft := &FieldType{resolver: s.resolver}
	switch t := expr.(type) {
	case *ast.Ident:
		ft.Name = t.Name
		// Check if it's a built-in type or a type parameter
		isTypeParam := false
		if currentTypeParams != nil {
			for _, tp := range currentTypeParams {
				if tp.Name == t.Name {
					isTypeParam = true
					break
				}
			}
		}
		if isTypeParam {
			ft.IsTypeParam = true
		} else {
			switch t.Name {
			case "bool", "byte", "complex64", "complex128", "error", "float32", "float64",
				"int", "int8", "int16", "int32", "int64", "rune", "string",
				"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
				"any", "comparable": // Add Go 1.18 predeclared identifiers for constraints
				ft.IsBuiltin = true
				if t.Name == "any" || t.Name == "comparable" {
					ft.IsConstraint = true // Mark them as constraints
				}
			}
		}
	case *ast.StarExpr:
		underlyingType := s.parseTypeExpr(ctx, t.X, currentTypeParams) // Pass ctx and currentTypeParams
		underlyingType.IsPointer = true
		return underlyingType
	case *ast.SelectorExpr:
		pkgIdent, ok := t.X.(*ast.Ident)
		if !ok {
			// Could be a more complex expression on the left of selector, e.g. another SelectorExpr
			// For now, assume simple Pkg.Type form.
			// If t.X is another SelectorExpr, like "a.b.C", then parseTypeExpr(t.X) should handle it.
			// This part might need refinement for complex selectors.
			// Let's represent the full selector as Name for now if not a simple Ident.
			// This case needs to be robust.
			// A common pattern is `pkg.Type` where `pkg` is `ast.Ident`.
			// If `t.X` is not `ast.Ident`, it could be `anotherPkg.subPkg` which itself is a `SelectorExpr`.
			// This recursive nature should be handled by `parseTypeExpr` returning a `FieldType`
			// that represents `anotherPkg.subPkg` which we then use as `PkgName`.
			// For now, let's make a simple assumption.
			slog.Warn("Unhandled SelectorExpr with non-Ident X part", slog.Any("selector_x_type", fmt.Sprintf("%T", t.X)))
			ft.Name = fmt.Sprintf("unsupported_selector_expr.%s", t.Sel.Name) // Fallback name
			return ft
		}
		pkgImportPath, _ := s.importLookup[pkgIdent.Name]
		qualifiedName := fmt.Sprintf("%s.%s", pkgImportPath, t.Sel.Name)

		if overrideType, ok := s.ExternalTypeOverrides[qualifiedName]; ok {
			ft.Name = overrideType
			ft.IsResolvedByConfig = true
			// If the override itself is a pointer, etc., this simple assignment isn't enough.
			// Assume overrides are simple type names for now.
			return ft
		}
		ft.Name = fmt.Sprintf("%s.%s", pkgIdent.Name, t.Sel.Name) // This might be too simple; Name should be t.Sel.Name
		ft.PkgName = pkgIdent.Name
		ft.typeName = t.Sel.Name
		ft.fullImportPath = pkgImportPath
	case *ast.IndexExpr: // For single type argument, e.g., MyType[string]
		// t.X is the generic type (e.g., MyType)
		// t.Index is the type argument (e.g., string)
		genericType := s.parseTypeExpr(ctx, t.X, currentTypeParams)
		if genericType.IsTypeParam {
			// This case is likely an error in user code, e.g. T[int] where T is a type parameter.
			// Or it could be a more complex scenario not yet fully handled for type parameters used as generic types.
			slog.WarnContext(ctx, "IndexExpr on a type parameter, this might not be fully supported or implies an error", slog.String("type_param_name", genericType.Name))
			// For now, return the type parameter itself, and attach the index as a "type arg" - this might need refinement.
		}
		typeArg := s.parseTypeExpr(ctx, t.Index, currentTypeParams)
		genericType.TypeArgs = append(genericType.TypeArgs, typeArg)
		// The Name of the FieldType should ideally represent the instantiated type,
		// e.g., "MyType[string]". For now, genericType.Name still holds "MyType".
		// The String() method for FieldType will need to account for TypeArgs.
		return genericType // Return the base type with TypeArgs populated
	case *ast.IndexListExpr: // For multiple type arguments (Go 1.18+), e.g., MyMap[string, int]
		// t.X is the generic type (e.g., MyMap)
		// t.Indices contains the type arguments (e.g., string, int)
		genericType := s.parseTypeExpr(ctx, t.X, currentTypeParams)
		if genericType.IsTypeParam {
			slog.WarnContext(ctx, "IndexListExpr on a type parameter, this might not be fully supported or implies an error", slog.String("type_param_name", genericType.Name))
		}
		for _, indexExpr := range t.Indices {
			typeArg := s.parseTypeExpr(ctx, indexExpr, currentTypeParams)
			genericType.TypeArgs = append(genericType.TypeArgs, typeArg)
		}
		// Similar to IndexExpr, Name holds the base generic type name.
		return genericType // Return the base type with TypeArgs populated
	case *ast.ArrayType:
		ft.IsSlice = true
		// For unnamed slices, Name could be "slice" or derived.
		// Let's keep Name for the element type if possible, or make it specific like "[]<ElemType>".
		// The current FieldType.String() reconstructs this.
		// So ft.Name can be just "slice" or empty if Elem is filled.
		ft.Name = "slice"                                        // Placeholder name
		ft.Elem = s.parseTypeExpr(ctx, t.Elt, currentTypeParams) // Pass ctx and currentTypeParams
	case *ast.MapType:
		ft.IsMap = true
		ft.Name = "map"                                            // Placeholder name
		ft.MapKey = s.parseTypeExpr(ctx, t.Key, currentTypeParams) // Pass ctx and currentTypeParams
		ft.Elem = s.parseTypeExpr(ctx, t.Value, currentTypeParams) // Pass ctx and currentTypeParams
	case *ast.InterfaceType: // Handle interface types used as constraints or inlined
		// This represents an anonymous interface type, e.g. `interface{String() string}`
		// It could be a constraint for a type parameter.
		// For now, we'll give it a generic name. A fuller implementation might
		// parse its methods if needed, but that's part of parseInterfaceType.
		// Here, it's being used as a type expression.
		ft.Name = "interface{}" // Simplified representation
		// A more detailed parsing could involve creating a temporary TypeInfo for this interface.
		// If this interface is a constraint, mark it.
		// ft.IsConstraint = true; // This should be set if the context implies it's a constraint.
		// The caller (parseTypeParamList) will set IsConstraint on the resulting FieldType.
	default:
		// Ensure logging uses context if available, or fallback for general utility functions.
		slog.Warn("Unhandled type expression", slog.String("type", fmt.Sprintf("%T", t)))
		ft.Name = fmt.Sprintf("unhandled_type_%T", t)
	}
	return ft
}

// commentText extracts the text from a comment group.
func commentText(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	return strings.TrimSpace(cg.Text())
}

// (No trailing comments or code after the last function - ensure this is the true end of the file)
