package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"go/ast"
	// "go/ast" // Removed duplicate import
	"go/parser" // Ensure go/parser is imported
	"go/token"
	"os"
	"path/filepath" // Added for go-scan
	"strconv"
	"strings"

	goscan "github.com/podhmo/go-scan" // Using top-level go-scan
	"github.com/podhmo/go-scan/scanner" // Import the scanner package
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

	activeFileSet *token.FileSet // FileSet currently active for evaluation context
}

func NewInterpreter() *Interpreter {
	env := NewEnvironment(nil)
	// FileSet will be initialized by the scanner used for the main script.
	// sharedScanner can be nil initially and created on-demand by LoadAndRun if not set by tests.
	i := &Interpreter{
		globalEnv:        env,
		FileSet:          nil, // To be set by the main script parser in LoadAndRun
		sharedScanner:    nil, // Can be preset by tests, or created by LoadAndRun if needed for imports
		importedPackages: make(map[string]struct{}),
		importAliasMap:   make(map[string]string),
		activeFileSet:    nil, // Initialized in LoadAndRun
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

	// For the main script file (.mgo), parse it directly using go/parser.
	// go-scan is used for imported Go packages, not for the minigo script itself.
	i.FileSet = token.NewFileSet() // Initialize a new FileSet for the main script
	mainFileAst, err := parser.ParseFile(i.FileSet, absFilePath, nil, parser.ParseComments)
	if err != nil {
		// We don't have a specific token.Pos here for formatErrorWithContext if ParseFile itself fails.
		// However, parser.ParseFile usually returns an error that includes position info.
		// For simplicity, wrap the error.
		return fmt.Errorf("error parsing main script file %s: %w", filename, err)
	}

	// Ensure the sharedScanner (for imports) is available if needed.
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

	// First pass: Process all TYPE declarations from the main script's AST
	// We need to iterate over mainFileAst.Decls and manually create StructDefinition objects
	// or adapt evalDeclStmt to work with the AST directly without go-scan's PkgInfo for the main file.
	// The existing evalDeclStmt should work if called with *ast.DeclStmt.
	for _, declNode := range mainFileAst.Decls {
		if genDecl, ok := declNode.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
			tempDeclStmt := &ast.DeclStmt{Decl: genDecl}
			_, evalErr := i.eval(ctx, tempDeclStmt, i.globalEnv)
			if evalErr != nil {
				// Pass genDecl.Pos() for better error location
				return formatErrorWithContext(i.FileSet, genDecl.Pos(), evalErr, "Error evaluating type declaration in main script")
			}
		}
	}

	// Second pass: Process function declarations from the main script's AST
	for _, declNode := range mainFileAst.Decls {
		if fnDecl, ok := declNode.(*ast.FuncDecl); ok {
			_, evalErr := i.evalFuncDecl(ctx, fnDecl, i.globalEnv) // evalFuncDecl takes *ast.FuncDecl
			if evalErr != nil {
				return formatErrorWithContext(i.FileSet, fnDecl.Pos(), evalErr, fmt.Sprintf("Error evaluating function declaration %s in main script", fnDecl.Name.Name))
			}
		}
	}

	// Third pass: Process global variable declarations from the main script's AST
	for _, declNode := range mainFileAst.Decls {
		if genDecl, ok := declNode.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			tempDeclStmt := &ast.DeclStmt{Decl: genDecl}
			_, evalErr := i.eval(ctx, tempDeclStmt, i.globalEnv)
			if evalErr != nil {
				return formatErrorWithContext(i.FileSet, genDecl.Pos(), evalErr, "Error evaluating global variable declaration in main script")
			}
		}
	}

	i.activeFileSet = i.FileSet // Initialize activeFileSet with main script's FileSet

	// Ensure the sharedScanner (for imports) is available if needed.
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
			return formatErrorWithContext(i.activeFileSet, token.NoPos, errSharedGs, fmt.Sprintf("Failed to create default shared go-scan scanner (path: %s): %v", scanPathForShared, errSharedGs))
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
			importPathVal, errPath := strconv.Unquote(impSpec.Path.Value)
			if errPath != nil {
				return formatErrorWithContext(i.activeFileSet, impSpec.Path.Pos(), errPath, fmt.Sprintf("Invalid import path: %s", impSpec.Path.Value))
			}

			localName := ""
			if impSpec.Name != nil {
				localName = impSpec.Name.Name
				if localName == "_" {
					// Blank imports are ignored, do not add to importAliasMap
					continue
				}
				if localName == "." {
					return formatErrorWithContext(i.activeFileSet, impSpec.Name.Pos(), errors.New("dot imports are not supported"), "")
				}
			} else {
				localName = filepath.Base(importPathVal)
			}

			if existingPath, ok := i.importAliasMap[localName]; ok && existingPath != importPathVal {
				return formatErrorWithContext(i.activeFileSet, impSpec.Pos(), fmt.Errorf("import alias/name %q already used for %q, cannot reuse for %q", localName, existingPath, importPathVal), "")
			}
			i.importAliasMap[localName] = importPathVal
		}
	}

	// First pass: Process all TYPE declarations from the main script's AST
	// We need to iterate over mainFileAst.Decls and manually create StructDefinition objects
	// or adapt evalDeclStmt to work with the AST directly without go-scan's PkgInfo for the main file.
	// The existing evalDeclStmt should work if called with *ast.DeclStmt.
	for _, declNode := range mainFileAst.Decls {
		if genDecl, ok := declNode.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
			tempDeclStmt := &ast.DeclStmt{Decl: genDecl}
			_, evalErr := i.eval(ctx, tempDeclStmt, i.globalEnv)
			if evalErr != nil {
				// Pass genDecl.Pos() for better error location
				return formatErrorWithContext(i.activeFileSet, genDecl.Pos(), evalErr, "Error evaluating type declaration in main script")
			}
		}
	}

	// Second pass: Process function declarations from the main script's AST
	for _, declNode := range mainFileAst.Decls {
		if fnDecl, ok := declNode.(*ast.FuncDecl); ok {
			_, evalErr := i.evalFuncDecl(ctx, fnDecl, i.globalEnv) // evalFuncDecl takes *ast.FuncDecl
			if evalErr != nil {
				return formatErrorWithContext(i.activeFileSet, fnDecl.Pos(), evalErr, fmt.Sprintf("Error evaluating function declaration %s in main script", fnDecl.Name.Name))
			}
		}
	}

	// Third pass: Process global variable declarations from the main script's AST
	for _, declNode := range mainFileAst.Decls {
		if genDecl, ok := declNode.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			tempDeclStmt := &ast.DeclStmt{Decl: genDecl}
			_, evalErr := i.eval(ctx, tempDeclStmt, i.globalEnv)
			if evalErr != nil {
				return formatErrorWithContext(i.activeFileSet, genDecl.Pos(), evalErr, "Error evaluating global variable declaration in main script")
			}
		}
	}

	// Get the entry function *object* from the global environment
	entryFuncObj, ok := i.globalEnv.Get(entryPoint)
	if !ok {
		return formatErrorWithContext(i.activeFileSet, token.NoPos, fmt.Errorf("entry point function '%s' not found in global environment", entryPoint), "Setup error")
	}

	userEntryFunc, ok := entryFuncObj.(*UserDefinedFunction)
	if !ok {
		return formatErrorWithContext(i.activeFileSet, token.NoPos, fmt.Errorf("entry point '%s' is not a user-defined function (type: %s)", entryPoint, entryFuncObj.Type()), "Setup error")
	}

	fmt.Printf("Executing entry point function: %s\n", entryPoint)
	// For main/entry point, we assume no arguments are passed.
	// Pass the current activeFileSet (which is the main script's FileSet) as the callerFileSet
	result, errApply := i.applyUserDefinedFunction(ctx, userEntryFunc, []Object{}, token.NoPos, i.activeFileSet)
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

