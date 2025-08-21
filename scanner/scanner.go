package scanner

import (
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// fileParseResult holds the result of parsing a single Go source file.
type fileParseResult struct {
	filePath string
	fileAst  *ast.File
	err      error
}

// Scanner parses Go source files within a package.
type Scanner struct {
	fset                  *token.FileSet
	resolver              PackageResolver
	ExternalTypeOverrides ExternalTypeOverride
	Overlay               Overlay
	modulePath            string
	moduleRootDir         string
	inspect               bool
	logger                *slog.Logger
	mu                    sync.Mutex
}

// FileSet returns the underlying token.FileSet used by the scanner.
func (s *Scanner) FileSet() *token.FileSet {
	return s.fset
}

// New creates a new Scanner.
func New(fset *token.FileSet, overrides ExternalTypeOverride, overlay Overlay, modulePath string, moduleRootDir string, resolver PackageResolver, inspect bool, logger *slog.Logger) (*Scanner, error) {
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
	if resolver == nil {
		return nil, fmt.Errorf("resolver cannot be nil")
	}

	return &Scanner{
		fset:                  fset,
		ExternalTypeOverrides: overrides,
		Overlay:               overlay,
		modulePath:            modulePath,
		moduleRootDir:         moduleRootDir,
		resolver:              resolver,
		inspect:               inspect,
		logger:                logger,
	}, nil
}

// ResolveType starts the type resolution process for a given field type.
func (s *Scanner) ResolveType(ctx context.Context, fieldType *FieldType) (*TypeInfo, error) {
	// This is the start of a resolution chain. Initialize the path in the context.
	ctxWithPath := context.WithValue(ctx, ResolutionPathKey, []string{})
	return fieldType.Resolve(ctxWithPath)
}

// ScanPackageByImport makes scanner.Scanner implement the PackageResolver interface.
func (s *Scanner) ScanPackageByImport(ctx context.Context, importPath string) (*PackageInfo, error) {
	if s.resolver == nil {
		return nil, fmt.Errorf("scanner's internal resolver is not set, cannot scan by import path %q", importPath)
	}
	return s.resolver.ScanPackageByImport(ctx, importPath)
}

// ScanFiles parses a specific list of .go files and returns PackageInfo.
func (s *Scanner) ScanFiles(ctx context.Context, filePaths []string, pkgDirPath string) (*PackageInfo, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no files provided to scan for package at %s", pkgDirPath)
	}

	relPath, err := filepath.Rel(s.moduleRootDir, pkgDirPath)
	if err != nil {
		slog.WarnContext(ctx, "Could not determine relative path for import path derivation", "dirPath", pkgDirPath, "moduleRootDir", s.moduleRootDir)
		relPath = "."
	}
	importPath := filepath.ToSlash(filepath.Join(s.modulePath, relPath))
	if strings.HasSuffix(importPath, "/.") {
		importPath = importPath[:len(importPath)-2]
	}

	return s.scanGoFiles(ctx, filePaths, pkgDirPath, importPath)
}

// ScanFilesWithKnownImportPath parses files with a predefined import path.
func (s *Scanner) ScanFilesWithKnownImportPath(ctx context.Context, filePaths []string, pkgDirPath string, canonicalImportPath string) (*PackageInfo, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no files provided to scan for package at %s", pkgDirPath)
	}
	return s.scanGoFiles(ctx, filePaths, pkgDirPath, canonicalImportPath)
}

