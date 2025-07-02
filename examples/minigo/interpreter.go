package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"go/ast"

	// "go/parser" // Will be replaced by go-scan
	"go/token"
	"os"
	"path/filepath" // Added for go-scan
	"strconv"
	"strings"

	goscan "github.com/podhmo/go-scan"  // Using top-level go-scan
	"github.com/podhmo/go-scan/scanner" // No longer directly needed for minigo's use of ConstantInfo
)

// formatErrorWithContext creates a detailed error message including file, line, column, and source code.
func formatErrorWithContext(fset *token.FileSet, pos token.Pos, originalErr error, customMsg string) error {
	baseErrMsg := ""
	if originalErr != nil {
		baseErrMsg = originalErr.Error()
	}

	if pos == token.NoPos {
		if customMsg != "" {
			if originalErr != nil {
				return fmt.Errorf("%s: %w", customMsg, originalErr)
			}
			return errors.New(customMsg)
		}
		if originalErr != nil { // Return original error if no custom message and no pos
			return originalErr
		}
		return errors.New("unknown error") // Should not happen if originalErr is always provided
	}

	position := fset.Position(pos)
	filename := position.Filename
	line := position.Line
	column := position.Column

	var sourceLine string
	file, err := os.Open(filename)
	if err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for i := 1; scanner.Scan(); i++ {
			if i == line {
				sourceLine = strings.TrimSpace(scanner.Text())
				break
			}
		}
		if err := scanner.Err(); err != nil {
			sourceLine = fmt.Sprintf("[Error reading source line: %v]", err)
		}
	} else {
		sourceLine = fmt.Sprintf("[Error opening source file: %v]", err)
	}

	detailMsg := fmt.Sprintf("Error in %s at line %d, column %d", filename, line, column)
	if customMsg != "" {
		detailMsg = fmt.Sprintf("%s: %s", customMsg, detailMsg)
	}

	if sourceLine != "" {
		if baseErrMsg != "" {
			return fmt.Errorf("%s\n  Source: %s\n  Details: %s", detailMsg, sourceLine, baseErrMsg)
		}
		return fmt.Errorf("%s\n  Source: %s", detailMsg, sourceLine)
	}

	if baseErrMsg != "" {
		return fmt.Errorf("%s\n  Details: %s", detailMsg, baseErrMsg)
	}
	return fmt.Errorf("%s", detailMsg) // Use %s to treat detailMsg as a string literal
}

// parseInt64 is a helper function to parse a string to an int64.
// It's defined here to keep the main eval function cleaner.
func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 0, 64)
}

// astNodeToString converts an AST node to its string representation.
// This is a helper for error messages. It's not exhaustive.
func astNodeToString(node ast.Node, fset *token.FileSet) string {
	// This is a simplified version. For more complex nodes, you might need
	// to use format.Node from go/format, but that requires an io.Writer.
	// For simple identifiers or selectors, this should suffice.
	switch n := node.(type) {
	case *ast.Ident:
		return n.Name
	case *ast.SelectorExpr:
		return astNodeToString(n.X, fset) + "." + n.Sel.Name
	// Add other cases as needed
	default:
		if fset != nil && node != nil && node.Pos() != token.NoPos && node.End() != token.NoPos {
			// Fallback to raw text if possible, limited range
			// This is very basic and might not be ideal for all nodes.
			// Consider a more robust way if complex types are common in errors.
			// For now, often the type is %T.
			return fmt.Sprintf("%T at %s", node, fset.Position(node.Pos()).String())
		}
		return fmt.Sprintf("%T", node)
	}
}

// Interpreter holds the state of the interpreter
type Interpreter struct {
	globalEnv        *Environment
	FileSet          *token.FileSet
	scn              *goscan.Scanner     // Use the top-level go-scan Scanner
	importedPackages map[string]struct{} // Key: importPath, keeps track of resolved packages
	importAliasMap   map[string]string   // Key: localPkgName (alias or base), Value: importPath
	// currentFileDir is the directory of the main file being interpreted.
	// This helps in resolving relative imports if go.mod is not present or
	// for files not part of a clear module structure.
	currentFileDir string
	sharedScanner  *goscan.Scanner // Renamed from scn, used for resolving imports. Can be pre-configured for tests.
	ModuleRoot     string          // Optional: Explicitly set module root directory for scanner initialization.
}

func NewInterpreter() *Interpreter {
	env := NewEnvironment(nil)
	// FileSet will be initialized by the scanner used for the main script.
	// sharedScanner can be nil initially and created on-demand by LoadAndRun if not set by tests.
	i := &Interpreter{
		globalEnv:        env,
		FileSet:          nil, // To be set by the localScriptScanner in LoadAndRun
		sharedScanner:    nil, // Can be preset by tests, or created by LoadAndRun if needed for imports
		importedPackages: make(map[string]struct{}),
		importAliasMap:   make(map[string]string),
	}

	builtins := GetBuiltinFmtFunctions()
	for name, builtin := range builtins {
		env.Define(name, builtin)
	}
	builtinsStrings := GetBuiltinStringsFunctions()
	for name, builtin := range builtinsStrings {
		env.Define(name, builtin)
	}
	return i
}