func (i *Interpreter) applyUserDefinedFunction(ctx context.Context, fn *UserDefinedFunction, args []Object, callPos token.Pos, callerFileSet *token.FileSet) (Object, error) {
	originalActiveFileSet := i.activeFileSet // Save current active FileSet
	if fn.FileSet != nil {
		i.activeFileSet = fn.FileSet // Set active FileSet to the function's own FileSet
	} else {
		// This case should ideally not happen if all UserDefinedFunctions (local and external) have a FileSet.
		// If it's a local function, fn.FileSet should be i.FileSet (main script's).
		// If it's an external function, fn.FileSet should be i.sharedScanner.Fset().
		// Fallback to caller's FileSet if function's is missing for some reason.
		i.activeFileSet = callerFileSet
		if i.activeFileSet == nil && fn.Name != "" { // Log if still nil for a named function
			fmt.Fprintf(os.Stderr, "Warning: activeFileSet became nil for function %s. This might lead to incorrect error reporting.\n", fn.Name)
		}
	}
	defer func() { i.activeFileSet = originalActiveFileSet }() // Restore active FileSet

	if len(args) != len(fn.Parameters) {
		errMsg := fmt.Sprintf("wrong number of arguments for function %s: expected %d, got %d", fn.Name, len(fn.Parameters), len(args))
		return nil, formatErrorWithContext(i.activeFileSet, callPos, errors.New(errMsg), "Function call error")
	}

	// Argument Type Checking
	if len(fn.Parameters) == len(fn.ParamTypeExprs) { // Ensure ParamTypeExprs is correctly populated
		for idx, paramIdent := range fn.Parameters {
			paramTypeExpr := fn.ParamTypeExprs[idx]
			arg := args[idx]

			// Use fn.Env for resolving parameter types, as types are defined in the function's lexical scope (or globally for packages)
			// Pass `fn` as contextFn for potential qualification of type names.
			// Use i.activeFileSet (which is fn.FileSet here) for error reporting context if type resolution fails.
			expectedObjType, expectedStructDef, errType := i.resolveTypeAstToObjectType(paramTypeExpr, fn.Env, fn, i.activeFileSet)
			if errType != nil {
				// Error resolving the expected type of the parameter
				return nil, formatErrorWithContext(i.activeFileSet, paramTypeExpr.Pos(),
					fmt.Errorf("error resolving type for parameter %s: %w", paramIdent.Name, errType), "Function call error")
			}

			actualObjType := arg.Type()

			if actualObjType != expectedObjType {
				errMsg := fmt.Sprintf("type mismatch for argument %d (%s) of function %s: expected %s, got %s",
					idx+1, paramIdent.Name, fn.Name, expectedObjType, actualObjType)
				return nil, formatErrorWithContext(i.activeFileSet, callPos, errors.New(errMsg), "Function call error") // Use callPos for argument error
			}

			if expectedObjType == STRUCT_INSTANCE_OBJ {
				actualStructInstance, ok := arg.(*StructInstance)
				if !ok { // Should not happen if actualObjType matched STRUCT_INSTANCE_OBJ
					errMsg := fmt.Sprintf("internal error: argument %d (%s) type is STRUCT_INSTANCE_OBJ but not a StructInstance", idx+1, paramIdent.Name)
					return nil, formatErrorWithContext(i.activeFileSet, callPos, errors.New(errMsg), "Internal error")
				}
				if actualStructInstance.Definition != expectedStructDef {
					expectedName := "unknown"
					if expectedStructDef != nil { expectedName = expectedStructDef.Name }
					actualName := "unknown"
					if actualStructInstance.Definition != nil { actualName = actualStructInstance.Definition.Name }

					errMsg := fmt.Sprintf("type mismatch for struct argument %d (%s) of function %s: expected struct type %s, got %s",
						idx+1, paramIdent.Name, fn.Name, expectedName, actualName)
					return nil, formatErrorWithContext(i.activeFileSet, callPos, errors.New(errMsg), "Function call error")
				}
			}
			// Add more checks if needed (e.g., for pointer types, array types, etc., once supported)
		}
	} else {
		// This case indicates an internal inconsistency if ParamTypeExprs isn't populated correctly.
		// For now, skip type checking if this happens, but ideally, it should be an error or ensure it never happens.
		fmt.Fprintf(os.Stderr, "Warning: ParamTypeExprs length mismatch for function %s. Skipping argument type checks.\n", fn.Name)
	}


	funcEnv := NewEnvironment(fn.Env) // Closure: fn.Env is the lexical scope

	// If it's an external function, populate its environment with unqualified names
	// from its own package. This allows calling other functions or using types/constants
	// from the same package with their simple names.
	if fn.IsExternal && fn.PackageAlias != "" && fn.PackagePath != "" {
		// fn.Env for an external function is the environment where it was defined,
		// which is the global environment where "PkgAlias.SymbolName" entries are stored.
		// We iterate over all entries in that environment (and its outers, though for global fn.Env, outer is nil).
		allGlobalSymbols := fn.Env.GetAllEntries() // Get all symbols from the function's definition environment.

		prefix := fn.PackageAlias + "."
		for qualifiedName, obj := range allGlobalSymbols {
			if strings.HasPrefix(qualifiedName, prefix) {
				unqualifiedName := strings.TrimPrefix(qualifiedName, prefix)
				if unqualifiedName != "" { // Ensure it's not just "alias."
					// Define in funcEnv without the prefix.
					// This effectively makes package-local symbols directly available.
					funcEnv.Define(unqualifiedName, obj)
				}
			}
		}
	}

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
				result, err = i.evalBlockStatement(ctx, fnDecl.Body, env) // evalBlockStatement uses i.activeFileSet internally for errors
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
		return evalIdentifier(n, env, i.activeFileSet) // Pass activeFileSet

	case *ast.BasicLit:
		switch n.Kind {
		case token.STRING:
			return &String{Value: n.Value[1 : len(n.Value)-1]}, nil
		case token.INT:
			val, err := parseInt64(n.Value)
			if err != nil {
				return nil, formatErrorWithContext(i.activeFileSet, n.Pos(), err, fmt.Sprintf("Could not parse integer literal '%s'", n.Value))
			}
			return &Integer{Value: val}, nil
		default:
			return nil, formatErrorWithContext(i.activeFileSet, n.Pos(), fmt.Errorf("unsupported literal type: %s", n.Kind), fmt.Sprintf("Unsupported literal value: %s", n.Value))
		}

	case *ast.DeclStmt:
		return i.evalDeclStmt(ctx, n, env) // This will use i.activeFileSet for errors inside

	case *ast.BinaryExpr:
		return i.evalBinaryExpr(ctx, n, env) // Uses i.activeFileSet via helper functions

	case *ast.UnaryExpr:
		return i.evalUnaryExpr(ctx, n, env) // Uses i.activeFileSet

	case *ast.ParenExpr:
		return i.eval(ctx, n.X, env)

	case *ast.IfStmt:
		return i.evalIfStmt(ctx, n, env) // Uses i.activeFileSet

	case *ast.AssignStmt:
		return i.evalAssignStmt(ctx, n, env) // Uses i.activeFileSet

	case *ast.CallExpr:
		return i.evalCallExpr(ctx, n, env) // Uses i.activeFileSet

	case *ast.SelectorExpr:
		return i.evalSelectorExpr(ctx, n, env) // Uses i.activeFileSet

	case *ast.ReturnStmt:
		return i.evalReturnStmt(ctx, n, env) // Uses i.activeFileSet

	case *ast.FuncDecl:
		return i.evalFuncDecl(ctx, n, env) // Uses i.activeFileSet

	case *ast.FuncLit:
		return i.evalFuncLit(ctx, n, env) // Uses i.activeFileSet

	case *ast.ForStmt:
		return i.evalForStmt(ctx, n, env) // Uses i.activeFileSet

	case *ast.BranchStmt:
		return i.evalBranchStmt(ctx, n, env) // Uses i.activeFileSet

	case *ast.LabeledStmt:
		return i.eval(ctx, n.Stmt, env)

	case *ast.CompositeLit:
		return i.evalCompositeLit(ctx, n, env) // Uses i.activeFileSet

	default:
		return nil, formatErrorWithContext(i.activeFileSet, n.Pos(), fmt.Errorf("unsupported AST node type: %T", n), fmt.Sprintf("Unsupported AST node value: %+v", n))
	}
}