// ScanPackageImports parses only the import declarations from a set of Go files.
func (s *Scanner) ScanPackageImports(ctx context.Context, filePaths []string, pkgDirPath string, canonicalImportPath string) (*PackageImports, error) {
	info := &PackageImports{
		ImportPath:  canonicalImportPath,
		FileImports: make(map[string][]string),
	}
	imports := make(map[string]struct{})

	packageNames := make(map[string]int)
	fileAsts := make(map[string]*ast.File)

	// First pass: parse all files and collect package names
	for _, filePath := range filePaths {
		var content any
		if s.Overlay != nil {
			relPath, err := filepath.Rel(s.moduleRootDir, filePath)
			if err == nil {
				if overlayContent, ok := s.Overlay[relPath]; ok {
					content = overlayContent
				}
			}
		}

		fileAst, err := parser.ParseFile(s.fset, filePath, content, parser.ImportsOnly)
		if err != nil {
			return nil, fmt.Errorf("failed to parse imports for file %s: %w", filePath, err)
		}
		fileAsts[filePath] = fileAst
		if fileAst.Name != nil {
			packageNames[fileAst.Name.Name]++
		}
	}

	// Determine the dominant package name, ignoring 'main' if another name exists
	var dominantPackageName string
	if len(packageNames) > 1 {
		if _, hasMain := packageNames["main"]; hasMain {
			for name := range packageNames {
				if name != "main" {
					dominantPackageName = name
					break // Pick the first non-main package
				}
			}
		}
		// If there are multiple non-main packages, or only main and no other, it's an error
		if dominantPackageName == "" {
			var names []string
			for name := range packageNames {
				names = append(names, name)
			}
			return nil, fmt.Errorf("mismatched package names: %v in directory %s", names, pkgDirPath)
		}
	} else if len(packageNames) == 1 {
		for name := range packageNames {
			dominantPackageName = name
		}
	}

	if dominantPackageName == "" && len(filePaths) > 0 {
		return nil, fmt.Errorf("could not determine package name from files in %s", pkgDirPath)
	}
	info.Name = dominantPackageName

	// Second pass: process imports only for files matching the dominant package name
	for filePath, fileAst := range fileAsts {
		if fileAst.Name == nil || fileAst.Name.Name != dominantPackageName {
			continue // Skip files not belonging to the dominant package
		}

		var fileImports []string
		for _, imp := range fileAst.Imports {
			if imp.Path != nil {
				importPath := strings.Trim(imp.Path.Value, `"`)
				imports[importPath] = struct{}{}
				fileImports = append(fileImports, importPath)
			}
		}
		if len(fileImports) > 0 {
			info.FileImports[filePath] = fileImports
		}
	}

	if info.Name == "" && len(filePaths) > 0 {
		return nil, fmt.Errorf("could not determine package name from files in %s", pkgDirPath)
	}

	info.Imports = make([]string, 0, len(imports))
	for imp := range imports {
		info.Imports = append(info.Imports, imp)
	}

	return info, nil
}