// LoadAndRun loads a Go source file, parses it, and runs the specified entry point function.
func (i *Interpreter) LoadAndRun(ctx context.Context, filename string, entryPoint string) error {
	absFilePath, err := filepath.Abs(filename)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %s: %w", filename, err)
	}
	i.currentFileDir = filepath.Dir(absFilePath)

	// Create a local scanner specifically for parsing the main script file.
	// Use ModuleRoot if available, otherwise use the script's directory.
	scanPathForLocal := i.currentFileDir
	if i.ModuleRoot != "" {
		scanPathForLocal = i.ModuleRoot
	}
	localScriptScanner, errGs := goscan.New(scanPathForLocal)
	if errGs != nil {
		// If this fails, we can't get a FileSet for error reporting, so use token.NoPos
		return formatErrorWithContext(nil, token.NoPos, errGs, fmt.Sprintf("Failed to create go-scan scanner for local script (path: %s): %v", scanPathForLocal, errGs))
	}
	if localScriptScanner.Fset() == nil {
		return formatErrorWithContext(nil, token.NoPos, errors.New("internal error: localScriptScanner created by goscan.New has a nil FileSet"), "")
	}
	// The primary FileSet for the interpreter run is taken from this local scanner,
	// as it pertains to the main script being processed.
	i.FileSet = localScriptScanner.Fset()

	// Use the localScriptScanner to parse the main script file.
	pkgInfo, scanErr := localScriptScanner.ScanFiles(ctx, []string{absFilePath})
	if scanErr != nil {
		// Use i.FileSet which is now localScriptScanner.Fset()
		return formatErrorWithContext(i.FileSet, token.NoPos, scanErr, fmt.Sprintf("Error scanning main script file %s using go-scan: %v", filename, scanErr))
	}

	// Retrieve the AST for the main file from pkgInfo
	mainFileAst, ok := pkgInfo.AstFiles[absFilePath]
	if !ok || mainFileAst == nil {
		return formatErrorWithContext(i.FileSet, token.NoPos, errors.New("AST for main file not found in go-scan PackageInfo"), fmt.Sprintf("File: %s", absFilePath))
	}
	// pkgInfo.Fset should be the same as i.FileSet if localScriptScanner worked correctly.

	// Ensure the sharedScanner (for imports) is available if needed.
	// If not preset by tests, initialize it based on the current file's directory.
	// This might be overridden by tests for specific module contexts.
	if i.sharedScanner == nil {
		// Default sharedScanner if not set by tests.
		// Its module context will be based on the main script's directory.
		// This is suitable if imports are relative or within the same implicit module as the script.
		// Tests for specific module structures (like mytestmodule) will pre-set i.sharedScanner.
		scanPathForShared := i.currentFileDir
		if i.ModuleRoot != "" {
			scanPathForShared = i.ModuleRoot
		}
		defaultSharedScanner, errSharedGs := goscan.New(scanPathForShared)
		if errSharedGs != nil {
			return formatErrorWithContext(i.FileSet, token.NoPos, errSharedGs, fmt.Sprintf("Failed to create default shared go-scan scanner (path: %s): %v", scanPathForShared, errSharedGs))
		}
		i.sharedScanner = defaultSharedScanner
	}
	// If i.sharedScanner was preset by a test, that test is also responsible for ensuring
	// its FileSet is appropriate or that i.FileSet (from localScriptScanner) is used carefully.
	// For now, errors from imports via sharedScanner will use sharedScanner.Fset() internally if they format.

	// Process import declarations from the AST to populate importAliasMap
	// This part still uses mainFileAst directly, which is fine.
	for _, declNode := range mainFileAst.Decls {
		genDecl, ok := declNode.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.IMPORT {
			continue
		}
		for _, spec := range genDecl.Specs {
			impSpec, ok := spec.(*ast.ImportSpec)
			if !ok {
				continue
			}
			importPath, err := strconv.Unquote(impSpec.Path.Value)
			if err != nil {
				return formatErrorWithContext(i.FileSet, impSpec.Path.Pos(), err, fmt.Sprintf("Invalid import path: %s", impSpec.Path.Value))
			}

			localName := ""
			if impSpec.Name != nil {
				localName = impSpec.Name.Name
				if localName == "_" {
					// Blank imports are ignored, do not add to importAliasMap
					continue
				}
				if localName == "." {
					return formatErrorWithContext(i.FileSet, impSpec.Name.Pos(), errors.New("dot imports are not supported"), "")
				}
			} else {
				localName = filepath.Base(importPath)
			}

			if existingPath, ok := i.importAliasMap[localName]; ok && existingPath != importPath {
				return formatErrorWithContext(i.FileSet, impSpec.Pos(), fmt.Errorf("import alias/name %q already used for %q, cannot reuse for %q", localName, existingPath, importPath), "")
			}
			i.importAliasMap[localName] = importPath
		}
	}

	// Process function declarations using FunctionInfo from go-scan's PackageInfo
	for _, funcInfo := range pkgInfo.Functions {
		if funcInfo.AstDecl == nil {
			// This should not happen if go-scan is working correctly
			errMsg := fmt.Sprintf("FunctionInfo for '%s' from go-scan is missing AstDecl", funcInfo.Name)
			return formatErrorWithContext(i.FileSet, token.NoPos, errors.New(errMsg), "Internal error with go-scan data")
		}
		// funcInfo.FilePath should match absFilePath if it's from the main file.
		// We are interested in functions from the main file.
		if funcInfo.FilePath == absFilePath {
			_, evalErr := i.evalFuncDecl(ctx, funcInfo.AstDecl, i.globalEnv)
			if evalErr != nil {
				// evalFuncDecl itself should use i.FileSet for formatting,
				// but its error might not have original context if it's a general one.
				// The AstDecl.Pos() would be the best position.
				return formatErrorWithContext(i.FileSet, funcInfo.AstDecl.Pos(), evalErr, fmt.Sprintf("Error evaluating function declaration %s", funcInfo.Name))
			}
		}
	}

	// Process global variable declarations from the AST (mainFileAst)
	// This part remains similar, iterating mainFileAst.Decls
	for _, declNode := range mainFileAst.Decls {
		if genDecl, ok := declNode.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			tempDeclStmt := &ast.DeclStmt{Decl: genDecl}
			_, evalErr := i.eval(ctx, tempDeclStmt, i.globalEnv)
			if evalErr != nil {
				return formatErrorWithContext(i.FileSet, genDecl.Pos(), evalErr, "Error evaluating global variable declaration")
			}
		}
	}

	// Get the entry function *object* from the global environment
	entryFuncObj, ok := i.globalEnv.Get(entryPoint)
	if !ok {
		return formatErrorWithContext(i.FileSet, token.NoPos, fmt.Errorf("entry point function '%s' not found in global environment", entryPoint), "Setup error")
	}

	userEntryFunc, ok := entryFuncObj.(*UserDefinedFunction)
	if !ok {
		return formatErrorWithContext(i.FileSet, token.NoPos, fmt.Errorf("entry point '%s' is not a user-defined function (type: %s)", entryPoint, entryFuncObj.Type()), "Setup error")
	}

	fmt.Printf("Executing entry point function: %s\n", entryPoint)
	// For main/entry point, we assume no arguments are passed.
	result, errApply := i.applyUserDefinedFunction(ctx, userEntryFunc, []Object{}, token.NoPos)
	if errApply != nil {
		if errObj, isErrObj := errApply.(*Error); isErrObj {
			return fmt.Errorf("Runtime error in %s: %s", entryPoint, errObj.Message)
		}
		return errApply
	}

	if result != nil && result.Type() != NULL_OBJ {
		fmt.Printf("Entry point '%s' finished, result: %s\n", entryPoint, result.Inspect())
	} else {
		fmt.Printf("Entry point '%s' finished.\n", entryPoint)
	}
	return nil
}

func (i *Interpreter) applyUserDefinedFunction(ctx context.Context, fn *UserDefinedFunction, args []Object, callPos token.Pos) (Object, error) {
	// Use the FileSet associated with the function for error reporting within its context.
	// If the function's FileSet is nil (e.g., for older UserDefinedFunction objects not yet updated),
	// fall back to the interpreter's main FileSet.
	errorFileSet := fn.FileSet
	if errorFileSet == nil {
		errorFileSet = i.FileSet // Fallback
	}

	if len(args) != len(fn.Parameters) {
		errMsg := fmt.Sprintf("wrong number of arguments for function %s: expected %d, got %d", fn.Name, len(fn.Parameters), len(args))
		return nil, formatErrorWithContext(errorFileSet, callPos, errors.New(errMsg), "Function call error")
	}

	funcEnv := NewEnvironment(fn.Env) // Closure: fn.Env is the lexical scope

	for idx, paramIdent := range fn.Parameters {
		funcEnv.Define(paramIdent.Name, args[idx])
	}

	evaluated, errEval := i.evalBlockStatement(ctx, fn.Body, funcEnv)
	if errEval != nil {
		return nil, errEval
	}

	if retVal, ok := evaluated.(*ReturnValue); ok {
		if retVal.Value == nil {
			return NULL, nil
		}
		return retVal.Value, nil
	}
	return NULL, nil
}

// findFunction is likely unused but kept for now.
func findFunction(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if fn.Name.Name == name {
				return fn
			}
		}
	}
	return nil
}

func (i *Interpreter) eval(ctx context.Context, node ast.Node, env *Environment) (Object, error) {
	switch n := node.(type) {
	case *ast.File:
		var result Object
		var err error
		for _, decl := range n.Decls {
			if fnDecl, ok := decl.(*ast.FuncDecl); ok && fnDecl.Name.Name == "main" {
				result, err = i.evalBlockStatement(ctx, fnDecl.Body, env)
				if err != nil {
					return nil, err
				}
			}
		}
		return result, nil

	case *ast.BlockStmt:
		return i.evalBlockStatement(ctx, n, env)

	case *ast.ExprStmt:
		return i.eval(ctx, n.X, env)

	case *ast.Ident:
		return evalIdentifier(n, env, i.FileSet) // evalIdentifier does not need ctx

	case *ast.BasicLit:
		switch n.Kind {
		case token.STRING:
			return &String{Value: n.Value[1 : len(n.Value)-1]}, nil
		case token.INT:
			val, err := parseInt64(n.Value)
			if err != nil {
				return nil, formatErrorWithContext(i.FileSet, n.Pos(), err, fmt.Sprintf("Could not parse integer literal '%s'", n.Value))
			}
			return &Integer{Value: val}, nil
		default:
			return nil, formatErrorWithContext(i.FileSet, n.Pos(), fmt.Errorf("unsupported literal type: %s", n.Kind), fmt.Sprintf("Unsupported literal value: %s", n.Value))
		}

	case *ast.DeclStmt:
		return i.evalDeclStmt(ctx, n, env)

	case *ast.BinaryExpr:
		return i.evalBinaryExpr(ctx, n, env)

	case *ast.UnaryExpr:
		return i.evalUnaryExpr(ctx, n, env)

	case *ast.ParenExpr:
		return i.eval(ctx, n.X, env)

	case *ast.IfStmt:
		return i.evalIfStmt(ctx, n, env)

	case *ast.AssignStmt:
		return i.evalAssignStmt(ctx, n, env)

	case *ast.CallExpr:
		return i.evalCallExpr(ctx, n, env)

	case *ast.SelectorExpr:
		return i.evalSelectorExpr(ctx, n, env)

	case *ast.ReturnStmt:
		return i.evalReturnStmt(ctx, n, env)

	case *ast.FuncDecl:
		return i.evalFuncDecl(ctx, n, env)

	case *ast.FuncLit:
		return i.evalFuncLit(ctx, n, env)

	case *ast.ForStmt:
		return i.evalForStmt(ctx, n, env)

	case *ast.BranchStmt:
		return i.evalBranchStmt(ctx, n, env)

	case *ast.LabeledStmt:
		// Labels are handled by specific statements that use them (like break/continue).
		// For other statements, the label itself doesn't change evaluation.
		// We just evaluate the statement the label is attached to.
		// If a break/continue needs this label, its ast.BranchStmt.Label will be non-nil.
		return i.eval(ctx, n.Stmt, env)

	case *ast.CompositeLit:
		return i.evalCompositeLit(ctx, n, env)

	default:
		return nil, formatErrorWithContext(i.FileSet, n.Pos(), fmt.Errorf("unsupported AST node type: %T", n), fmt.Sprintf("Unsupported AST node value: %+v", n))
	}
}