func (i *Interpreter) evalCompositeLit(ctx context.Context, lit *ast.CompositeLit, env *Environment) (Object, error) {
	var structDef *StructDefinition

	switch typeNode := lit.Type.(type) {
	case *ast.Ident:
		typeNameStr := typeNode.Name
		obj, found := env.Get(typeNameStr)
		if !found {
			// TODO: Consider context for unqualified type names within external functions.
			// For now, if not found directly, it's an error.
			return nil, formatErrorWithContext(i.activeFileSet, typeNode.Pos(), fmt.Errorf("undefined type '%s' used in composite literal", typeNameStr), "Struct instantiation error")
		}
		sDef, ok := obj.(*StructDefinition)
		if !ok {
			return nil, formatErrorWithContext(i.activeFileSet, typeNode.Pos(), fmt.Errorf("type '%s' is not a struct type, but %s", typeNameStr, obj.Type()), "Struct instantiation error")
		}
		structDef = sDef

	case *ast.SelectorExpr: // Handle pkg.Type
		pkgIdent, ok := typeNode.X.(*ast.Ident)
		if !ok {
			return nil, formatErrorWithContext(i.activeFileSet, typeNode.X.Pos(), fmt.Errorf("package selector X in composite literal type must be an identifier, got %T", typeNode.X), "Struct instantiation error")
		}
		pkgName := pkgIdent.Name
		structName := typeNode.Sel.Name
		qualifiedName := pkgName + "." + structName

		obj, found := env.Get(qualifiedName)
		if !found {
			// Attempt to load the package if the qualified type name is not found.
			// The environment `env` passed here is the current evaluation environment.
			// loadPackageIfNeeded expects the global environment to register symbols.
			// We assume that for struct literals, `env` will be or will chain to i.globalEnv
			// where package symbols are expected. For external struct literals, this is true.
			// For local struct literals within functions, this also holds due to lexical scoping.
			_, errLoad := i.loadPackageIfNeeded(ctx, pkgName, i.globalEnv, typeNode.X.Pos()) // Pass i.globalEnv
			if errLoad != nil {
				// Error during package loading attempt.
				// The error from loadPackageIfNeeded is already formatted.
				// We can add context that this was for a composite literal.
				return nil, formatErrorWithContext(i.activeFileSet, typeNode.Pos(), errLoad, fmt.Sprintf("Error loading package '%s' for struct literal type '%s'", pkgName, qualifiedName))
			}
			// Try getting the struct definition again after attempting to load the package.
			obj, found = env.Get(qualifiedName) // Search in the original env
			if !found {
				return nil, formatErrorWithContext(i.activeFileSet, typeNode.Pos(), fmt.Errorf("undefined type '%s' used in composite literal even after attempting to load package '%s'", qualifiedName, pkgName), "Struct instantiation error")
			}
		}
		sDef, ok := obj.(*StructDefinition)
		if !ok {
			return nil, formatErrorWithContext(i.activeFileSet, typeNode.Pos(), fmt.Errorf("type '%s' is not a struct type, but %s", qualifiedName, obj.Type()), "Struct instantiation error")
		}
		structDef = sDef

	default:
		return nil, formatErrorWithContext(i.activeFileSet, lit.Type.Pos(), fmt.Errorf("expected identifier or selector for composite literal type, got %T", lit.Type), "Struct instantiation error")
	}

	instance := &StructInstance{
		Definition:     structDef,
		FieldValues:    make(map[string]Object),
		EmbeddedValues: make(map[string]*StructInstance),
	}

	if len(lit.Elts) == 0 && len(structDef.Fields) > 0 {
		// Handling of T{} is simplified for now.
	}

	isKeyValueForm := false
	if len(lit.Elts) > 0 {
		_, isKeyValueForm = lit.Elts[0].(*ast.KeyValueExpr)
	}

	if isKeyValueForm {
		for _, elt := range lit.Elts {
			kvExpr, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				return nil, formatErrorWithContext(i.activeFileSet, elt.Pos(), fmt.Errorf("mixture of keyed and non-keyed fields in struct literal for '%s'", structDef.Name), "Struct instantiation error")
			}
			keyIdent, ok := kvExpr.Key.(*ast.Ident)
			if !ok {
				return nil, formatErrorWithContext(i.activeFileSet, kvExpr.Key.Pos(), fmt.Errorf("struct field key must be an identifier, got %T for struct '%s'", kvExpr.Key, structDef.Name), "Struct instantiation error")
			}
			fieldName := keyIdent.Name
			valueExpr := kvExpr.Value

			if _, isDirectField := structDef.Fields[fieldName]; isDirectField {
				valObj, err := i.eval(ctx, valueExpr, env)
				if err != nil { return nil, err }
				instance.FieldValues[fieldName] = valObj
				continue
			}
			// ... (simplified embedded/promoted field handling for brevity, assumes direct fields primarily) ...
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
				if err != nil { return nil, err }
				embInstanceVal, ok := valObj.(*StructInstance)
				if !ok || embInstanceVal.Definition.Name != targetEmbeddedDefForExplicitInit.Name {
					return nil, formatErrorWithContext(i.activeFileSet, kvExpr.Value.Pos(),
						fmt.Errorf("value for embedded struct '%s' is not a compatible struct instance (expected '%s', got '%s')",
							fieldName, targetEmbeddedDefForExplicitInit.Name, valObj.Type()), "Struct instantiation error")
				}
				instance.EmbeddedValues[fieldName] = embInstanceVal
				continue
			}
			// Promoted field logic (simplified)
			var owningEmbDef *StructDefinition
			for _, embDef := range structDef.EmbeddedDefs {
				if _, isPromoted := embDef.Fields[fieldName]; isPromoted {
					owningEmbDef = embDef; break
				}
			}
			if owningEmbDef != nil {
                 // ... (logic to set field in embedded instance) ...
				 valObj, err := i.eval(ctx, valueExpr, env)
				 if err != nil { return nil, err }
				 embInstance, ok := instance.EmbeddedValues[owningEmbDef.Name]
				 if !ok {
					 embInstance = &StructInstance{Definition: owningEmbDef, FieldValues: make(map[string]Object), EmbeddedValues: make(map[string]*StructInstance)}
					 instance.EmbeddedValues[owningEmbDef.Name] = embInstance
				 }
				 embInstance.FieldValues[fieldName] = valObj
				 continue
			}
			return nil, formatErrorWithContext(i.activeFileSet, keyIdent.Pos(), fmt.Errorf("unknown field '%s' in struct literal of type '%s'", fieldName, structDef.Name), "Struct instantiation error")
		}
	} else {
		if len(lit.Elts) > 0 && len(structDef.Fields) > 0 {
			return nil, formatErrorWithContext(i.activeFileSet, lit.Pos(), fmt.Errorf("ordered (non-keyed) struct literal values are not supported yet for struct '%s'", structDef.Name), "Struct instantiation error")
		}
	}
	return instance, nil
}