func (s *Scanner) scanGoFiles(ctx context.Context, filePaths []string, pkgDirPath string, canonicalImportPath string) (*PackageInfo, error) {
	info := &PackageInfo{
		Path:       pkgDirPath,
		ImportPath: canonicalImportPath,
		Fset:       s.fset,
		AstFiles:   make(map[string]*ast.File),
	}

	// Stage 1: Parallel Parsing
	results := make(chan fileParseResult, len(filePaths))
	g, ctx := errgroup.WithContext(ctx)

	for _, filePath := range filePaths {
		fp := filePath // create a new variable for the closure
		g.Go(func() error {
			var content []byte
			var err error
			if s.Overlay != nil {
				relPath, _ := filepath.Rel(s.moduleRootDir, fp)
				if overlayContent, ok := s.Overlay[relPath]; ok {
					content = overlayContent
				}
			}

			if content == nil {
				content, err = os.ReadFile(fp)
				if err != nil {
					// Send error to the channel and exit goroutine
					results <- fileParseResult{filePath: fp, err: fmt.Errorf("reading file: %w", err)}
					return nil
				}
			}

			// Lock the mutex to ensure that parser.ParseFile, which modifies the shared
			// token.FileSet, is not called concurrently.
			s.mu.Lock()
			fileAst, err := parser.ParseFile(s.fset, fp, content, parser.ParseComments)
			s.mu.Unlock()

			select {
			case results <- fileParseResult{filePath: fp, fileAst: fileAst, err: err}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}

	if err := g.Wait(); err != nil {
		close(results)
		return nil, err
	}
	close(results)

	// Stage 2: Collect Results
	parsedFileResults := make([]fileParseResult, 0, len(filePaths))
	for result := range results {
		if result.err != nil {
			return nil, fmt.Errorf("failed to parse file %s: %w", result.filePath, result.err)
		}
		if result.fileAst.Name == nil {
			continue // Skip files with no package name
		}
		parsedFileResults = append(parsedFileResults, result)
	}

	// Stage 3: Sequential Processing
	var dominantPackageName string
	var parsedFiles []*ast.File
	var filePathsForDominantPkg []string

	// First pass over results to determine dominant package name
	for _, result := range parsedFileResults {
		currentPackageName := result.fileAst.Name.Name

		if dominantPackageName == "" {
			dominantPackageName = currentPackageName
			parsedFiles = append(parsedFiles, result.fileAst)
			filePathsForDominantPkg = append(filePathsForDominantPkg, result.filePath)
		} else if currentPackageName != dominantPackageName {
			// Mismatch detected. Apply heuristic for `go test` main package issue.
			if dominantPackageName == "main" && currentPackageName != "main" {
				// The real package is `currentPackageName`. Discard previous `main` files.
				dominantPackageName = currentPackageName

				newParsedFiles := []*ast.File{}
				newFilePaths := []string{}
				for i, astFile := range parsedFiles {
					if astFile.Name.Name == dominantPackageName {
						newParsedFiles = append(newParsedFiles, astFile)
						newFilePaths = append(newFilePaths, filePathsForDominantPkg[i])
					}
				}
				parsedFiles = newParsedFiles
				filePathsForDominantPkg = newFilePaths

				// Add the current file
				parsedFiles = append(parsedFiles, result.fileAst)
				filePathsForDominantPkg = append(filePathsForDominantPkg, result.filePath)

			} else if dominantPackageName != "main" && currentPackageName == "main" {
				// The real package is `dominantPackageName`. Ignore the `main` file.
				continue
			} else {
				// Two different non-main packages. This is a real error.
				return nil, fmt.Errorf("mismatched package names: %s and %s in directory %s", dominantPackageName, currentPackageName, pkgDirPath)
			}
		} else {
			// Package names match, just add the file.
			parsedFiles = append(parsedFiles, result.fileAst)
			filePathsForDominantPkg = append(filePathsForDominantPkg, result.filePath)
		}
	}

	info.Name = dominantPackageName
	info.Files = filePathsForDominantPkg

	// Second pass: Process declarations from the final list of valid ASTs
	for i, fileAst := range parsedFiles {
		filePath := info.Files[i]
		info.AstFiles[filePath] = fileAst
		importLookup := s.BuildImportLookup(fileAst)

		for _, decl := range fileAst.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				s.parseGenDecl(ctx, d, info, filePath, importLookup)
			case *ast.FuncDecl:
				info.Functions = append(info.Functions, s.parseFuncDecl(ctx, d, filePath, info, importLookup))
			}
		}
	}

	if info.Name == "" && len(filePaths) > 0 {
		return nil, fmt.Errorf("could not determine package name from scanned files in %s", pkgDirPath)
	}

	// Third pass: Evaluate all collected constants
	s.evaluateAllConstants(ctx, info)

	s.resolveEnums(info)
	return info, nil
}

// resolveEnums performs a linking pass to connect constants with their enum types.
func (s *Scanner) resolveEnums(pkgInfo *PackageInfo) {
	for _, c := range pkgInfo.Constants {
		// A constant must have an explicit type to be considered an enum member.
		if c.Type == nil || c.Type.TypeName == "" {
			continue
		}

		// The constant's type must belong to the package being scanned.
		// The parser sets FullImportPath for local types, so this check is reliable.
		if c.Type.FullImportPath != pkgInfo.ImportPath {
			continue
		}

		// Find the TypeInfo corresponding to the constant's type name.
		typeInfo := pkgInfo.Lookup(c.Type.TypeName)
		if typeInfo == nil {
			continue
		}

		// Link the constant to the type.
		typeInfo.EnumMembers = append(typeInfo.EnumMembers, c)
		typeInfo.IsEnum = true
	}
}

// BuildImportLookup creates a map of local import names to their full package paths.
func (s *Scanner) BuildImportLookup(file *ast.File) map[string]string {
	importLookup := make(map[string]string)
	for _, i := range file.Imports {
		path := strings.Trim(i.Path.Value, `"`)
		if i.Name != nil {
			importLookup[i.Name.Name] = path
		} else {
			parts := strings.Split(path, "/")
			importLookup[parts[len(parts)-1]] = path
		}
	}
	return importLookup
}

// constContext holds the state needed for evaluating constants across a package.
type constContext struct {
	pkg        *PackageInfo
	env        map[string]*ConstantInfo // Maps constant name to its info
	evaluating map[string]bool          // For cycle detection
}

// Pass 1: Just collect constant declarations without evaluating them.
func (s *Scanner) parseGenDecl(ctx context.Context, decl *ast.GenDecl, info *PackageInfo, absFilePath string, importLookup map[string]string) {
	if decl.Tok == token.CONST {
		var lastConstType *FieldType
		var lastConstValues []ast.Expr
		for iota, spec := range decl.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok {
				var currentSpecType *FieldType
				if vs.Type != nil {
					currentSpecType = s.TypeInfoFromExpr(ctx, vs.Type, nil, info, importLookup)
					lastConstType = currentSpecType
				} else {
					currentSpecType = lastConstType
				}

				if len(vs.Values) > 0 {
					lastConstValues = vs.Values
				}

				for i, name := range vs.Names {
					var valExpr ast.Expr
					if i < len(lastConstValues) {
						valExpr = lastConstValues[i]
					}

					constInfo := &ConstantInfo{
						Name:       name.Name,
						FilePath:   absFilePath,
						Doc:        commentText(vs.Doc),
						Type:       currentSpecType,
						IsExported: name.IsExported(),
						Node:       name,
						IotaValue:  iota,
						ValExpr:    valExpr,
					}
					info.Constants = append(info.Constants, constInfo)
				}
			}
		}
	} else if decl.Tok == token.TYPE {
		for _, spec := range decl.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok {
				typeInfo := s.parseTypeSpec(ctx, ts, info, absFilePath, importLookup)
				if typeInfo.Doc == "" && decl.Doc != nil {
					typeInfo.Doc = commentText(decl.Doc)
				}
				info.Types = append(info.Types, typeInfo)
			}
		}
	} else if decl.Tok == token.VAR {
		for _, spec := range decl.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok {
				var varType *FieldType
				if vs.Type != nil {
					varType = s.TypeInfoFromExpr(ctx, vs.Type, nil, info, importLookup)
				}

				for _, name := range vs.Names {
					varInfo := &VariableInfo{
						Name:       name.Name,
						FilePath:   absFilePath,
						Doc:        commentText(vs.Doc),
						Type:       varType,
						IsExported: name.IsExported(),
						Node:       name,
					}
					info.Variables = append(info.Variables, varInfo)
				}
			}
		}
	}
}