func (i *Interpreter) evalCompositeLit(ctx context.Context, lit *ast.CompositeLit, env *Environment) (Object, error) {
	// 1. Evaluate the type of the composite literal.
	// For struct literals, this should be an *ast.Ident (the struct name).
	typeNameIdent, ok := lit.Type.(*ast.Ident)
	if !ok {
		// TODO: Handle other types of composite literals if minigo supports them later (e.g., arrays, slices, maps)
		return nil, formatErrorWithContext(i.FileSet, lit.Type.Pos(), fmt.Errorf("expected identifier for composite literal type, got %T", lit.Type), "Struct instantiation error")
	}

	// 2. Look up the StructDefinition in the environment.
	obj, found := env.Get(typeNameIdent.Name)
	if !found {
		return nil, formatErrorWithContext(i.FileSet, typeNameIdent.Pos(), fmt.Errorf("undefined type '%s' used in composite literal", typeNameIdent.Name), "Struct instantiation error")
	}
	structDef, ok := obj.(*StructDefinition)
	if !ok {
		return nil, formatErrorWithContext(i.FileSet, typeNameIdent.Pos(), fmt.Errorf("type '%s' is not a struct type, but %s", typeNameIdent.Name, obj.Type()), "Struct instantiation error")
	}

	// 3. Create a new StructInstance.
	instance := &StructInstance{
		Definition:     structDef,
		FieldValues:    make(map[string]Object),
		EmbeddedValues: make(map[string]*StructInstance), // Initialize map for embedded instances
	}

	// 4. Evaluate and assign field values.
	if len(lit.Elts) == 0 && len(structDef.Fields) > 0 {
		// Handle T{} - zero value initialization for all fields
		// This is more advanced as it requires knowing the zero value for each field type.
		// For now, an empty literal for a non-empty struct could mean an instance with no fields explicitly set,
		// or it could be an error, or it could mean all fields get their zero values.
		// Let's start by requiring explicit fields if the struct has fields.
		// Or, more simply for now: if Elts is empty, the FieldValues map remains empty. Accessing fields later would yield an error or nil.
		// A stricter approach: if structDef.Fields is not empty and lit.Elts is empty, this could be an error or imply zero-values.
		// For now, we'll allow it, and FieldValues will be empty. Accessing a field not in FieldValues can be handled by evalSelectorExpr.
	}


	expectedFieldCount := 0
	isKeyValueForm := false // True if first element is KeyValueExpr
	if len(lit.Elts) > 0 {
		_, isKeyValueForm = lit.Elts[0].(*ast.KeyValueExpr)
	}

	if isKeyValueForm { // Form: T{Key: Value, ...}
		for _, elt := range lit.Elts {
			kvExpr, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				return nil, formatErrorWithContext(i.FileSet, elt.Pos(), fmt.Errorf("mixture of keyed and non-keyed fields in struct literal for '%s' (or non-keyed field in keyed literal)", structDef.Name), "Struct instantiation error")
			}

			keyIdent, ok := kvExpr.Key.(*ast.Ident)
			if !ok {
				return nil, formatErrorWithContext(i.FileSet, kvExpr.Key.Pos(), fmt.Errorf("struct field key must be an identifier, got %T for struct '%s'", kvExpr.Key, structDef.Name), "Struct instantiation error")
			}
			fieldName := keyIdent.Name
			valueExpr := kvExpr.Value

			// TODO: Check for duplicate field names in literal.

			// Case 1: Direct field of the current struct
			if _, isDirectField := structDef.Fields[fieldName]; isDirectField {
				valObj, err := i.eval(ctx, valueExpr, env)
				if err != nil {
					return nil, err
				}
				// TODO: Type check valObj against structDef.Fields[fieldName]
				instance.FieldValues[fieldName] = valObj
				expectedFieldCount++
				continue
			}

			// Case 2: Field name is the type name of an embedded struct (explicit embedded struct initialization)
			var isEmbeddedTypeNameInitialization bool
			var targetEmbeddedDefForExplicitInit *StructDefinition
			for _, embDef := range structDef.EmbeddedDefs {
				if embDef.Name == fieldName {
					isEmbeddedTypeNameInitialization = true
					targetEmbeddedDefForExplicitInit = embDef
					break
				}
			}

			if isEmbeddedTypeNameInitialization {
				valObj, err := i.eval(ctx, valueExpr, env)
				if err != nil {
					return nil, err
				}
				embInstanceVal, ok := valObj.(*StructInstance)
				if !ok || embInstanceVal.Definition.Name != targetEmbeddedDefForExplicitInit.Name {
					return nil, formatErrorWithContext(i.FileSet, kvExpr.Value.Pos(),
						fmt.Errorf("value for embedded struct '%s' is not a compatible struct instance (expected '%s', got '%s')",
							fieldName, targetEmbeddedDefForExplicitInit.Name, valObj.Type()), "Struct instantiation error")
				}
				instance.EmbeddedValues[fieldName] = embInstanceVal
				expectedFieldCount++
				continue
			}

			// Case 3: Promoted field from an embedded struct
			var owningEmbDef *StructDefinition
			var foundPromotedCount int
			for _, embDef := range structDef.EmbeddedDefs {
				// Check direct fields of this embedded struct
				if _, isPromoted := embDef.Fields[fieldName]; isPromoted {
					if foundPromotedCount > 0 {
						return nil, formatErrorWithContext(i.FileSet, keyIdent.Pos(), fmt.Errorf("ambiguous promoted field '%s' in literal for type '%s'", fieldName, structDef.Name), "Struct instantiation error")
					}
					owningEmbDef = embDef
					foundPromotedCount++
				}
				// TODO: Recursively check fields of `embDef`'s own embedded structs.
				// This requires a helper like `findFieldDefinitionInEmbedded` to correctly identify the owning embedded struct.
				// For now, only checking one level of promotion.
			}

			if owningEmbDef != nil {
				// Get or create the instance for this embedded type
				embInstance, ok := instance.EmbeddedValues[owningEmbDef.Name]
				if !ok {
					embInstance = &StructInstance{
						Definition:     owningEmbDef,
						FieldValues:    make(map[string]Object),
						EmbeddedValues: make(map[string]*StructInstance), // For deeper embeddings
					}
					instance.EmbeddedValues[owningEmbDef.Name] = embInstance
				}

				valObj, err := i.eval(ctx, valueExpr, env)
				if err != nil {
					return nil, err
				}
				// TODO: Type check valObj against owningEmbDef.Fields[fieldName]
				embInstance.FieldValues[fieldName] = valObj
				expectedFieldCount++
				continue
			}

			// Case 4: Unknown field
			return nil, formatErrorWithContext(i.FileSet, keyIdent.Pos(), fmt.Errorf("unknown field '%s' in struct literal of type '%s'", fieldName, structDef.Name), "Struct instantiation error")
		}
	} else { // Form: T{Value1, Value2, ...} - Order matters
		// This form is harder because ast.StructType.Fields.List gives us fields, but their order
		// might not be easily accessible or guaranteed in the same way map iteration isn't.
		// For simplicity, MiniGo will initially NOT support this unkeyed form if fields are present.
		// Or, require the number of values to match the number of fields.
		// Go requires this form to either provide all fields or be empty T{}.
		// Let's disallow this form for now if struct has fields and Elts is not empty.
		if len(lit.Elts) > 0 && len(structDef.Fields) > 0 {
			return nil, formatErrorWithContext(i.FileSet, lit.Pos(), fmt.Errorf("ordered (non-keyed) struct literal values are not supported yet for struct '%s'; use key:value form or ensure the struct has no fields if using T{}", structDef.Name), "Struct instantiation error")
		}
		// If structDef.Fields is empty and lit.Elts is also empty (e.g. type EmptyStruct struct{}; e := EmptyStruct{}), this is fine.
	}

	// Optional: Check if all fields defined in StructDefinition are present if using keyed form,
	// or if a policy of requiring all fields is desired. Go allows unkeyed fields to take zero values.
	// For now, we allow partial initialization. Fields not in the literal will not be in instance.FieldValues.

	return instance, nil
}