func (i *Interpreter) evalBranchStmt(ctx context.Context, stmt *ast.BranchStmt, env *Environment) (Object, error) {
	if stmt.Label != nil {
		return nil, formatErrorWithContext(i.activeFileSet, stmt.Pos(), fmt.Errorf("labeled break/continue not supported"), "")
	}

	switch stmt.Tok {
	case token.BREAK:
		return BREAK, nil
	case token.CONTINUE:
		return CONTINUE, nil
	default:
		return nil, formatErrorWithContext(i.activeFileSet, stmt.Pos(), fmt.Errorf("unsupported branch statement: %s", stmt.Tok), "")
	}
}

func (i *Interpreter) evalForStmt(ctx context.Context, stmt *ast.ForStmt, env *Environment) (Object, error) {
	loopEnv := NewEnvironment(env)
	if stmt.Init != nil {
		if _, err := i.eval(ctx, stmt.Init, loopEnv); err != nil { return nil, err }
	}
	for {
		if stmt.Cond != nil {
			condition, err := i.eval(ctx, stmt.Cond, loopEnv)
			if err != nil { return nil, err }
			boolCond, ok := condition.(*Boolean)
			if !ok {
				return nil, formatErrorWithContext(i.activeFileSet, stmt.Cond.Pos(),
					fmt.Errorf("condition for for statement must be a boolean, got %s (type: %s)", condition.Inspect(), condition.Type()), "")
			}
			if !boolCond.Value { break }
		}
		bodyResult, err := i.evalBlockStatement(ctx, stmt.Body, loopEnv)
		if err != nil { return nil, err }

		var broke bool
		switch res := bodyResult.(type) {
		case *ReturnValue: return res, nil
		case *Error: return res, nil
		case *BreakStatement: broke = true
		case *ContinueStatement:
			if stmt.Post != nil {
				if _, postErr := i.eval(ctx, stmt.Post, loopEnv); postErr != nil { return nil, postErr }
			}
			continue
		}
		if broke { break }
		if stmt.Post != nil {
			if _, err := i.eval(ctx, stmt.Post, loopEnv); err != nil { return nil, err }
		}
	}
	return NULL, nil
}