// Pass 2: Evaluate all collected constants.
func (s *Scanner) evaluateAllConstants(ctx context.Context, info *PackageInfo) {
	cctx := &constContext{
		pkg:        info,
		env:        make(map[string]*ConstantInfo),
		evaluating: make(map[string]bool),
	}
	for _, c := range info.Constants {
		cctx.env[c.Name] = c
	}

	for _, c := range info.Constants {
		s.evaluateConstant(cctx, c)
	}
}

// safeEvalConstExpr wraps evalConstExpr with a recover block to prevent panics
// from the go/constant package from crashing the scanner.
func (s *Scanner) safeEvalConstExpr(cctx *constContext, currentConst *ConstantInfo, expr ast.Expr) (val constant.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during constant evaluation: %v", r)
			val = constant.MakeUnknown()
		}
	}()
	return s.evalConstExpr(cctx, currentConst, expr)
}

// evaluateConstant is the entry point for evaluating a single constant. It handles caching and cycle detection.
func (s *Scanner) evaluateConstant(cctx *constContext, c *ConstantInfo) {
	// Pragmatic workaround for architecture-dependent constants in the stdlib.
	// Based on the user's hint that the target context (minigo) is always 64-bit.
	if cctx.pkg.ImportPath == "math/bits" && (c.Name == "UintSize" || c.Name == "uintSize") {
		c.ConstVal = constant.MakeInt64(64)
		c.Value = "64"
		return
	}
	if cctx.pkg.ImportPath == "strconv" && c.Name == "intSize" {
		c.ConstVal = constant.MakeInt64(64) // Assume 64-bit for consistency
		c.Value = "64"
		return
	}

	if c.ConstVal != nil {
		return // Already evaluated
	}
	if cctx.evaluating[c.Name] {
		c.Value = "evaluation_error_cycle"
		c.ConstVal = constant.MakeUnknown()
		return
	}
	cctx.evaluating[c.Name] = true
	defer func() { cctx.evaluating[c.Name] = false }()

	if c.ValExpr == nil {
		c.Value = "evaluation_error_implicit"
		c.ConstVal = constant.MakeUnknown()
		return
	}

	val, err := s.safeEvalConstExpr(cctx, c, c.ValExpr)
	if err != nil {
		// If evaluation fails, just leave the value as unknown.
		// The binding generator can still bind the symbol by name,
		// and the Go compiler will handle the actual value.
		c.ConstVal = constant.MakeUnknown()
		c.Value = "" // Leave it empty to signify it's not resolved.
		return
	}
	c.ConstVal = val
	c.Value = val.String()
	if val.Kind() == constant.String {
		c.RawValue = constant.StringVal(val)
	}
}