func (i *Interpreter) evalBranchStmt(ctx context.Context, stmt *ast.BranchStmt, env *Environment) (Object, error) {
	if stmt.Label != nil {
		return nil, formatErrorWithContext(i.FileSet, stmt.Pos(), fmt.Errorf("labeled break/continue not supported"), "")
	}

	switch stmt.Tok {
	case token.BREAK:
		return BREAK, nil
	case token.CONTINUE:
		return CONTINUE, nil
	default:
		return nil, formatErrorWithContext(i.FileSet, stmt.Pos(), fmt.Errorf("unsupported branch statement: %s", stmt.Tok), "")
	}
}

func (i *Interpreter) evalForStmt(ctx context.Context, stmt *ast.ForStmt, env *Environment) (Object, error) {
	// For loops create a new scope for their initialization, condition, post, and body.
	loopEnv := NewEnvironment(env)

	// 1. Initialization
	if stmt.Init != nil {
		if _, err := i.eval(ctx, stmt.Init, loopEnv); err != nil {
			return nil, err
		}
	}

	for {
		// 2. Condition
		if stmt.Cond != nil {
			condition, err := i.eval(ctx, stmt.Cond, loopEnv)
			if err != nil {
				return nil, err
			}
			boolCond, ok := condition.(*Boolean)
			if !ok {
				return nil, formatErrorWithContext(i.FileSet, stmt.Cond.Pos(),
					fmt.Errorf("condition for for statement must be a boolean, got %s (type: %s)", condition.Inspect(), condition.Type()), "")
			}
			if !boolCond.Value {
				break // Exit loop if condition is false
			}
		} else {
			// No condition means an infinite loop, effectively `for true {}`
			// unless broken by other means (not yet supported: break/return)
		}

		// 3. Body
		// The body of the loop also executes in its own sub-scope, but inherits from loopEnv.
		// This is important if the body itself contains declarations that should not
		// persist across iterations or conflict with the loop's own variables (like the iterator in some languages).
		// However, for simple for loops as in Go, the init/cond/post variables are in the same scope as the body.
		// So, we'll use loopEnv directly for the body. If we were to support `break` or `continue` with labels,
		// or more complex scoping within loops (e.g. Python's for-else), this might need adjustment.
		// For now, a single loopEnv for init, cond, post, and body is consistent with Go's for loop.
		bodyResult, err := i.evalBlockStatement(ctx, stmt.Body, loopEnv)
		if err != nil {
			return nil, err
		}

		// Check for ReturnValue, Error, Break, or Continue from the body
		var broke bool // Flag to indicate if a break occurred
		switch res := bodyResult.(type) {
		case *ReturnValue:
			return res, nil // Propagate return
		case *Error:
			return res, nil // Propagate error
		case *BreakStatement:
			broke = true // Signal to break the outer Go for loop
		case *ContinueStatement:
			// Skip to the post statement, then next iteration
			if stmt.Post != nil {
				if _, postErr := i.eval(ctx, stmt.Post, loopEnv); postErr != nil {
					return nil, postErr
				}
			}
			continue // Go to the next iteration of the Go `for` loop
		}

		if broke {
			break // Break the Go `for` loop
		}

		// 4. Post-iteration statement
		// Only execute if we didn't break out of the loop
		if !broke && stmt.Post != nil {
			if _, err := i.eval(ctx, stmt.Post, loopEnv); err != nil {
				return nil, err
			}
		}
	}

	return NULL, nil // For statement itself doesn't produce a value
}

func (i *Interpreter) evalSelectorExpr(ctx context.Context, node *ast.SelectorExpr, env *Environment) (Object, error) {
	// Evaluate the expression on the left of the selector (node.X)
	// This could be an identifier (variable holding a struct instance) or another expression.
	xObj, err := i.eval(ctx, node.X, env)
	if err != nil {
		return nil, err // Error already formatted by i.eval
	}

	fieldName := node.Sel.Name

	// Check if xObj is a struct instance
	if structInstance, ok := xObj.(*StructInstance); ok {
		// 1. Check direct fields that were explicitly set
		if val, found := structInstance.FieldValues[fieldName]; found {
			return val, nil
		}

		// 2. Check if it's a defined direct field but not explicitly set (uninitialized)
		if _, isDirectField := structInstance.Definition.Fields[fieldName]; isDirectField {
			// Field is defined on the struct but wasn't in FieldValues (not explicitly initialized).
			// Return NULL for now. Later, this could be a zero value.
			return NULL, nil
		}

		// 3. Search in embedded structs
		// This requires a recursive search function to handle multiple levels of embedding
		// and ambiguity detection.
		foundValue, err := findFieldInEmbedded(structInstance, fieldName, i.FileSet, node.Sel.Pos())
		if err != nil {
			return nil, err // Error already formatted by findFieldInEmbedded
		}
		if foundValue != nil {
			return foundValue, nil
		}

		// 4. If not found in direct or embedded fields, it's an undefined field for this struct type.
		return nil, formatErrorWithContext(i.FileSet, node.Sel.Pos(), fmt.Errorf("type %s has no field %s", structInstance.Definition.Name, fieldName), "Field access error")
	}

	// Check if xObj is an identifier representing a package (for package.Symbol access)
	if xIdent, ok := node.X.(*ast.Ident); ok {
		localPkgName := xIdent.Name
		qualifiedNameInEnv := localPkgName + "." + fieldName // Symbol name is fieldName here

		// Check if the symbol is already in the environment (e.g., from a previous import of this pkg)
		if val, found := env.Get(qualifiedNameInEnv); found {
			return val, nil
		}

		// If not in env, check if we have an import path for this localPkgName
		importPath, knownAlias := i.importAliasMap[localPkgName]
		if !knownAlias {
			return nil, formatErrorWithContext(i.FileSet, xIdent.Pos(), fmt.Errorf("undefined: %s (neither struct instance nor package)", localPkgName), "Selector error")
		}

		// We have an importPath. Now check if this importPath has already been processed.
		if _, alreadyImported := i.importedPackages[importPath]; !alreadyImported {
			if i.sharedScanner == nil {
				return nil, formatErrorWithContext(i.FileSet, node.X.Pos(), errors.New("shared go-scan scanner (for imports) not initialized in interpreter"), "Internal error")
			}

			var importPkgInfo *scanner.PackageInfo
			var errImport error
			importPkgInfo, errImport = i.sharedScanner.ScanPackageByImport(ctx, importPath)

			if errImport != nil {
				return nil, formatErrorWithContext(i.FileSet, xIdent.Pos(), fmt.Errorf("package %q (aliased as %q) not found or failed to scan: %w", importPath, localPkgName, errImport), "Import error")
			}

			if importPkgInfo != nil {
				// Process constants from the imported package
				for _, c := range importPkgInfo.Constants {
					if !c.IsExported {
						continue
					}
					var constObj Object
					if c.Type != nil {
						switch c.Type.Name {
						case "int", "int64", "int32", "uint", "uint64", "uint32", "rune", "byte":
							valInt, errParse := parseInt64(c.Value)
							if errParse == nil {
								constObj = &Integer{Value: valInt}
							} else {
								fmt.Fprintf(os.Stderr, "Warning: Could not parse external const integer %s.%s (value: %s): %v\n", localPkgName, c.Name, c.Value, errParse)
							}
						case "string":
							unquotedVal, errParse := strconv.Unquote(c.Value)
							if errParse == nil {
								constObj = &String{Value: unquotedVal}
							} else {
								fmt.Fprintf(os.Stderr, "Warning: Could not unquote external const string %s.%s (value: %s): %v\n", localPkgName, c.Name, c.Value, errParse)
							}
						case "bool":
							switch c.Value {
							case "true":
								constObj = TRUE
							case "false":
								constObj = FALSE
							default:
								fmt.Fprintf(os.Stderr, "Warning: Could not parse external const bool %s.%s (value: %s)\n", localPkgName, c.Name, c.Value)
							}
						default:
							fmt.Fprintf(os.Stderr, "Warning: Unsupported external const type %s for %s.%s\n", c.Type.Name, localPkgName, c.Name)
						}
					} else {
						fmt.Fprintf(os.Stderr, "Warning: External const %s.%s has no type info, cannot determine type for value: %s\n", localPkgName, c.Name, c.Value)
					}
					if constObj != nil {
						env.Define(localPkgName+"."+c.Name, constObj)
					}
				}

				// Process functions from the imported package
				for _, fInfo := range importPkgInfo.Functions {
					if !ast.IsExported(fInfo.Name) || fInfo.AstDecl == nil {
						continue
					}
					params := []*ast.Ident{}
					if fInfo.AstDecl.Type.Params != nil {
						for _, field := range fInfo.AstDecl.Type.Params.List {
							params = append(params, field.Names...)
						}
					}
					importedFunc := &UserDefinedFunction{
						Name:       fInfo.Name,
						Parameters: params,
						Body:       fInfo.AstDecl.Body,
						Env:        env,
						FileSet:    i.sharedScanner.Fset(),
					}
					env.Define(localPkgName+"."+fInfo.Name, importedFunc)
				}
			}
			i.importedPackages[importPath] = struct{}{}
		}

		// After attempting import and processing, try getting the symbol again
		if val, found := env.Get(qualifiedNameInEnv); found {
			return val, nil
		}
		// If still not found after import processing for package.Symbol.
		return nil, formatErrorWithContext(i.FileSet, node.Sel.Pos(), fmt.Errorf("undefined: %s.%s (package %s, path %s)", localPkgName, fieldName, localPkgName, importPath), "Selector error")
	}

	// If xObj is not a struct instance and not a package identifier, it's an unsupported selector base.
	return nil, formatErrorWithContext(i.FileSet, node.X.Pos(), fmt.Errorf("selector base must be a struct instance or package identifier, got %s", xObj.Type()), "Unsupported selector expression")
}