func (i *Interpreter) evalSelectorExpr(ctx context.Context, node *ast.SelectorExpr, env *Environment) (Object, error) {
	xObj, err := i.eval(ctx, node.X, env)
	fieldName := node.Sel.Name

	if err != nil {
		_, isIdent := node.X.(*ast.Ident)
		if isIdent && err != nil && strings.Contains(err.Error(), "identifier not found") {
			goto handlePackageAccess
		}
		return nil, err
	}

	if structInstance, ok := xObj.(*StructInstance); ok {
		if val, found := structInstance.FieldValues[fieldName]; found { return val, nil }
		if _, isDirectField := structInstance.Definition.Fields[fieldName]; isDirectField { return NULL, nil }

		// Use activeFileSet for findFieldInEmbedded if it's from current context,
		// or structInstance.Definition.FileSet if that's more appropriate for the definition.
		// For now, findFieldInEmbedded takes the FileSet from the selector expression's context.
		foundValue, _, _, embErr := findFieldInEmbedded(structInstance, fieldName, i.activeFileSet, node.Sel.Pos())
		if embErr != nil { return nil, embErr }
		if foundValue != nil { return foundValue, nil }
		return nil, formatErrorWithContext(i.activeFileSet, node.Sel.Pos(), fmt.Errorf("type %s has no field %s", structInstance.Definition.Name, fieldName), "Field access error")
	}

handlePackageAccess:
	if identX, ok := node.X.(*ast.Ident); ok {
		localPkgName := identX.Name
		qualifiedNameInEnv := localPkgName + "." + fieldName

		if val, found := env.Get(qualifiedNameInEnv); found { return val, nil }

		importPath, knownAlias := i.importAliasMap[localPkgName]
		if !knownAlias {
			originalErrorMsg := "undefined"
			if err != nil { originalErrorMsg = err.Error() } // err here is from the outer scope, potentially from xObj, err := i.eval
			return nil, formatErrorWithContext(i.FileSet, identX.Pos(), fmt.Errorf("%s: %s (not a struct instance and not a known package alias/name)", originalErrorMsg, localPkgName), "Selector error")
		}

		// Declare variables here so they are in scope for later use.
		var loadedPkgInfo *scanner.PackageInfo
		var loadErr error
		var val Object
		var found bool

		if _, alreadyImported := i.importedPackages[importPath]; !alreadyImported {
			// Package not yet imported, call loadPackageIfNeeded to scan and define symbols.
			loadedPkgInfo, loadErr = i.loadPackageIfNeeded(ctx, localPkgName, env, identX.Pos()) // Assign using =
			if loadErr != nil {
				// loadErr should be already formatted by loadPackageIfNeeded
				return nil, loadErr
			}
			// loadedPkgInfo is captured but not directly used here further; its purpose was loading symbols into env.
			_ = loadedPkgInfo // Explicitly ignore if not used further, to satisfy the compiler if checks change.
		}

		// After ensuring the package is loaded (either now or previously, or if there was an error),
		// try getting the symbol again from the environment.
		val, found = env.Get(qualifiedNameInEnv) // Assign using =
		if found {
			return val, nil
		}

		// If still not found, then it's truly undefined.
		// Note: importPath is resolved inside loadPackageIfNeeded, so we use localPkgName for error msg here if path wasn't found.
		// Re-fetch importPath for error message as it might not have been set if knownAlias was false.
		resolvedImportPath, _ := i.importAliasMap[localPkgName]
		return nil, formatErrorWithContext(i.activeFileSet, node.Sel.Pos(), fmt.Errorf("undefined: %s.%s (package %s, path %s, was loaded or loading attempted)", localPkgName, fieldName, localPkgName, resolvedImportPath), "Selector error")
	}

	if xObj != nil { // This check should be xObj from the initial eval, not a new one.
		return nil, formatErrorWithContext(i.activeFileSet, node.X.Pos(), fmt.Errorf("selector base must be a struct instance or package identifier, got %s", xObj.Type()), "Unsupported selector expression")
	}
	return nil, formatErrorWithContext(i.activeFileSet, node.Pos(), errors.New("internal error in selector evaluation"), "")
}