// evalConstExpr recursively evaluates an AST expression to a constant.Value.
func (s *Scanner) evalConstExpr(cctx *constContext, currentConst *ConstantInfo, expr ast.Expr) (constant.Value, error) {
	switch n := expr.(type) {
	case *ast.Ident:
		if n.Name == "iota" {
			return constant.MakeFromLiteral(fmt.Sprintf("%d", currentConst.IotaValue), token.INT, 0), nil
		}
		if c, ok := cctx.env[n.Name]; ok {
			s.evaluateConstant(cctx, c) // Ensure dependency is evaluated
			if c.ConstVal != nil && c.ConstVal.Kind() != constant.Unknown {
				return c.ConstVal, nil
			}
			return nil, fmt.Errorf("dependency %s could not be evaluated", n.Name)
		}
		// Handle built-in `true` and `false`
		if n.Obj != nil && n.Obj.Kind == ast.Con && n.Obj.Data == nil {
			switch n.Name {
			case "true":
				return constant.MakeBool(true), nil
			case "false":
				return constant.MakeBool(false), nil
			}
		}
		return nil, fmt.Errorf("unresolved identifier: %s", n.Name)
	case *ast.BasicLit:
		return constant.MakeFromLiteral(n.Value, n.Kind, 0), nil
	case *ast.ParenExpr:
		return s.evalConstExpr(cctx, currentConst, n.X)
	case *ast.UnaryExpr:
		x, err := s.evalConstExpr(cctx, currentConst, n.X)
		if err != nil {
			return nil, err
		}
		return constant.UnaryOp(n.Op, x, 0), nil
	case *ast.BinaryExpr:
		x, err := s.evalConstExpr(cctx, currentConst, n.X)
		if err != nil {
			return nil, err
		}
		y, err := s.evalConstExpr(cctx, currentConst, n.Y)
		if err != nil {
			return nil, err
		}

		if n.Op == token.SHL || n.Op == token.SHR {
			if y_uint64, exact := constant.Uint64Val(y); exact {
				return constant.Shift(x, n.Op, uint(y_uint64)), nil
			}
			return nil, fmt.Errorf("shift amount must be an unsigned integer, got %s", y.String())
		}
		return constant.BinaryOp(x, n.Op, y), nil
	case *ast.SelectorExpr:
		// TODO: Handle cross-package constant references.
		return nil, fmt.Errorf("cross-package constant references not supported")
	case *ast.CallExpr:
		// Handle simple type conversions like `uint(0)`.
		if typeIdent, ok := n.Fun.(*ast.Ident); ok {
			if len(n.Args) == 1 {
				if lit, ok := n.Args[0].(*ast.BasicLit); ok && lit.Kind == token.INT {
					// This is a basic form of type conversion, e.g., uint(0).
					// We can treat the literal as the value.
					// This is a simplification and doesn't handle all conversions.
					switch typeIdent.Name {
					case "uint", "int", "uint64", "int64", "float64", "float32", "string":
						return constant.MakeFromLiteral(lit.Value, lit.Kind, 0), nil
					}
				}
			}
		}
		// TODO: Handle built-in functions like unsafe.Sizeof.
		return nil, fmt.Errorf("built-in functions and complex type conversions in const expressions not supported")
	default:
		return nil, fmt.Errorf("unsupported const expression type: %T", expr)
	}
}