// findFieldInEmbedded recursively searches for a field within the embedded structs of a given StructInstance.
// It returns:
//   - foundValue: The Object found (can be NULL if the field is defined but uninitialized).
//   - found:      A boolean indicating if the field was found (even if its value is NULL).
//   - foundIn:    The name of the struct definition where the field was ultimately found.
//   - err:        An error, typically for ambiguity.
func findFieldInEmbedded(instance *StructInstance, fieldName string, fset *token.FileSet, selPos token.Pos) (foundValue Object, found bool, foundIn string, err error) {
	var overallFoundValue Object
	var overallFoundInDefinitionName string
	var numFoundPaths int = 0

	for _, embDef := range instance.Definition.EmbeddedDefs {
		embInstance, embInstanceExists := instance.EmbeddedValues[embDef.Name]
		if !embInstanceExists {
			// If an embedded struct was not initialized in the literal, its instance won't exist here.
			// Go would treat its fields as accessible and having zero values.
			// For MiniGo, if the embInstance isn't there, we can't get fields from it.
			// This path implies the field is sought through an uninitialized embedded struct.
			// We must check if `fieldName` *would* be in `embDef`.
			if _, isFieldInEmbDef := embDef.Fields[fieldName]; isFieldInEmbDef {
				// The field *is* defined in this uninitialized embedded struct.
				// According to Go's rules, this should resolve and yield a zero value.
				// For MiniGo, we return NULL.
				if numFoundPaths > 0 && overallFoundInDefinitionName != embDef.Name { // Check if already found in a *different* embedded path
					return nil, false, "", formatErrorWithContext(fset, selPos,
						fmt.Errorf("ambiguous selector %s (found in %s and as uninitialized field in %s)", fieldName, overallFoundInDefinitionName, embDef.Name), "")
				}
				overallFoundValue = NULL
				overallFoundInDefinitionName = embDef.Name
				numFoundPaths++
				// Continue to check for ambiguity with other embedded structs
			} else {
				// Recursively check deeper embeddings within this embDef, even if embInstance doesn't exist directly,
				// the definition (embDef) might have further embeddings.
				// This requires passing the definition `embDef` to a recursive call.
				// Let's simplify for now: if embInstance is nil, we only consider direct fields of embDef (as done above).
				// Deeper recursion without an instance is complex.
			}
			continue // Move to the next embedded definition.
		}

		// Path 1: Field is a direct, explicitly set field of the current embedded instance.
		if val, isSet := embInstance.FieldValues[fieldName]; isSet {
			if numFoundPaths > 0 && overallFoundInDefinitionName != embDef.Name {
				return nil, false, "", formatErrorWithContext(fset, selPos,
					fmt.Errorf("ambiguous selector %s (found in %s and as set field in %s)", fieldName, overallFoundInDefinitionName, embDef.Name), "")
			}
			overallFoundValue = val
			overallFoundInDefinitionName = embDef.Name
			numFoundPaths++
			continue // Found directly, check next sibling for ambiguity.
		}

		// Path 2: Field is a defined direct field of the embedded struct, but not explicitly set (uninitialized).
		if _, isDirectField := embDef.Fields[fieldName]; isDirectField {
			if numFoundPaths > 0 && overallFoundInDefinitionName != embDef.Name {
				return nil, false, "", formatErrorWithContext(fset, selPos,
					fmt.Errorf("ambiguous selector %s (found in %s and as uninitialized field in %s)", fieldName, overallFoundInDefinitionName, embDef.Name), "")
			}
			overallFoundValue = NULL
			overallFoundInDefinitionName = embDef.Name
			numFoundPaths++
			continue // Found as uninitialized, check next sibling for ambiguity.
		}

		// Path 3: Recursively search in deeper embedded structs of this current embedded instance.
		// Only proceed if we haven't found a more direct version of the field in *this* embDef.
		// (Direct fields of embDef shadow its own embedded fields).
		recVal, recFound, recIn, recErr := findFieldInEmbedded(embInstance, fieldName, fset, selPos)
		if recErr != nil {
			return nil, false, "", recErr // Propagate ambiguity error from deeper level
		}
		if recFound {
			if numFoundPaths > 0 && overallFoundInDefinitionName != embDef.Name { // `embDef.Name` here means "found via this path"
				// If already found via a different top-level embedded struct, it's ambiguous.
				return nil, false, "", formatErrorWithContext(fset, selPos,
					fmt.Errorf("ambiguous selector %s (found in %s and via deeper embedding in %s through %s)", fieldName, overallFoundInDefinitionName, recIn, embDef.Name), "")
			}
			overallFoundValue = recVal
			overallFoundInDefinitionName = recIn // Or embDef.Name + "." + recIn for a path
			numFoundPaths++
			// Continue, an ambiguity might arise with the next sibling embDef.
		}
	}

	if numFoundPaths > 1 {
		// This specific check might be redundant if ambiguity is caught when `numFoundPaths` increments to 2.
		// However, it's a final safeguard. The error message here might be generic.
		return nil, false, "", formatErrorWithContext(fset, selPos,
			fmt.Errorf("ambiguous selector %s (found in multiple embedded structs)", fieldName), "")
	}

	if numFoundPaths == 1 {
		return overallFoundValue, true, overallFoundInDefinitionName, nil
	}

	return nil, false, "", nil // Not found
}