// resolveTypeAstToObjectType resolves an AST type expression to an interpreter ObjectType and optionally a StructDefinition.
// typeExpr: The AST node representing the type.
// resolutionEnv: The environment to use for looking up type names (typically fn.Env for function parameters).
// contextFn: Optional. If resolving a type for a parameter of this function, it's used to qualify unqualified type names (e.g. `Point` becomes `pkg.Point`).
// activeFset: FileSet for error reporting.
func (i *Interpreter) resolveTypeAstToObjectType(typeExpr ast.Expr, resolutionEnv *Environment, contextFn *UserDefinedFunction, activeFset *token.FileSet) (ObjectType, *StructDefinition, error) {
	switch te := typeExpr.(type) {
	case *ast.Ident:
		typeName := te.Name
		// Check for built-in types first
		switch typeName {
		case "int":
			return INTEGER_OBJ, nil, nil
		case "string":
			return STRING_OBJ, nil, nil
		case "bool":
			return BOOLEAN_OBJ, nil, nil
		}

		// If it's a parameter of an external function, the type name might be unqualified (e.g., "Point" instead of "pkg.Point")
		// In this case, resolutionEnv is fn.Env (global), and we should try qualifying it.
		if contextFn != nil && contextFn.IsExternal && contextFn.PackageAlias != "" {
			qualifiedTypeName := contextFn.PackageAlias + "." + typeName
			// External function types are defined in the global environment (contextFn.Env, which is resolutionEnv here)
			obj, found := resolutionEnv.Get(qualifiedTypeName)
			if found {
				if structDef, ok := obj.(*StructDefinition); ok {
					return STRUCT_INSTANCE_OBJ, structDef, nil
				}
				// Found something but not a struct def (e.g. a function with the same name as a type)
				return "", nil, formatErrorWithContext(activeFset, te.Pos(), fmt.Errorf("type '%s' (resolved as '%s') is not a struct definition, but %s", typeName, qualifiedTypeName, obj.Type()), "")
			}
			// If not found qualified, it might be an error, or it could be a global built-in type not yet handled above.
			// For now, let's fall through to the general lookup, which might be an error.
		}

		// General lookup for identifier type names (e.g., local struct, or if not external context)
		obj, found := resolutionEnv.Get(typeName)
		if !found {
			return "", nil, formatErrorWithContext(activeFset, te.Pos(), fmt.Errorf("undefined type: %s", typeName), "")
		}
		if structDef, ok := obj.(*StructDefinition); ok {
			return STRUCT_INSTANCE_OBJ, structDef, nil
		}
		return "", nil, formatErrorWithContext(activeFset, te.Pos(), fmt.Errorf("type '%s' is not a struct definition, but %s", typeName, obj.Type()), "")

	case *ast.SelectorExpr: // e.g., pkg.TypeName
		pkgIdent, ok := te.X.(*ast.Ident)
		if !ok {
			return "", nil, formatErrorWithContext(activeFset, te.X.Pos(), fmt.Errorf("package selector in type must be an identifier, got %T", te.X), "Type resolution error")
		}
		pkgName := pkgIdent.Name
		typeName := te.Sel.Name
		qualifiedName := pkgName + "." + typeName

		// Ensure package is loaded (this might be redundant if already handled, but good for safety)
		// This call to loadPackageIfNeeded uses i.globalEnv because type definitions from imports are stored globally.
		// The `env` passed to resolveTypeAstToObjectType might be a more local function environment.
		if _, err := i.loadPackageIfNeeded(context.TODO(), pkgName, i.globalEnv, pkgIdent.Pos()); err != nil {
			// Don't return error if already loaded (err might be nil, and pkgInfo nil)
			// The error from loadPackageIfNeeded is already formatted.
			// We only care about fatal errors here preventing type lookup.
			// If loadPackageIfNeeded returns (nil, nil), it means it was already processed or found.
			// Check if error is non-nil and then if it's a "real" error.
			// This logic might need refinement based on loadPackageIfNeeded's exact return for "already loaded".
			// For now, assume any error from loadPackageIfNeeded is problematic for type resolution here.
			// However, loadPackageIfNeeded itself checks `importedPackages`.
			// A simpler approach: try Get, if not found, then try load.
		}

		// For qualified names (pkg.Type), the definition should be in the global environment.
		// resolutionEnv might be a local function scope, so directly use i.globalEnv.
		obj, found := i.globalEnv.Get(qualifiedName)
		if !found {
			// Attempt to load the package if not found after checking env and globalEnv.
			// This is important if the type is from a package not explicitly loaded via a selector expression value yet.
			_, loadErr := i.loadPackageIfNeeded(context.TODO(), pkgName, i.globalEnv, pkgIdent.Pos())
			if loadErr != nil {
				// If loading fails, the type cannot be resolved.
				return "", nil, formatErrorWithContext(activeFset, te.Pos(), fmt.Errorf("failed to load package '%s' for type resolution of '%s': %w", pkgName, qualifiedName, loadErr), "")
			}
			// Try fetching again after loading attempt
			obj, found = i.globalEnv.Get(qualifiedName)
			if !found {
				return "", nil, formatErrorWithContext(activeFset, te.Pos(), fmt.Errorf("undefined type: %s (after attempting package load)", qualifiedName), "")
			}
		}


		if structDef, ok := obj.(*StructDefinition); ok {
			return STRUCT_INSTANCE_OBJ, structDef, nil
		}
		return "", nil, formatErrorWithContext(activeFset, te.Pos(), fmt.Errorf("qualified type '%s' is not a struct definition, but %s", qualifiedName, obj.Type()), "")
	// TODO: Handle *ast.StarExpr for pointers, *ast.ArrayType, *ast.MapType, *ast.InterfaceType etc.
	default:
		return "", nil, formatErrorWithContext(activeFset, typeExpr.Pos(), fmt.Errorf("unsupported AST node type for type resolution: %T", typeExpr), "")
	}
}