func (s *Scanner) parseTypeSpec(ctx context.Context, sp *ast.TypeSpec, info *PackageInfo, absFilePath string, importLookup map[string]string) *TypeInfo {
	typeInfo := &TypeInfo{
		Name:     sp.Name.Name,
		PkgPath:  info.ImportPath,
		FilePath: absFilePath,
		Doc:      commentText(sp.Doc),
		Node:     sp,
		Inspect:  s.inspect,
		Logger:   s.logger,
		Fset:     info.Fset,
	}

	// Set up the initial resolution context for this type.
	// Any types resolved from this type's fields will have this type's identifier in their path.
	typeIdentifier := info.ImportPath + "." + sp.Name.Name
	initialPath := []string{typeIdentifier}
	childCtx := context.WithValue(context.Background(), ResolutionPathKey, initialPath)
	if s.logger != nil {
		childCtx = context.WithValue(childCtx, LoggerKey, s.logger)
	}
	childCtx = context.WithValue(childCtx, InspectKey, s.inspect)
	typeInfo.ResolutionContext = childCtx

	if sp.TypeParams != nil {
		typeInfo.TypeParams = s.parseTypeParamList(ctx, sp.TypeParams.List, info, importLookup)
	}

	switch t := sp.Type.(type) {
	case *ast.StructType:
		typeInfo.Kind = StructKind
		typeInfo.Struct = s.parseStructType(ctx, t, typeInfo.TypeParams, info, importLookup)
	case *ast.InterfaceType:
		typeInfo.Kind = InterfaceKind
		typeInfo.Interface = s.parseInterfaceType(ctx, t, typeInfo.TypeParams, info, importLookup)
	case *ast.FuncType:
		typeInfo.Kind = FuncKind
		typeInfo.Func = s.parseFuncType(ctx, t, typeInfo.TypeParams, info, importLookup)
	default:
		typeInfo.Kind = AliasKind
		typeInfo.Underlying = s.TypeInfoFromExpr(ctx, sp.Type, typeInfo.TypeParams, info, importLookup)
	}
	return typeInfo
}

func (s *Scanner) parseTypeParamList(ctx context.Context, typeParamFields []*ast.Field, info *PackageInfo, importLookup map[string]string) []*TypeParamInfo {
	var params []*TypeParamInfo
	if typeParamFields == nil {
		return nil
	}
	for _, typeParamField := range typeParamFields {
		var constraintFieldType *FieldType
		if constraintExpr := typeParamField.Type; constraintExpr != nil {
			constraintFieldType = s.TypeInfoFromExpr(ctx, constraintExpr, nil, info, importLookup)
			if constraintFieldType != nil {
				constraintFieldType.IsConstraint = true
			}
		}
		for _, nameIdent := range typeParamField.Names {
			params = append(params, &TypeParamInfo{
				Name:       nameIdent.Name,
				Constraint: constraintFieldType,
			})
		}
	}
	return params
}

func (s *Scanner) parseInterfaceType(ctx context.Context, it *ast.InterfaceType, currentTypeParams []*TypeParamInfo, info *PackageInfo, importLookup map[string]string) *InterfaceInfo {
	if it.Methods == nil || len(it.Methods.List) == 0 {
		return &InterfaceInfo{Methods: []*MethodInfo{}}
	}
	interfaceInfo := &InterfaceInfo{
		Methods: make([]*MethodInfo, 0, len(it.Methods.List)),
	}
	for _, field := range it.Methods.List {
		if len(field.Names) > 0 {
			methodName := field.Names[0].Name
			funcType, ok := field.Type.(*ast.FuncType)
			if !ok {
				continue
			}
			methodInfo := &MethodInfo{Name: methodName}
			parsedFuncDetails := s.parseFuncType(ctx, funcType, currentTypeParams, info, importLookup)
			methodInfo.Parameters = parsedFuncDetails.Parameters
			methodInfo.Results = parsedFuncDetails.Results
			interfaceInfo.Methods = append(interfaceInfo.Methods, methodInfo)
		} else {
			embeddedType := s.TypeInfoFromExpr(ctx, field.Type, currentTypeParams, info, importLookup)
			interfaceInfo.Methods = append(interfaceInfo.Methods, &MethodInfo{
				Name:    fmt.Sprintf("embedded_%s", embeddedType.String()),
				Results: []*FieldInfo{{Type: embeddedType}},
			})
		}
	}
	return interfaceInfo
}

func (s *Scanner) parseStructType(ctx context.Context, st *ast.StructType, currentTypeParams []*TypeParamInfo, info *PackageInfo, importLookup map[string]string) *StructInfo {
	structInfo := &StructInfo{}
	for _, field := range st.Fields.List {
		fieldType := s.TypeInfoFromExpr(ctx, field.Type, currentTypeParams, info, importLookup)
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
					Name:       name.Name,
					Doc:        doc,
					Type:       fieldType,
					Tag:        tag,
					IsExported: name.IsExported(),
				})
			}
		} else {
			structInfo.Fields = append(structInfo.Fields, &FieldInfo{
				Name:     fieldType.Name,
				Doc:      doc,
				Type:     fieldType,
				Tag:      tag,
				Embedded: true,
			})
		}
	}
	return structInfo
}