func (i *Interpreter) evalBlockStatement(ctx context.Context, block *ast.BlockStmt, env *Environment) (Object, error) {
	var result Object
	var err error

	for _, stmt := range block.List {
		result, err = i.eval(ctx, stmt, env)
		if err != nil {
			return nil, err
		}
		switch res := result.(type) {
		case *ReturnValue, *Error, *BreakStatement, *ContinueStatement:
			// If any of these control flow objects are encountered,
			// stop executing statements in this block and propagate the object.
			return res, nil
		}
	}
	return result, nil // Return the result of the last statement if no control flow object was encountered.
}

func (i *Interpreter) evalFuncDecl(ctx context.Context, fd *ast.FuncDecl, env *Environment) (Object, error) {
	params := []*ast.Ident{}
	if fd.Type.Params != nil && fd.Type.Params.List != nil {
		for _, field := range fd.Type.Params.List {
			if field.Names != nil {
				for _, name := range field.Names {
					params = append(params, name)
				}
			}
		}
	}

	function := &UserDefinedFunction{
		Name:       fd.Name.Name,
		Parameters: params,
		Body:       fd.Body,
		Env:        env,
		FileSet:    i.FileSet, // Set the FileSet
	}

	if fd.Name != nil && fd.Name.Name != "" {
		env.Define(fd.Name.Name, function)
		return nil, nil
	}
	return nil, formatErrorWithContext(i.FileSet, fd.Pos(), fmt.Errorf("function declaration must have a name"), "")
}

func (i *Interpreter) evalFuncLit(ctx context.Context, fl *ast.FuncLit, env *Environment) (Object, error) {
	params := []*ast.Ident{}
	if fl.Type.Params != nil && fl.Type.Params.List != nil {
		for _, field := range fl.Type.Params.List {
			if field.Names != nil {
				for _, name := range field.Names {
					params = append(params, name)
				}
			}
		}
	}

	return &UserDefinedFunction{
		Name:       "",
		Parameters: params,
		Body:       fl.Body,
		Env:        env,
		FileSet:    i.FileSet, // Set the FileSet
	}, nil
}

func (i *Interpreter) evalReturnStmt(ctx context.Context, rs *ast.ReturnStmt, env *Environment) (Object, error) {
	if len(rs.Results) == 0 {
		return &ReturnValue{Value: NULL}, nil
	}

	if len(rs.Results) > 1 {
		return nil, formatErrorWithContext(i.FileSet, rs.Pos(), fmt.Errorf("multiple return values not supported"), "")
	}

	val, err := i.eval(ctx, rs.Results[0], env)
	if err != nil {
		return nil, err
	}
	return &ReturnValue{Value: val}, nil
}

func (i *Interpreter) evalDeclStmt(ctx context.Context, declStmt *ast.DeclStmt, env *Environment) (Object, error) {
	genDecl, ok := declStmt.Decl.(*ast.GenDecl)
	if !ok {
		return nil, formatErrorWithContext(i.FileSet, declStmt.Pos(), fmt.Errorf("unsupported declaration type: %T", declStmt.Decl), "")
	}

	switch genDecl.Tok {
	case token.VAR:
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				return nil, formatErrorWithContext(i.FileSet, spec.Pos(), fmt.Errorf("unsupported spec type in var declaration: %T", spec), "")
			}

			for idx, nameIdent := range valueSpec.Names {
				varName := nameIdent.Name
				if len(valueSpec.Values) > idx {
					val, err := i.eval(ctx, valueSpec.Values[idx], env)
					if err != nil {
						return nil, err
					}
					env.Define(varName, val)
				} else {
					if valueSpec.Type == nil {
						return nil, formatErrorWithContext(i.FileSet, valueSpec.Pos(), fmt.Errorf("variable '%s' declared without initializer must have a type", varName), "")
					}

					var zeroVal Object
					switch T := valueSpec.Type.(type) {
					case *ast.Ident:
						switch T.Name {
						case "string":
							zeroVal = &String{Value: ""}
						case "int":
							zeroVal = &Integer{Value: 0}
						case "bool":
							zeroVal = FALSE
						default:
							// Could be a struct type name, attempt to find its definition for zero value
							typeObj, typeFound := env.Get(T.Name)
							if typeFound && typeObj.Type() == STRUCT_DEF_OBJ {
								// This part is complex: creating a zero-value struct instance.
								// For now, error out, as full zero-value struct instantiation is not implemented.
								return nil, formatErrorWithContext(i.FileSet, T.Pos(), fmt.Errorf("unsupported type '%s' for uninitialized variable '%s' (struct zero values not fully implemented yet)", T.Name, varName), "")
							}
							return nil, formatErrorWithContext(i.FileSet, T.Pos(), fmt.Errorf("unsupported type '%s' for uninitialized variable '%s'", T.Name, varName), "")
						}
					case *ast.InterfaceType:
						if T.Methods == nil || len(T.Methods.List) == 0 {
							zeroVal = NULL
						} else {
							return nil, formatErrorWithContext(i.FileSet, T.Pos(), fmt.Errorf("unsupported specific interface type for uninitialized variable '%s'", varName), "")
						}
					default:
						return nil, formatErrorWithContext(i.FileSet, valueSpec.Type.Pos(), fmt.Errorf("unsupported type expression for zero value for variable '%s': %T", varName, valueSpec.Type), "")
					}
					env.Define(varName, zeroVal)
				}
			}
		}
	case token.TYPE:
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				return nil, formatErrorWithContext(i.FileSet, spec.Pos(), fmt.Errorf("unsupported spec type in type declaration: %T", spec), "")
			}
			typeName := typeSpec.Name.Name

			switch sType := typeSpec.Type.(type) {
			case *ast.StructType:
				directFields := make(map[string]string)
				var embeddedDefs []*StructDefinition
				var fieldOrder []string

				if sType.Fields != nil {
					for _, field := range sType.Fields.List {
						var fieldTypeName string
						var isEmbedded bool

						// Determine the type name of the field or embedded struct
						// and whether it's an embedded field.
						switch typeExpr := field.Type.(type) {
						case *ast.Ident:
							fieldTypeName = typeExpr.Name
						case *ast.SelectorExpr: // For pkg.Type
							// For simplicity, assume SelectorExpr is for qualified type names (pkg.Type)
							// and we don't resolve external packages for struct defs yet in MiniGo.
							// If field.Names is empty, this would be an embedded pkg.Type.
							// If field.Names is not empty, it's a normal field of type pkg.Type.
							// Current MiniGo struct fields only support simple idents like "int".
							// This part needs careful extension if we support pkg.Type for fields or embedding.
							// For now, let's assume embedded types are *ast.Ident.
							// And direct fields are also *ast.Ident for their types.
							return nil, formatErrorWithContext(i.FileSet, field.Type.Pos(), fmt.Errorf("field type '%s' in struct '%s' uses SelectorExpr, which is not fully supported for struct field types or embedding yet", astNodeToString(typeExpr, i.FileSet), typeName), "Struct definition error")
						default:
							return nil, formatErrorWithContext(i.FileSet, field.Type.Pos(), fmt.Errorf("struct field in '%s' has unsupported type specifier %T", typeName, field.Type), "Struct definition error")
						}

						// Check if it's an embedded field
						// An embedded field typically has no name, or its name is the same as its type.
						if len(field.Names) == 0 { // Anonymous field, e.g., `string` or `MyStruct`
							isEmbedded = true
						} else if len(field.Names) == 1 && field.Names[0].Name == fieldTypeName {
							// Named field where name matches type, e.g. `Point Point`. Go treats this as embedding.
							// However, `go/ast` seems to parse `Point` alone as Type=Ident{Point}, Names=nil.
							// And `P Point` as Type=Ident{Point}, Names=[Ident{P}].
							// So, `len(field.Names) == 0` is the primary check for typical embedding.
							// Let's stick to `len(field.Names) == 0` for simple anonymous embedding.
							// The case `P P` where P is a type would be a regular field P of type P.
						}


						if isEmbedded {
							fieldOrder = append(fieldOrder, fieldTypeName) // Record embedded type name in order

							// Look up the definition of the embedded struct.
							obj, found := env.Get(fieldTypeName)
							if !found {
								return nil, formatErrorWithContext(i.FileSet, field.Type.Pos(), fmt.Errorf("undefined type '%s' embedded in struct '%s'", fieldTypeName, typeName), "Struct definition error")
							}
							embeddedDef, ok := obj.(*StructDefinition)
							if !ok {
								return nil, formatErrorWithContext(i.FileSet, field.Type.Pos(), fmt.Errorf("type '%s' embedded in struct '%s' is not a struct definition (got %s)", fieldTypeName, typeName, obj.Type()), "Struct definition error")
							}
							embeddedDefs = append(embeddedDefs, embeddedDef)
						} else {
							// Regular named field
							// Ensure fieldTypeIdent is *ast.Ident for type name
							fieldTypeIdent, ok := field.Type.(*ast.Ident)
							if !ok {
								// This case might be redundant if the switch above handles all typeExpr variants,
								// but good for safety if only *ast.Ident is supported for field types.
								return nil, formatErrorWithContext(i.FileSet, field.Type.Pos(), fmt.Errorf("struct field '%s' in '%s' has complex type specifier %T; only simple type names supported for fields", field.Names[0].Name, typeName, field.Type), "")
							}

							for _, nameIdent := range field.Names {
								directFields[nameIdent.Name] = fieldTypeIdent.Name // Store type name as string
								fieldOrder = append(fieldOrder, nameIdent.Name)   // Record field name in order
							}
						}
					}
				}
				structDef := &StructDefinition{
					Name:          typeName,
					Fields:        directFields,
					EmbeddedDefs:  embeddedDefs,
					FieldOrder:    fieldOrder,
				}
				env.Define(typeName, structDef)
			default:
				return nil, formatErrorWithContext(i.FileSet, typeSpec.Type.Pos(), fmt.Errorf("unsupported type specifier in type declaration '%s': %T", typeName, typeSpec.Type), "")
			}
		}
	default:
		return nil, formatErrorWithContext(i.FileSet, genDecl.Pos(), fmt.Errorf("unsupported declaration token: %s (expected VAR or TYPE)", genDecl.Tok), "")
	}
	// Processing is done within the switch cases.
	return nil, nil
}