// loadPackageIfNeeded handles the logic for loading symbols from an imported package
// if it hasn't been loaded yet. It populates the provided 'env' (expected to be global)
// with the package's exported symbols, qualified by pkgAlias.
func (i *Interpreter) loadPackageIfNeeded(ctx context.Context, pkgAlias string, env *Environment, errorPos token.Pos) (*scanner.PackageInfo, error) {
	// Ensure sharedScanner is available
	if i.sharedScanner == nil {
		return nil, formatErrorWithContext(i.activeFileSet, errorPos, errors.New("shared go-scan scanner (for imports) not initialized in interpreter"), "Internal error")
	}

	importPath, knownAlias := i.importAliasMap[pkgAlias]
	if !knownAlias {
		// This case should ideally be caught before calling loadPackageIfNeeded,
		// e.g., in evalSelectorExpr, if pkgAlias is not in importAliasMap.
		// However, if called directly, this provides a safeguard.
		return nil, formatErrorWithContext(i.activeFileSet, errorPos, fmt.Errorf("package alias %s not found in import map", pkgAlias), "Import error")
	}

	// Check if already processed (e.g. symbols defined)
	if _, alreadyImported := i.importedPackages[importPath]; alreadyImported {
		// If it was already imported, we expect symbols to be in 'env'.
		// We can return nil, nil to indicate no *new* package info was loaded *this time*,
		// but also no error. The caller (evalSelectorExpr) will then try to find the symbol.
		// Alternatively, we could try to find the PackageInfo from a cache if we stored it,
		// but for now, this behavior is simple.
		return nil, nil // Indicate already processed, no error.
	}

	// Store original active FileSet and set it to the shared scanner's for the duration of this import.
	originalActiveFileSet := i.activeFileSet
	i.activeFileSet = i.sharedScanner.Fset()
	defer func() { i.activeFileSet = originalActiveFileSet }() // Restore

	importPkgInfo, errImport := i.sharedScanner.ScanPackageByImport(ctx, importPath)
	if errImport != nil {
		return nil, formatErrorWithContext(i.sharedScanner.Fset(), errorPos, fmt.Errorf("package %q (aliased as %q) not found or failed to scan: %w", importPath, pkgAlias, errImport), "Import error")
	}

	if importPkgInfo == nil {
		// Should be covered by errImport != nil, but as a safeguard.
		return nil, formatErrorWithContext(i.sharedScanner.Fset(), errorPos, fmt.Errorf("ScanPackageByImport returned nil for %q (%s) without error", importPath, pkgAlias), "Internal error")
	}

	// Populate the global environment with symbols from the imported package.
	// Constants
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
					fmt.Fprintf(os.Stderr, "Warning: Could not parse external const integer %s.%s (value: %s): %v\n", pkgAlias, c.Name, c.Value, errParse)
				}
			case "string":
				unquotedVal, errParse := strconv.Unquote(c.Value)
				if errParse == nil {
					constObj = &String{Value: unquotedVal}
				} else {
					fmt.Fprintf(os.Stderr, "Warning: Could not unquote external const string %s.%s (value: %s): %v\n", pkgAlias, c.Name, c.Value, errParse)
				}
			case "bool":
				switch c.Value {
				case "true":
					constObj = TRUE
				case "false":
					constObj = FALSE
				default:
					fmt.Fprintf(os.Stderr, "Warning: Could not parse external const bool %s.%s (value: %s)\n", pkgAlias, c.Name, c.Value)
				}
			default:
				fmt.Fprintf(os.Stderr, "Warning: Unsupported external const type %s for %s.%s\n", c.Type.Name, pkgAlias, c.Name)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Warning: External const %s.%s has no type info\n", pkgAlias, c.Name)
		}
		if constObj != nil {
			env.Define(pkgAlias+"."+c.Name, constObj)
		}
	}

	// Functions
	for _, fInfo := range importPkgInfo.Functions {
		if !ast.IsExported(fInfo.Name) || fInfo.AstDecl == nil {
			continue
		}
		params := []*ast.Ident{}
		paramTypeExprs := []ast.Expr{}
		if fInfo.AstDecl.Type.Params != nil {
			for _, field := range fInfo.AstDecl.Type.Params.List {
				params = append(params, field.Names...)
				for range field.Names {
					paramTypeExprs = append(paramTypeExprs, field.Type)
				}
			}
		}
		// Use sharedScanner's FileSet for external functions
		env.Define(pkgAlias+"."+fInfo.Name, &UserDefinedFunction{
			Name:           fInfo.Name,
			Parameters:     params,
			ParamTypeExprs: paramTypeExprs,
			Body:           fInfo.AstDecl.Body,
			Env:            env, // Lexical scope is the global env where "Pkg.Func" is defined
			FileSet:        i.sharedScanner.Fset(),
			IsExternal:     true,
			PackagePath:    importPath,
			PackageAlias:   pkgAlias,
		})
	}

	// Types/Structs
	for _, typeInfo := range importPkgInfo.Types {
		if !ast.IsExported(typeInfo.Name) || typeInfo.Node == nil {
			continue
		}
		typeSpec, ok := typeInfo.Node.(*ast.TypeSpec)
		if !ok {
			continue
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			// Not a struct, could be an alias or interface, skip for now if only structs are handled.
			// Or, if you want to represent other types, add logic here.
			fmt.Fprintf(os.Stderr, "Info: Skipping non-struct type %s.%s in import.\n", pkgAlias, typeInfo.Name)
			continue
		}

		directFields := make(map[string]string) // FieldName -> FieldTypeName (simple string for now)
		var embeddedDefs []*StructDefinition     // List of *StructDefinition for embedded types
		var fieldOrder []string                  // To maintain original field order

		if structType.Fields != nil && structType.Fields.List != nil {
			for _, field := range structType.Fields.List {
				var fieldTypeNameStr string // To store the string representation of the field's type

				// Determine field type name string
				switch typeExpr := field.Type.(type) {
				case *ast.Ident:
					fieldTypeNameStr = typeExpr.Name // e.g., "int", "MyStruct"
				case *ast.SelectorExpr: // e.g., "pkg.OtherStruct"
					xIdent, okX := typeExpr.X.(*ast.Ident)
					if !okX {
						// This should not happen for valid Go code if SelectorExpr.X is not an Ident
						fmt.Fprintf(os.Stderr, "Warning: Skipping field with complex selector type X in %s.%s: %T\n", pkgAlias, typeSpec.Name.Name, typeExpr.X)
						continue
					}
					fieldTypeNameStr = xIdent.Name + "." + typeExpr.Sel.Name // e.g., "anotherpkg.MyType"
				default:
					fmt.Fprintf(os.Stderr, "Warning: Skipping field with unsupported type expr %T in %s.%s\n", field.Type, pkgAlias, typeSpec.Name.Name)
					continue
				}

				if len(field.Names) == 0 { // Embedded field
					// For an embedded field, fieldTypeNameStr is the name of the type being embedded.
					// It could be a simple name (e.g., "MyEmbeddedStruct") or qualified (e.g., "otherpkg.AnotherStruct").
					fieldOrder = append(fieldOrder, fieldTypeNameStr) // Use the type name itself for order

					// Resolve the embedded struct definition.
					// If it's a simple name, assume it's from the *same* package.
					// If it's qualified, it's from another package.
					var qualifiedEmbTypeName string
					if strings.Contains(fieldTypeNameStr, ".") {
						qualifiedEmbTypeName = fieldTypeNameStr // Already qualified, e.g. "pkg.Type"
					} else {
						// Not qualified, assume it's from the *current* package being imported (importPkgInfo.Name)
						// This is tricky. If MyStruct embeds Point, and Point is in the same package,
						// then fieldTypeNameStr is "Point". We need to look up "pkgAlias.Point" in `env`.
						qualifiedEmbTypeName = pkgAlias + "." + fieldTypeNameStr
					}

					embObj, found := env.Get(qualifiedEmbTypeName)
					if found {
						if ed, okEd := embObj.(*StructDefinition); okEd {
							embeddedDefs = append(embeddedDefs, ed)
						} else {
							fmt.Fprintf(os.Stderr, "Warning: Embedded type %s in %s.%s is not a struct definition (%s)\n", fieldTypeNameStr, pkgAlias, typeSpec.Name.Name, embObj.Type())
						}
					} else {
						// This can happen if the embedded type is defined *later* in the same package,
						// or if it's from a package not yet fully processed.
						// For now, we'll just warn. A multi-pass approach might be needed for complex cases.
						fmt.Fprintf(os.Stderr, "Warning: Could not find definition for embedded type %s (%s) while importing %s.%s\n", fieldTypeNameStr, qualifiedEmbTypeName, pkgAlias, typeSpec.Name.Name)
					}
				} else { // Named field
					for _, nameIdent := range field.Names {
						directFields[nameIdent.Name] = fieldTypeNameStr // Store the type name string
						fieldOrder = append(fieldOrder, nameIdent.Name)
					}
				}
			}
		}

		structDef := &StructDefinition{
			Name:         typeSpec.Name.Name,
			Fields:       directFields,
			EmbeddedDefs: embeddedDefs,
			FieldOrder:   fieldOrder,
			FileSet:      i.sharedScanner.Fset(), // FileSet from the imported package's scanner
			IsExternal:   true,
			PackagePath:  importPath,
			// PackageAlias is not stored directly in StructDefinition, as it's known by the qualified name.
		}
		qualifiedStructName := pkgAlias + "." + structDef.Name
		env.Define(qualifiedStructName, structDef)
	}

	// Mark this package path as imported to avoid reprocessing.
	i.importedPackages[importPath] = struct{}{}

	return importPkgInfo, nil
}