func (s *Scanner) parseFuncDecl(ctx context.Context, f *ast.FuncDecl, absFilePath string, pkgInfo *PackageInfo, importLookup map[string]string) *FunctionInfo {
	var funcOwnTypeParams []*TypeParamInfo
	if f.Type.TypeParams != nil {
		funcOwnTypeParams = s.parseTypeParamList(ctx, f.Type.TypeParams.List, pkgInfo, importLookup)
	}

	funcInfo := s.parseFuncType(ctx, f.Type, funcOwnTypeParams, pkgInfo, importLookup)
	funcInfo.Name = f.Name.Name
	funcInfo.FilePath = absFilePath
	funcInfo.Doc = commentText(f.Doc)
	funcInfo.AstDecl = f
	funcInfo.TypeParams = funcOwnTypeParams

	if f.Recv != nil && len(f.Recv.List) > 0 {
		recvField := f.Recv.List[0]
		var recvName string
		if len(recvField.Names) > 0 {
			recvName = recvField.Names[0].Name
		}

		var receiverBaseTypeParams []*TypeParamInfo
		parsedRecvFieldType := s.TypeInfoFromExpr(ctx, recvField.Type, funcOwnTypeParams, pkgInfo, importLookup)

		if parsedRecvFieldType != nil {
			baseRecvTypeName := parsedRecvFieldType.Name
			if parsedRecvFieldType.IsPointer && parsedRecvFieldType.Elem != nil {
				baseRecvTypeName = parsedRecvFieldType.Elem.Name
			}
			if parts := strings.Split(baseRecvTypeName, "."); len(parts) > 1 {
				baseRecvTypeName = parts[len(parts)-1]
			}

			if pkgInfo != nil {
				for _, ti := range pkgInfo.Types {
					if ti.Name == baseRecvTypeName {
						receiverBaseTypeParams = ti.TypeParams
						parsedRecvFieldType = s.TypeInfoFromExpr(ctx, recvField.Type, receiverBaseTypeParams, pkgInfo, importLookup)
						break
					}
				}
			}
		}

		funcInfo.Receiver = &FieldInfo{
			Name: recvName,
			Type: parsedRecvFieldType,
		}

		methodScopeTypeParams := append([]*TypeParamInfo{}, receiverBaseTypeParams...)
		methodScopeTypeParams = append(methodScopeTypeParams, funcOwnTypeParams...)

		reparsedFuncSignature := s.parseFuncType(ctx, f.Type, methodScopeTypeParams, pkgInfo, importLookup)
		funcInfo.Parameters = reparsedFuncSignature.Parameters
		funcInfo.Results = reparsedFuncSignature.Results
	}
	return funcInfo
}

func (s *Scanner) parseFuncType(ctx context.Context, ft *ast.FuncType, currentTypeParams []*TypeParamInfo, info *PackageInfo, importLookup map[string]string) *FunctionInfo {
	funcInfo := &FunctionInfo{}
	if ft.Params != nil {
		funcInfo.Parameters = s.parseFieldList(ctx, ft.Params.List, currentTypeParams, info, importLookup)
		if len(ft.Params.List) > 0 {
			if _, ok := ft.Params.List[len(ft.Params.List)-1].Type.(*ast.Ellipsis); ok {
				funcInfo.IsVariadic = true
			}
		}
	}
	if ft.Results != nil {
		funcInfo.Results = s.parseFieldList(ctx, ft.Results.List, currentTypeParams, info, importLookup)
	}
	return funcInfo
}

func (s *Scanner) parseFieldList(ctx context.Context, fields []*ast.Field, currentTypeParams []*TypeParamInfo, info *PackageInfo, importLookup map[string]string) []*FieldInfo {
	var result []*FieldInfo
	for _, field := range fields {
		fieldType := s.TypeInfoFromExpr(ctx, field.Type, currentTypeParams, info, importLookup)
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				result = append(result, &FieldInfo{Name: name.Name, Type: fieldType, Doc: commentText(field.Doc)})
			}
		} else {
			result = append(result, &FieldInfo{Type: fieldType, Doc: commentText(field.Doc)})
		}
	}
	return result
}