func evalIdentifier(ident *ast.Ident, env *Environment, fset *token.FileSet) (Object, error) {
	switch ident.Name {
	case "true":
		return TRUE, nil
	case "false":
		return FALSE, nil
	}
	if val, ok := env.Get(ident.Name); ok {
		return val, nil
	}
	return nil, formatErrorWithContext(fset, ident.Pos(), fmt.Errorf("identifier not found: %s", ident.Name), "")
}

func (i *Interpreter) evalBinaryExpr(ctx context.Context, node *ast.BinaryExpr, env *Environment) (Object, error) {
	left, err := i.eval(ctx, node.X, env)
	if err != nil {
		return nil, err
	}
	right, err := i.eval(ctx, node.Y, env)
	if err != nil {
		return nil, err
	}

	switch {
	case left.Type() == STRING_OBJ && right.Type() == STRING_OBJ:
		return evalStringBinaryExpr(node.Op, left.(*String), right.(*String), i.FileSet, node.Pos())
	case left.Type() == INTEGER_OBJ && right.Type() == INTEGER_OBJ:
		return evalIntegerBinaryExpr(node.Op, left.(*Integer), right.(*Integer), i.FileSet, node.Pos())
	case left.Type() == BOOLEAN_OBJ && right.Type() == BOOLEAN_OBJ:
		// TODO: Implement short-circuiting for token.LAND and token.LOR
		// Currently, both left and right operands are evaluated before this point.
		// For true short-circuiting, the evaluation of the right operand
		// would need to be conditional within these cases.
		return evalBooleanBinaryExpr(node.Op, left.(*Boolean), right.(*Boolean), i.FileSet, node.Pos())
	default:
		return nil, formatErrorWithContext(i.FileSet, node.Pos(),
			fmt.Errorf("type mismatch or unsupported operation for binary expression: %s %s %s (left: %s, right: %s)", left.Type(), node.Op, right.Type(), left.Inspect(), right.Inspect()), "")
	}
}

func evalIntegerBinaryExpr(op token.Token, left, right *Integer, fset *token.FileSet, pos token.Pos) (Object, error) {
	leftVal := left.Value
	rightVal := right.Value

	switch op {
	case token.ADD:
		return &Integer{Value: leftVal + rightVal}, nil
	case token.SUB:
		return &Integer{Value: leftVal - rightVal}, nil
	case token.MUL:
		return &Integer{Value: leftVal * rightVal}, nil
	case token.QUO:
		if rightVal == 0 {
			return nil, formatErrorWithContext(fset, pos, fmt.Errorf("division by zero"), "")
		}
		return &Integer{Value: leftVal / rightVal}, nil
	case token.REM:
		if rightVal == 0 {
			return nil, formatErrorWithContext(fset, pos, fmt.Errorf("division by zero (modulo)"), "")
		}
		return &Integer{Value: leftVal % rightVal}, nil
	case token.EQL:
		return nativeBoolToBooleanObject(leftVal == rightVal), nil
	case token.NEQ:
		return nativeBoolToBooleanObject(leftVal != rightVal), nil
	case token.LSS:
		return nativeBoolToBooleanObject(leftVal < rightVal), nil
	case token.LEQ:
		return nativeBoolToBooleanObject(leftVal <= rightVal), nil
	case token.GTR:
		return nativeBoolToBooleanObject(leftVal > rightVal), nil
	case token.GEQ:
		return nativeBoolToBooleanObject(leftVal >= rightVal), nil
	default:
		return nil, formatErrorWithContext(fset, pos, fmt.Errorf("unknown operator for integers: %s", op), "")
	}
}

func evalStringBinaryExpr(op token.Token, left, right *String, fset *token.FileSet, pos token.Pos) (Object, error) {
	switch op {
	case token.EQL:
		return nativeBoolToBooleanObject(left.Value == right.Value), nil
	case token.NEQ:
		return nativeBoolToBooleanObject(left.Value != right.Value), nil
	case token.ADD:
		return &String{Value: left.Value + right.Value}, nil
	default:
		return nil, formatErrorWithContext(fset, pos, fmt.Errorf("unknown operator for strings: %s (left: %q, right: %q)", op, left.Value, right.Value), "")
	}
}

func evalBooleanBinaryExpr(op token.Token, left, right *Boolean, fset *token.FileSet, pos token.Pos) (Object, error) {
	leftVal := left.Value
	rightVal := right.Value

	switch op {
	case token.EQL:
		return nativeBoolToBooleanObject(leftVal == rightVal), nil
	case token.NEQ:
		return nativeBoolToBooleanObject(leftVal != rightVal), nil
	case token.LAND: // &&
		return nativeBoolToBooleanObject(leftVal && rightVal), nil
	case token.LOR: // ||
		return nativeBoolToBooleanObject(leftVal || rightVal), nil
	default:
		// Return a generic unsupported operation error for consistency with other types
		return nil, formatErrorWithContext(fset, pos,
			fmt.Errorf("type mismatch or unsupported operation for binary expression: %s %s %s", left.Type(), op, right.Type()), "")
	}
}

func (i *Interpreter) evalCallExpr(ctx context.Context, node *ast.CallExpr, env *Environment) (Object, error) {
	funcObj, err := i.eval(ctx, node.Fun, env)
	if err != nil {
		return nil, err
	}

	args := make([]Object, len(node.Args))
	for idx, argExpr := range node.Args {
		argVal, err := i.eval(ctx, argExpr, env)
		if err != nil {
			return nil, err
		}
		args[idx] = argVal
	}

	switch fn := funcObj.(type) {
	case *BuiltinFunction:
		return fn.Fn(env, args...) // BuiltinFunction does not need ctx
	case *UserDefinedFunction:
		return i.applyUserDefinedFunction(ctx, fn, args, node.Fun.Pos())
	default:
		funcName := "unknown"
		if ident, ok := node.Fun.(*ast.Ident); ok {
			funcName = ident.Name
		} else if selExpr, ok := node.Fun.(*ast.SelectorExpr); ok {
			if xIdent, okX := selExpr.X.(*ast.Ident); okX {
				funcName = xIdent.Name + "." + selExpr.Sel.Name
			}
		}
		return nil, formatErrorWithContext(i.FileSet, node.Fun.Pos(), fmt.Errorf("cannot call non-function type %s (for function '%s')", funcObj.Type(), funcName), "")
	}
}