// findFieldInEmbedded uses fset passed to it for errors
func findFieldInEmbedded(instance *StructInstance, fieldName string, fset *token.FileSet, selPos token.Pos) (foundValue Object, found bool, foundIn string, err error) {
	// ... (no change to i.activeFileSet usage here as fset is explicit)
	var overallFoundValue Object
	var overallFoundInDefinitionName string
	var numFoundPaths int = 0

	for _, embDef := range instance.Definition.EmbeddedDefs {
		embInstance, embInstanceExists := instance.EmbeddedValues[embDef.Name]
		if !embInstanceExists {
			if _, isFieldInEmbDef := embDef.Fields[fieldName]; isFieldInEmbDef {
				if numFoundPaths > 0 && overallFoundInDefinitionName != embDef.Name {
					return nil, false, "", formatErrorWithContext(fset, selPos,
						fmt.Errorf("ambiguous selector %s (found in %s and as uninitialized field in %s)", fieldName, overallFoundInDefinitionName, embDef.Name), "")
				}
				overallFoundValue = NULL
				overallFoundInDefinitionName = embDef.Name
				numFoundPaths++
			}
			continue
		}
		if val, isSet := embInstance.FieldValues[fieldName]; isSet {
			if numFoundPaths > 0 && overallFoundInDefinitionName != embDef.Name {
				return nil, false, "", formatErrorWithContext(fset, selPos,
					fmt.Errorf("ambiguous selector %s (found in %s and as set field in %s)", fieldName, overallFoundInDefinitionName, embDef.Name), "")
			}
			overallFoundValue = val
			overallFoundInDefinitionName = embDef.Name
			numFoundPaths++
			continue
		}
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

	paramTypeExprs := []ast.Expr{}
	if fd.Type.Params != nil {
		for _, field := range fd.Type.Params.List {
			// Each field can declare multiple names for the same type (e.g., i, j int)
			// We need one type expression per parameter identifier.
			for range field.Names {
				paramTypeExprs = append(paramTypeExprs, field.Type)
			}
		}
	}

	function := &UserDefinedFunction{
		Name:           fd.Name.Name,
		Parameters:     params,
		ParamTypeExprs: paramTypeExprs,
		Body:           fd.Body,
		Env:            env,
		FileSet:        i.FileSet, // Set the FileSet
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

	paramTypeExprs := []ast.Expr{}
	if fl.Type.Params != nil {
		for _, field := range fl.Type.Params.List {
			for range field.Names {
				paramTypeExprs = append(paramTypeExprs, field.Type)
			}
		}
	}

	return &UserDefinedFunction{
		Name:           "",
		Parameters:     params,
		ParamTypeExprs: paramTypeExprs,
		Body:           fl.Body,
		Env:            env,
		FileSet:        i.FileSet, // Set the FileSet
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
							if typeFound {
								if structDef, ok := typeObj.(*StructDefinition); ok {
									// Create an empty instance for the struct type (zero-value representation)
									zeroVal = &StructInstance{
										Definition:     structDef,
										FieldValues:    make(map[string]Object),
										EmbeddedValues: make(map[string]*StructInstance),
									}
								} else {
									// Not a struct definition, unsupported type for zero value
									return nil, formatErrorWithContext(i.FileSet, T.Pos(), fmt.Errorf("unsupported type '%s' for uninitialized variable '%s' (not a known struct)", T.Name, varName), "")
								}
							} else {
								// Type not found in environment
								return nil, formatErrorWithContext(i.FileSet, T.Pos(), fmt.Errorf("undefined type '%s' for uninitialized variable '%s'", T.Name, varName), "")
							}
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
		return i.applyUserDefinedFunction(ctx, fn, args, node.Fun.Pos(), i.activeFileSet)
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
			return nil, formatErrorWithContext(i.activeFileSet, node.Pos(), fmt.Errorf("unsupported type for logical NOT: %s", operand.Type()), "")
		}
	case token.AND: // Address-of operator &
		// In this interpreter, taking the address of something might not mean a real memory address.
		// If it's a struct instance (from an identifier or a composite literal),
		// we can return it as is, signifying it's "addressable." This is a simplification.
		switch xNode := node.X.(type) {
		case *ast.Ident:
			// Operand is already evaluated when evalUnaryExpr is called.
			// So, `operand` is the result of evaluating xNode (the identifier).
			if _, isStruct := operand.(*StructInstance); isStruct {
				return operand, nil // Return the struct instance itself.
			}
			return nil, formatErrorWithContext(i.activeFileSet, xNode.Pos(), fmt.Errorf("cannot take address of identifier '%s' (type %s), not a struct instance", xNode.Name, operand.Type()), "")
		case *ast.CompositeLit:
			// If & is applied to a composite literal, e.g., &Point{X:1}.
			// The `operand` variable here is the result of i.eval(ctx, node.X, env),
			// which means the composite literal (node.X) has already been evaluated.
			// So, `operand` should be the StructInstance.
			if _, isStruct := operand.(*StructInstance); isStruct {
				return operand, nil // Return the struct instance from the composite literal.
			}
			// This case should ideally not be reached if composite lit eval fails or returns non-struct.
			return nil, formatErrorWithContext(i.activeFileSet, xNode.Pos(), fmt.Errorf("operator & on composite literal did not yield a struct instance, got %s", operand.Type()), "")
		default:
			// Other cases like &MyFunctionCall() or &someSelector.field might need more complex handling
			// if they are to be supported. For now, restrict to identifiers and composite literals.
			return nil, formatErrorWithContext(i.activeFileSet, node.Pos(), fmt.Errorf("operator & only supported on identifiers or composite literals for now, got %T", node.X), "")
		}
	default:
		return nil, formatErrorWithContext(i.activeFileSet, node.Pos(), fmt.Errorf("unsupported unary operator: %s", node.Op), "")
	}
}