// TypeInfoFromExpr resolves an AST expression that represents a type into a FieldType.
// This is the core type-parsing logic, exposed for tools that need to resolve
// type information dynamically.
func (s *Scanner) TypeInfoFromExpr(ctx context.Context, expr ast.Expr, currentTypeParams []*TypeParamInfo, info *PackageInfo, importLookup map[string]string) *FieldType {
	ft := &FieldType{Resolver: s.resolver, CurrentPkg: info}
	switch t := expr.(type) {
	case *ast.Ident:
		ft.Name = t.Name
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
				"any", "comparable":
				ft.IsBuiltin = true
				if t.Name == "any" || t.Name == "comparable" {
					ft.IsConstraint = true
				}
			default:
				if info != nil {
					ft.FullImportPath = info.ImportPath
					ft.TypeName = t.Name
				}
			}
		}
	case *ast.StarExpr:
		elemType := s.TypeInfoFromExpr(ctx, t.X, currentTypeParams, info, importLookup)
		return &FieldType{
			Resolver:           s.resolver,
			Name:               elemType.Name,
			IsPointer:          true,
			Elem:               elemType,
			FullImportPath:     elemType.FullImportPath,
			TypeName:           elemType.TypeName,
			PkgName:            elemType.PkgName,
			TypeArgs:           elemType.TypeArgs,
			IsResolvedByConfig: elemType.IsResolvedByConfig, // Propagate from element
			CurrentPkg:         info,                        // Ensure current package context is passed
		}
	case *ast.SelectorExpr:
		pkgIdent, ok := t.X.(*ast.Ident)
		if !ok {
			return &FieldType{Name: fmt.Sprintf("unsupported_selector_expr.%s", t.Sel.Name)}
		}
		pkgImportPath, _ := importLookup[pkgIdent.Name]
		qualifiedName := fmt.Sprintf("%s.%s", pkgImportPath, t.Sel.Name)

		// Check for external type overrides first.
		if overrideInfo, ok := s.ExternalTypeOverrides[qualifiedName]; ok {
			// If an override is found, create a FieldType from the synthetic TypeInfo.
			return &FieldType{
				Name:               overrideInfo.Name,
				PkgName:            overrideInfo.PkgPath,
				FullImportPath:     overrideInfo.PkgPath,
				TypeName:           overrideInfo.Name,
				IsResolvedByConfig: true,
				Definition:         overrideInfo, // Link to the synthetic definition
				Resolver:           s.resolver,
			}
		}

		// If no override, proceed with normal parsing.
		ft.PkgName = pkgIdent.Name
		ft.TypeName = t.Sel.Name
		ft.FullImportPath = pkgImportPath
		ft.Name = t.Sel.Name
	case *ast.IndexExpr:
		genericType := s.TypeInfoFromExpr(ctx, t.X, currentTypeParams, info, importLookup)
		typeArg := s.TypeInfoFromExpr(ctx, t.Index, currentTypeParams, info, importLookup)
		genericType.TypeArgs = append(genericType.TypeArgs, typeArg)
		return genericType
	case *ast.IndexListExpr:
		genericType := s.TypeInfoFromExpr(ctx, t.X, currentTypeParams, info, importLookup)
		for _, indexExpr := range t.Indices {
			typeArg := s.TypeInfoFromExpr(ctx, indexExpr, currentTypeParams, info, importLookup)
			genericType.TypeArgs = append(genericType.TypeArgs, typeArg)
		}
		return genericType
	case *ast.ArrayType:
		ft.IsSlice = true
		ft.Name = "slice"
		ft.Elem = s.TypeInfoFromExpr(ctx, t.Elt, currentTypeParams, info, importLookup)
	case *ast.MapType:
		ft.IsMap = true
		ft.Name = "map"
		ft.MapKey = s.TypeInfoFromExpr(ctx, t.Key, currentTypeParams, info, importLookup)
		ft.Elem = s.TypeInfoFromExpr(ctx, t.Value, currentTypeParams, info, importLookup)
	case *ast.InterfaceType:
		ft.Name = "interface{}"
	case *ast.Ellipsis:
		ft.IsSlice = true
		ft.Name = "slice"
		ft.Elem = s.TypeInfoFromExpr(ctx, t.Elt, currentTypeParams, info, importLookup)
	default:
		ft.Name = fmt.Sprintf("unhandled_type_%T", t)
	}
	return ft
}

func commentText(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	return strings.TrimSpace(cg.Text())
}