func (i *Interpreter) evalAssignStmt(ctx context.Context, assignStmt *ast.AssignStmt, env *Environment) (Object, error) {
	if len(assignStmt.Lhs) != 1 || len(assignStmt.Rhs) != 1 {
		return nil, formatErrorWithContext(i.FileSet, assignStmt.Pos(),
			fmt.Errorf("unsupported assignment: expected 1 expression on LHS and 1 on RHS, got %d and %d", len(assignStmt.Lhs), len(assignStmt.Rhs)), "")
	}

	lhs := assignStmt.Lhs[0]
	ident, ok := lhs.(*ast.Ident)
	if !ok {
		return nil, formatErrorWithContext(i.FileSet, lhs.Pos(), fmt.Errorf("unsupported assignment LHS: expected identifier, got %T", lhs), "")
	}
	varName := ident.Name

	val, err := i.eval(ctx, assignStmt.Rhs[0], env)
	if err != nil {
		return nil, err
	}

	switch assignStmt.Tok {
	case token.DEFINE: // :=
		// Check if variable already exists in the current scope (not outer)
		// For simplicity, MiniGo's env.Get checks all scopes.
		// A strict `:=` would error if `env.GetInCurrentScope(varName)` is true.
		// Our current Environment doesn't distinguish Get from current vs outer for this check easily.
		// So, we'll rely on Define to implicitly handle this "new variable" nature.
		// If Define were to allow re-definition in the same scope, this would be wrong.
		// Let's assume Define in the same scope is like an assignment,
		// or we need a way to check "exists in current scope only".
		// For MiniGo's purpose, `:=` should always create a new variable.
		// If `env.Get(varName)` returns true, it means it exists somewhere.
		// Go's `:=` allows shadowing. If `varName` exists in an outer scope, `:=` creates a new one in the current scope.
		// If `varName` exists in the current scope, `:=` is an error ("no new variables on left side of :=").

		// Simplified check: if it exists at all and we try to `:=`, it's an error if we don't allow shadowing.
		// Our `env.Define` effectively shadows if called in a nested environment.
		// If in the *same* environment, `env.Define` overwrites.
		// This part needs care.
		// For `:=`, it must define a *new* variable.
		// Go rule: "no new variables on left side of :=" means at least one variable on LHS must be new in the current block.
		// Since we only support single var LHS for now:
		if env.ExistsInCurrentScope(varName) {
			return nil, formatErrorWithContext(i.FileSet, ident.Pos(), fmt.Errorf("no new variables on left side of := (variable '%s' already declared in this scope)", varName), "")
		}
		env.Define(varName, val) // Define in current environment
		return nil, nil

	case token.ASSIGN: // =
		if _, ok := env.Assign(varName, val); !ok {
			return nil, formatErrorWithContext(i.FileSet, ident.Pos(), fmt.Errorf("cannot assign to undeclared variable '%s'", varName), "")
		}
		return nil, nil

	default: // Augmented assignments: +=, -=, etc.
		existingVal, ok := env.Get(varName)
		if !ok {
			return nil, formatErrorWithContext(i.FileSet, ident.Pos(), fmt.Errorf("cannot use %s on undeclared variable '%s'", assignStmt.Tok, varName), "")
		}

		// Determine the binary operation token corresponding to the assignment token
		var op token.Token
		switch assignStmt.Tok {
		case token.ADD_ASSIGN:
			op = token.ADD
		case token.SUB_ASSIGN:
			op = token.SUB
		case token.MUL_ASSIGN:
			op = token.MUL
		case token.QUO_ASSIGN:
			op = token.QUO
		case token.REM_ASSIGN:
			op = token.REM
		default:
			return nil, formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("unsupported assignment operator %s", assignStmt.Tok), "")
		}

		// Perform the operation based on type
		var resultVal Object
		var evalErr error

		switch eVal := existingVal.(type) {
		case *Integer:
			if vInt, okV := val.(*Integer); okV {
				// Use evalIntegerBinaryExpr for the core logic to avoid duplication
				tempBinExprResult, binErr := evalIntegerBinaryExpr(op, eVal, vInt, i.FileSet, assignStmt.Pos())
				if binErr != nil {
					return nil, formatErrorWithContext(i.FileSet, assignStmt.Pos(), binErr, fmt.Sprintf("error in augmented assignment %s for variable '%s'", assignStmt.Tok, varName))
				}
				resultVal = tempBinExprResult
			} else {
				evalErr = formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("type mismatch for %s: existing value is INTEGER, new value is %s", assignStmt.Tok, val.Type()), "")
			}
		case *String:
			if op == token.ADD { // Only += is supported for strings
				if vStr, okV := val.(*String); okV {
					resultVal = &String{Value: eVal.Value + vStr.Value}
				} else {
					evalErr = formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("type mismatch for string concatenation (+=): existing value is STRING, new value is %s", val.Type()), "")
				}
			} else {
				evalErr = formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("unsupported operator %s for augmented string assignment (only += is allowed)", assignStmt.Tok), "")
			}
		default:
			evalErr = formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("unsupported type %s for augmented assignment operator %s on variable '%s'", existingVal.Type(), assignStmt.Tok, varName), "")
		}

		if evalErr != nil {
			return nil, evalErr
		}

		// Assign the new value back to the variable
		if _, ok := env.Assign(varName, resultVal); !ok {
			// This should not happen if the variable was successfully fetched earlier
			return nil, formatErrorWithContext(i.FileSet, ident.Pos(), fmt.Errorf("internal error: failed to assign back to variable '%s' after augmented assignment", varName), "")
		}
		return nil, nil
	}
}

func (i *Interpreter) evalIfStmt(ctx context.Context, ifStmt *ast.IfStmt, env *Environment) (Object, error) {
	condition, err := i.eval(ctx, ifStmt.Cond, env)
	if err != nil {
		return nil, err
	}

	boolCond, ok := condition.(*Boolean)
	if !ok {
		return nil, formatErrorWithContext(i.FileSet, ifStmt.Cond.Pos(),
			fmt.Errorf("condition for if statement must be a boolean, got %s (type: %s)", condition.Inspect(), condition.Type()), "")
	}

	if boolCond.Value {
		// If block creates a new scope
		ifBodyEnv := NewEnvironment(env)
		return i.evalBlockStatement(ctx, ifStmt.Body, ifBodyEnv)
	} else if ifStmt.Else != nil {
		// Else block also creates a new scope if it's a block statement
		// If it's another IfStmt (else if), that IfStmt will handle its own scope.
		switch elseNode := ifStmt.Else.(type) {
		case *ast.BlockStmt:
			elseBodyEnv := NewEnvironment(env)
			return i.evalBlockStatement(ctx, elseNode, elseBodyEnv)
		case *ast.IfStmt: // else if
			return i.eval(ctx, elseNode, env) // The nested if will handle its own new scope creation
		default: // Should not happen with a valid Go AST for if-else
			return i.eval(ctx, ifStmt.Else, env)
		}
	}
	return nil, nil
}

func (i *Interpreter) evalUnaryExpr(ctx context.Context, node *ast.UnaryExpr, env *Environment) (Object, error) {
	operand, err := i.eval(ctx, node.X, env)
	if err != nil {
		return nil, err
	}

	switch node.Op {
	case token.SUB:
		if operand.Type() == INTEGER_OBJ {
			value := operand.(*Integer).Value
			return &Integer{Value: -value}, nil
		}
		return nil, formatErrorWithContext(i.FileSet, node.Pos(), fmt.Errorf("unsupported type for negation: %s", operand.Type()), "")
	case token.NOT:
		switch operand {
		case TRUE:
			return FALSE, nil
		case FALSE:
			return TRUE, nil
		default:
			return nil, formatErrorWithContext(i.FileSet, node.Pos(), fmt.Errorf("unsupported type for logical NOT: %s", operand.Type()), "")
		}
	default:
		return nil, formatErrorWithContext(i.FileSet, node.Pos(), fmt.Errorf("unsupported unary operator: %s", node.Op), "")
	}
}
