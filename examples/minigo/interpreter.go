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

// formatErrorWithContext creates a detailed error message including file, line, column, source code, and call stack.
func (i *Interpreter) formatErrorWithContext(fset *token.FileSet, pos token.Pos, originalErr error, customMsg string) error {
	var errorBuilder strings.Builder

	baseErrMsg := ""
	if originalErr != nil {
		baseErrMsg = originalErr.Error()
	}

	if pos == token.NoPos {
		if customMsg != "" {
			if originalErr != nil {
				errorBuilder.WriteString(fmt.Sprintf("%s: %s", customMsg, baseErrMsg))
			} else {
				errorBuilder.WriteString(customMsg)
			}
		} else if originalErr != nil {
			errorBuilder.WriteString(baseErrMsg)
		} else {
			errorBuilder.WriteString("unknown error")
		}
	} else {
		position := fset.Position(pos)
		sourceLineStr := getSourceLine(position.Filename, position.Line)

		detailMsg := fmt.Sprintf("Error in %s at line %d, column %d", position.Filename, position.Line, position.Column)
		if customMsg != "" {
			detailMsg = fmt.Sprintf("%s: %s", customMsg, detailMsg)
		}
		errorBuilder.WriteString(detailMsg)

		if sourceLineStr != "" {
			errorBuilder.WriteString(fmt.Sprintf("\n  Source: %s", sourceLineStr))
		}
		if baseErrMsg != "" {
			errorBuilder.WriteString(fmt.Sprintf("\n  Details: %s", baseErrMsg))
		}
	}

	// Add minigo call stack
	if len(i.callStack) > 0 {
		errorBuilder.WriteString("\nMinigo Call Stack:")
		for idx, frame := range i.callStack {
			framePositionStr := ""
			sourceLineStr := ""
			if frame.callPosition.IsValid() {
				framePositionStr = fmt.Sprintf(" (called at %s:%d:%d)", filepath.Base(frame.callPosition.Filename), frame.callPosition.Line, frame.callPosition.Column)
				sourceLineStr = getSourceLine(frame.callPosition.Filename, frame.callPosition.Line)
				if sourceLineStr != "" {
					sourceLineStr = fmt.Sprintf("\n    Source: %s", sourceLineStr)
				}
			}
			errorBuilder.WriteString(fmt.Sprintf("\n  %d: %s%s%s", idx, frame.functionName, framePositionStr, sourceLineStr))
		}
	}

	// Check if originalErr already contains the formatted error message.
	// This can happen if formatErrorWithContext is called multiple times.
	// If so, just return originalErr.
	// This is a simple check; more robust detection might be needed.
	if originalErr != nil && strings.Contains(originalErr.Error(), "Minigo Call Stack:") {
		return originalErr
	}

	if originalErr != nil {
		// Wrap the original error to preserve its type if needed, while adding the context.
		// However, since we are building a new string, we create a new error.
		// If originalErr is of a special type that needs to be preserved, this approach might need adjustment.
		return errors.New(errorBuilder.String())
	}
	return errors.New(errorBuilder.String())
}

// getSourceLine reads a specific line from a file.
func getSourceLine(filename string, lineNum int) string {
	if filename == "" || lineNum <= 0 {
		return "[No source line available: invalid input]"
	}
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Sprintf("[Error opening source file '%s': %v]", filename, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	currentLine := 1
	for scanner.Scan() {
		if currentLine == lineNum {
			return strings.TrimSpace(scanner.Text())
		}
		currentLine++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Sprintf("[Error reading source line from '%s': %v]", filename, err)
	}
	return fmt.Sprintf("[Source line %d not found in '%s']", lineNum, filename)
}

// parseInt64 is a helper function to parse a string to an int64.
// It's defined here to keep the main eval function cleaner.
func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 0, 64)
}

// astNodeToString converts an AST node to its string representation.
// This is a helper for error messages. It's not exhaustive.
func (i *Interpreter) astNodeToString(node ast.Node, fset *token.FileSet) string {
	// This is a simplified version. For more complex nodes, you might need
	// to use format.Node from go/format, but that requires an io.Writer.
	// For simple identifiers or selectors, this should suffice.
	switch n := node.(type) {
	case *ast.Ident:
		return n.Name
	case *ast.SelectorExpr:
		return i.astNodeToString(n.X, fset) + "." + n.Sel.Name
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
	callStack     []*callStackFrame // For minigo call stack trace
}

// callStackFrame stores information about a single function call.
type callStackFrame struct {
	functionName string
	callPosition token.Position // Position where the function was called
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

	// Register len() function
	env.Define("len", &BuiltinFunction{
		Name: "len",
		Fn: func(env *Environment, args ...Object) (Object, error) {
			if len(args) != 1 {
				// TODO: Later, use i.formatErrorWithContext if possible from builtins, or ensure evalCallExpr wraps it.
				return nil, fmt.Errorf("len() takes exactly one argument (%d given)", len(args))
			}
			switch arg := args[0].(type) {
			case *String:
				return &Integer{Value: int64(len(arg.Value))}, nil
			case *Array:
				return &Integer{Value: int64(len(arg.Elements))}, nil
			case *Slice:
				return &Integer{Value: int64(len(arg.Elements))}, nil
			case *Map:
				return &Integer{Value: int64(len(arg.Pairs))}, nil
			default:
				return nil, fmt.Errorf("len() not supported for type %s", args[0].Type())
			}
		},
	})

	// Register append() function
	env.Define("append", &BuiltinFunction{
		Name: "append",
		Fn: func(env *Environment, args ...Object) (Object, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("append() takes at least one argument (slice)")
			}

			sliceObj, ok := args[0].(*Slice)
			if !ok {
				return nil, fmt.Errorf("first argument to append() must be a slice, got %s", args[0].Type())
			}

			newElements := make([]Object, len(sliceObj.Elements))
			copy(newElements, sliceObj.Elements)

			for _, arg := range args[1:] {
				newElements = append(newElements, arg)
			}
			return &Slice{Elements: newElements}, nil
		},
	})

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
		// We don't have a specific token.Pos here for i.formatErrorWithContext if ParseFile itself fails.
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
			return i.formatErrorWithContext(i.FileSet, token.NoPos, errSharedGs, fmt.Sprintf("Failed to create default shared go-scan scanner (path: %s): %v", scanPathForShared, errSharedGs))
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
				return i.formatErrorWithContext(i.FileSet, impSpec.Path.Pos(), err, fmt.Sprintf("Invalid import path: %s", impSpec.Path.Value))
			}

			localName := ""
			if impSpec.Name != nil {
				localName = impSpec.Name.Name
				if localName == "_" {
					// Blank imports are ignored, do not add to importAliasMap
					continue
				}
				if localName == "." {
					return i.formatErrorWithContext(i.FileSet, impSpec.Name.Pos(), errors.New("dot imports are not supported"), "")
				}
			} else {
				localName = filepath.Base(importPath)
			}

			if existingPath, ok := i.importAliasMap[localName]; ok && existingPath != importPath {
				return i.formatErrorWithContext(i.FileSet, impSpec.Pos(), fmt.Errorf("import alias/name %q already used for %q, cannot reuse for %q", localName, existingPath, importPath), "")
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
				return i.formatErrorWithContext(i.FileSet, genDecl.Pos(), evalErr, "Error evaluating type declaration in main script")
			}
		}
	}

	// Second pass: Process function declarations from the main script's AST
	for _, declNode := range mainFileAst.Decls {
		if fnDecl, ok := declNode.(*ast.FuncDecl); ok {
			_, evalErr := i.evalFuncDecl(ctx, fnDecl, i.globalEnv) // evalFuncDecl takes *ast.FuncDecl
			if evalErr != nil {
				return i.formatErrorWithContext(i.FileSet, fnDecl.Pos(), evalErr, fmt.Sprintf("Error evaluating function declaration %s in main script", fnDecl.Name.Name))
			}
		}
	}

	// Third pass: Process global variable declarations from the main script's AST
	for _, declNode := range mainFileAst.Decls {
		if genDecl, ok := declNode.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			tempDeclStmt := &ast.DeclStmt{Decl: genDecl}
			_, evalErr := i.eval(ctx, tempDeclStmt, i.globalEnv)
			if evalErr != nil {
				return i.formatErrorWithContext(i.FileSet, genDecl.Pos(), evalErr, "Error evaluating global variable declaration in main script")
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
			return i.formatErrorWithContext(i.activeFileSet, token.NoPos, errSharedGs, fmt.Sprintf("Failed to create default shared go-scan scanner (path: %s): %v", scanPathForShared, errSharedGs))
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
				return i.formatErrorWithContext(i.activeFileSet, impSpec.Path.Pos(), errPath, fmt.Sprintf("Invalid import path: %s", impSpec.Path.Value))
			}

			localName := ""
			if impSpec.Name != nil {
				localName = impSpec.Name.Name
				if localName == "_" {
					// Blank imports are ignored, do not add to importAliasMap
					continue
				}
				if localName == "." {
					return i.formatErrorWithContext(i.activeFileSet, impSpec.Name.Pos(), errors.New("dot imports are not supported"), "")
				}
			} else {
				localName = filepath.Base(importPathVal)
			}

			if existingPath, ok := i.importAliasMap[localName]; ok && existingPath != importPathVal {
				return i.formatErrorWithContext(i.activeFileSet, impSpec.Pos(), fmt.Errorf("import alias/name %q already used for %q, cannot reuse for %q", localName, existingPath, importPathVal), "")
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
				return i.formatErrorWithContext(i.activeFileSet, genDecl.Pos(), evalErr, "Error evaluating type declaration in main script")
			}
		}
	}

	// Second pass: Process function declarations from the main script's AST
	for _, declNode := range mainFileAst.Decls {
		if fnDecl, ok := declNode.(*ast.FuncDecl); ok {
			_, evalErr := i.evalFuncDecl(ctx, fnDecl, i.globalEnv) // evalFuncDecl takes *ast.FuncDecl
			if evalErr != nil {
				return i.formatErrorWithContext(i.activeFileSet, fnDecl.Pos(), evalErr, fmt.Sprintf("Error evaluating function declaration %s in main script", fnDecl.Name.Name))
			}
		}
	}

	// Third pass: Process global variable declarations from the main script's AST
	for _, declNode := range mainFileAst.Decls {
		if genDecl, ok := declNode.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			tempDeclStmt := &ast.DeclStmt{Decl: genDecl}
			_, evalErr := i.eval(ctx, tempDeclStmt, i.globalEnv)
			if evalErr != nil {
				return i.formatErrorWithContext(i.activeFileSet, genDecl.Pos(), evalErr, "Error evaluating global variable declaration in main script")
			}
		}
	}

	// Get the entry function *object* from the global environment
	entryFuncObj, ok := i.globalEnv.Get(entryPoint)
	if !ok {
		return i.formatErrorWithContext(i.activeFileSet, token.NoPos, fmt.Errorf("entry point function '%s' not found in global environment", entryPoint), "Setup error")
	}

	userEntryFunc, ok := entryFuncObj.(*UserDefinedFunction)
	if !ok {
		return i.formatErrorWithContext(i.activeFileSet, token.NoPos, fmt.Errorf("entry point '%s' is not a user-defined function (type: %s)", entryPoint, entryFuncObj.Type()), "Setup error")
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

	// Push to call stack
	frame := &callStackFrame{
		functionName: fn.Name,
	}
	if callPos != token.NoPos && callerFileSet != nil { // Ensure callerFileSet is not nil
		frame.callPosition = callerFileSet.Position(callPos)
	}
	i.callStack = append(i.callStack, frame)
	defer func() {
		if len(i.callStack) > 0 {
			i.callStack = i.callStack[:len(i.callStack)-1] // Pop from call stack
		}
	}()

	if len(args) != len(fn.Parameters) {
		errMsg := fmt.Sprintf("wrong number of arguments for function %s: expected %d, got %d", fn.Name, len(fn.Parameters), len(args))
		return nil, i.formatErrorWithContext(i.activeFileSet, callPos, errors.New(errMsg), "Function call error")
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
				return nil, i.formatErrorWithContext(i.activeFileSet, paramTypeExpr.Pos(),
					fmt.Errorf("error resolving type for parameter %s: %w", paramIdent.Name, errType), "Function call error")
			}

			actualObjType := arg.Type()

			if actualObjType != expectedObjType {
				errMsg := fmt.Sprintf("type mismatch for argument %d (%s) of function %s: expected %s, got %s",
					idx+1, paramIdent.Name, fn.Name, expectedObjType, actualObjType)
				return nil, i.formatErrorWithContext(i.activeFileSet, callPos, errors.New(errMsg), "Function call error") // Use callPos for argument error
			}

			if expectedObjType == STRUCT_INSTANCE_OBJ {
				actualStructInstance, ok := arg.(*StructInstance)
				if !ok { // Should not happen if actualObjType matched STRUCT_INSTANCE_OBJ
					errMsg := fmt.Sprintf("internal error: argument %d (%s) type is STRUCT_INSTANCE_OBJ but not a StructInstance", idx+1, paramIdent.Name)
					return nil, i.formatErrorWithContext(i.activeFileSet, callPos, errors.New(errMsg), "Internal error")
				}
				if actualStructInstance.Definition != expectedStructDef {
					expectedName := "unknown"
					if expectedStructDef != nil { expectedName = expectedStructDef.Name }
					actualName := "unknown"
					if actualStructInstance.Definition != nil { actualName = actualStructInstance.Definition.Name }

					errMsg := fmt.Sprintf("type mismatch for struct argument %d (%s) of function %s: expected struct type %s, got %s",
						idx+1, paramIdent.Name, fn.Name, expectedName, actualName)
					return nil, i.formatErrorWithContext(i.activeFileSet, callPos, errors.New(errMsg), "Function call error")
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
		return i.evalIdentifier(n, env, i.activeFileSet) // Pass activeFileSet and Interpreter

	case *ast.BasicLit:
		switch n.Kind {
		case token.STRING:
			return &String{Value: n.Value[1 : len(n.Value)-1]}, nil
		case token.INT:
			val, err := parseInt64(n.Value)
			if err != nil {
				return nil, i.formatErrorWithContext(i.activeFileSet, n.Pos(), err, fmt.Sprintf("Could not parse integer literal '%s'", n.Value))
			}
			return &Integer{Value: val}, nil
		default:
			return nil, i.formatErrorWithContext(i.activeFileSet, n.Pos(), fmt.Errorf("unsupported literal type: %s", n.Kind), fmt.Sprintf("Unsupported literal value: %s", n.Value))
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

	case *ast.IndexExpr:
		return i.evalIndexExpression(ctx, n, env) // Placeholder for now

	case *ast.SliceExpr:
		return i.evalSliceExpression(ctx, n, env) // Placeholder for now

	default:
		return nil, i.formatErrorWithContext(i.activeFileSet, n.Pos(), fmt.Errorf("unsupported AST node type: %T", n), fmt.Sprintf("Unsupported AST node value: %+v", n))
	}
}

func (i *Interpreter) evalCompositeLit(ctx context.Context, lit *ast.CompositeLit, env *Environment) (Object, error) {
	switch typeNode := lit.Type.(type) {
	case *ast.Ident: // e.g. MyStruct{...}
		return i.evalStructLiteral(ctx, typeNode.Name, nil, lit, env)
	case *ast.SelectorExpr: // e.g. pkg.MyStruct{...}
		pkgIdent, ok := typeNode.X.(*ast.Ident)
		if !ok {
			return nil, i.formatErrorWithContext(i.activeFileSet, typeNode.X.Pos(), fmt.Errorf("package selector X in composite literal type must be an identifier, got %T", typeNode.X), "Struct instantiation error")
		}
		return i.evalStructLiteral(ctx, typeNode.Sel.Name, pkgIdent, lit, env)

	case *ast.ArrayType: // e.g. [3]int{1,2,3} or []int{1,2,3}
		// Distinguish between Array and Slice based on lit.Type.Len
		// For slice: typeNode.Len == nil
		// For array: typeNode.Len != nil (e.g. *ast.BasicLit for fixed size)

		elements := make([]Object, len(lit.Elts))
		for k, elt := range lit.Elts {
			// Array/Slice elements are not KeyValueExpr unless it's like `[]int{0:1, 1:2}` which is less common for basic literals.
			// For now, assume direct value expressions.
			if _, ok := elt.(*ast.KeyValueExpr); ok {
				return nil, i.formatErrorWithContext(i.activeFileSet, elt.Pos(), fmt.Errorf("keyed elements not supported in array/slice literals yet"), "Literal error")
			}
			evaluatedElt, err := i.eval(ctx, elt, env)
			if err != nil {
				return nil, err
			}
			elements[k] = evaluatedElt
		}

		if typeNode.Len == nil { // Slice literal: []T{...}
			return &Slice{Elements: elements}, nil
		} else { // Array literal: [N]T{...}
			// Evaluate the length expression for the array type.
			// This is part of the type definition, not the literal values usually.
			// E.g. `[N]int{}` where N is a const. `[3]int{}` has BasicLit for Len.
			// For now, we assume fixed size defined by number of elements if Len is complex,
			// or direct from BasicLit if simple.
			// Proper array type checking (element types against typeNode.Elt) is also needed.
			// For simplicity, we're not deeply checking typeNode.Elt against evaluatedElt types here yet.

			var arrayLength int64 = -1 // Sentinel for "not yet determined" or "error"
			switch lenExpr := typeNode.Len.(type) {
			case *ast.BasicLit:
				if lenExpr.Kind == token.INT {
					l, err := strconv.ParseInt(lenExpr.Value, 10, 64)
					if err != nil {
						return nil, i.formatErrorWithContext(i.activeFileSet, lenExpr.Pos(), fmt.Errorf("invalid array length: %s", lenExpr.Value), "Type error")
					}
					arrayLength = l
				} else {
					return nil, i.formatErrorWithContext(i.activeFileSet, lenExpr.Pos(), fmt.Errorf("array length must be an integer, got %s", lenExpr.Kind), "Type error")
				}
			case nil: // This case should be caught by `typeNode.Len == nil` above for slices.
				return nil, i.formatErrorWithContext(i.activeFileSet, typeNode.Pos(), errors.New("internal error: array type has nil length expression"), "Type error")
			default:
				// TODO: Support constant expressions for array length.
				// For now, if Len is not a BasicLit, we could infer length from elements if that's desired,
				// or error out. Go requires array lengths to be constant.
				return nil, i.formatErrorWithContext(i.activeFileSet, typeNode.Len.Pos(), fmt.Errorf("array length must be a constant integer literal, got %T", typeNode.Len), "Type error")
			}

			if arrayLength < 0 { // Should have been set or errored by now.
				return nil, i.formatErrorWithContext(i.activeFileSet, typeNode.Pos(), errors.New("could not determine array length"), "Type error")
			}

			if int64(len(elements)) > arrayLength {
				return nil, i.formatErrorWithContext(i.activeFileSet, lit.Rbrace, fmt.Errorf("too many elements in array literal (expected %d, got %d)", arrayLength, len(elements)), "Literal error")
			}

			// If fewer elements are provided than the array length, Go zero-fills the rest.
			// We need to know the zero value for typeNode.Elt. This is complex.
			// For now, let's require the number of elements to match the length for simplicity, or pad with NULL.
			// This is a simplification. True Go behavior requires typed zero values.
			finalElements := make([]Object, arrayLength)
			copy(finalElements, elements)
			for i := len(elements); i < int(arrayLength); i++ {
				// TODO: This should be the typed zero value of typeNode.Elt
				finalElements[i] = NULL
			}
			return &Array{Elements: finalElements}, nil
		}

	case *ast.MapType: // e.g. map[string]int{"a":1, "b":2}
		// ast.MapType has Key and Value fields (ast.Expr)
		// lit.Elts will be a slice of *ast.KeyValueExpr
		mapObj := &Map{Pairs: make(map[HashKey]MapPair)}

		for _, elt := range lit.Elts {
			kvExpr, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				return nil, i.formatErrorWithContext(i.activeFileSet, elt.Pos(), fmt.Errorf("map literal elements must be key-value pairs"), "Literal error")
			}

			keyObj, err := i.eval(ctx, kvExpr.Key, env)
			if err != nil {
				return nil, err
			}
			valObj, err := i.eval(ctx, kvExpr.Value, env)
			if err != nil {
				return nil, err
			}

			hashableKey, ok := keyObj.(Hashable)
			if !ok {
				return nil, i.formatErrorWithContext(i.activeFileSet, kvExpr.Key.Pos(), fmt.Errorf("map key type %s is not hashable", keyObj.Type()), "Type error")
			}
			hk, err := hashableKey.HashKey()
			if err != nil {
				return nil, i.formatErrorWithContext(i.activeFileSet, kvExpr.Key.Pos(), fmt.Errorf("error getting hash key for type %s: %v", keyObj.Type(), err), "Type error")
			}

			mapObj.Pairs[hk] = MapPair{Key: keyObj, Value: valObj}
		}
		return mapObj, nil

	default:
		// This case might be reached if lit.Type is nil (e.g. for untyped composite literals like `[]int{1,2}` if parser allows that at top level)
		// or some other ast.Expr that isn't an Ident, SelectorExpr, ArrayType, or MapType.
		// Go typically requires types for composite literals.
		return nil, i.formatErrorWithContext(i.activeFileSet, lit.Type.Pos(), fmt.Errorf("unsupported type for composite literal: %T", lit.Type), "Literal error")
	}
}

func (i *Interpreter) evalStructLiteral(ctx context.Context, structName string, pkgIdent *ast.Ident, lit *ast.CompositeLit, env *Environment) (Object, error) {
	var structDef *StructDefinition
	var qualifiedName string
	var typePos token.Pos

	if pkgIdent == nil { // Simple identifier for struct type
		qualifiedName = structName
		// Assuming lit.Type was *ast.Ident, its Pos can be used.
		// This is a bit indirect. If lit.Type is available, use its Pos.
		if identType, ok := lit.Type.(*ast.Ident); ok {
			typePos = identType.Pos()
		} else {
			typePos = lit.Lbrace // Fallback
		}
	} else { // Selector expression for struct type (pkg.StructName)
		qualifiedName = pkgIdent.Name + "." + structName
		// Assuming lit.Type was *ast.SelectorExpr
		if selType, ok := lit.Type.(*ast.SelectorExpr); ok {
			typePos = selType.Sel.Pos() // Position of the struct name itself
		} else {
			typePos = lit.Lbrace // Fallback
		}
	}

	obj, found := env.Get(qualifiedName)
	if !found {
		if pkgIdent != nil { // If it was a qualified name, try loading the package
			_, errLoad := i.loadPackageIfNeeded(ctx, pkgIdent.Name, i.globalEnv, pkgIdent.Pos())
			if errLoad != nil {
				return nil, i.formatErrorWithContext(i.activeFileSet, pkgIdent.Pos(), errLoad, fmt.Sprintf("Error loading package '%s' for struct literal type '%s'", pkgIdent.Name, qualifiedName))
			}
			obj, found = env.Get(qualifiedName) // Retry after load attempt
		}
		if !found {
			return nil, i.formatErrorWithContext(i.activeFileSet, typePos, fmt.Errorf("undefined type '%s' used in composite literal", qualifiedName), "Struct instantiation error")
		}
	}

	sDef, ok := obj.(*StructDefinition)
	if !ok {
		return nil, i.formatErrorWithContext(i.activeFileSet, typePos, fmt.Errorf("type '%s' is not a struct type, but %s", qualifiedName, obj.Type()), "Struct instantiation error")
	}
	structDef = sDef

	instance := &StructInstance{
		Definition:     structDef,
		FieldValues:    make(map[string]Object),
		EmbeddedValues: make(map[string]*StructInstance),
	}

	if len(lit.Elts) == 0 { // Handles T{}
		// No elements, just return the zero-value struct instance.
		// Fields are already empty/nil. EmbeddedValues also empty.
		return instance, nil
	}

	// Check if elements are of the form {key: value} or just {value, value}
	// Go struct literals can be keyed or unkeyed. Unkeyed requires all fields in order.
	// Keyed can be partial and in any order.
	// For simplicity, our previous version only supported keyed or fully empty.
	// Let's maintain keyed-only for now, or error on unkeyed if fields exist.

	_, isKeyValueForm := lit.Elts[0].(*ast.KeyValueExpr)

	if isKeyValueForm {
		for _, elt := range lit.Elts {
			kvExpr, ok := elt.(*ast.KeyValueExpr)
			if !ok { // Should not happen if isKeyValueForm was true and all elements are consistent
				return nil, i.formatErrorWithContext(i.activeFileSet, elt.Pos(), fmt.Errorf("mixture of keyed and non-keyed fields in struct literal for '%s'", structDef.Name), "Struct instantiation error")
			}
			keyIdent, ok := kvExpr.Key.(*ast.Ident)
			if !ok {
				return nil, i.formatErrorWithContext(i.activeFileSet, kvExpr.Key.Pos(), fmt.Errorf("struct field key must be an identifier, got %T for struct '%s'", kvExpr.Key, structDef.Name), "Struct instantiation error")
			}
			fieldName := keyIdent.Name
			valueExpr := kvExpr.Value

			// Logic for direct fields, embedded type initialization, and promoted fields
			// This part is complex and needs to correctly resolve fieldName.
			// (Using existing logic from original evalCompositeLit for this part)
			if _, isDirectField := structDef.Fields[fieldName]; isDirectField {
				valObj, err := i.eval(ctx, valueExpr, env)
				if err != nil { return nil, err }
				instance.FieldValues[fieldName] = valObj
				continue
			}

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
					return nil, i.formatErrorWithContext(i.activeFileSet, kvExpr.Value.Pos(),
						fmt.Errorf("value for embedded struct '%s' is not a compatible struct instance (expected '%s', got '%s')",
							fieldName, targetEmbeddedDefForExplicitInit.Name, valObj.Type()), "Struct instantiation error")
				}
				instance.EmbeddedValues[fieldName] = embInstanceVal
				continue
			}

			var owningEmbDef *StructDefinition
			for _, embDef := range structDef.EmbeddedDefs {
				if _, isPromoted := embDef.Fields[fieldName]; isPromoted {
					owningEmbDef = embDef; break
				}
			}
			if owningEmbDef != nil {
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
			return nil, i.formatErrorWithContext(i.activeFileSet, keyIdent.Pos(), fmt.Errorf("unknown field '%s' in struct literal of type '%s'", fieldName, structDef.Name), "Struct instantiation error")
		}
	} else { // Not KeyValue form (e.g. MyStruct{val1, val2})
		// Go allows this if all fields are provided in order.
		// This is more complex to implement correctly with type checking and field order.
		// For now, if it's not keyed and there are elements, and the struct has fields, we error.
		// If structDef.Fields is empty and lit.Elts is not empty (and not keyed), it's also an error.
		if len(lit.Elts) > 0 { // If there are elements but not in key-value form
			if len(structDef.Fields) > 0 || len(structDef.EmbeddedDefs) > 0 { // And struct expects fields/embedded
				return nil, i.formatErrorWithContext(i.activeFileSet, lit.Pos(), fmt.Errorf("ordered (non-keyed) struct literal values are not supported yet for struct '%s' that has fields/embedded types. Use keyed values e.g. Field:Val.", structDef.Name), "Struct instantiation error")
			} else { // Struct has no fields/embedded, but values provided without keys
				return nil, i.formatErrorWithContext(i.activeFileSet, lit.Pos(), fmt.Errorf("non-keyed values provided for struct '%s' which has no fields or embedded types", structDef.Name), "Struct instantiation error")
			}
		}
		// If len(lit.Elts) == 0, it was handled above.
	}
	return instance, nil
}

func (i *Interpreter) evalIndexExpression(ctx context.Context, node *ast.IndexExpr, env *Environment) (Object, error) {
	// Placeholder - TODO: Implement logic for array, slice, map indexing
	left, err := i.eval(ctx, node.X, env)
	if err != nil {
		return nil, err
	}

	index, err := i.eval(ctx, node.Index, env)
	if err != nil {
		return nil, err
	}

	switch leftObj := left.(type) {
	case *Array:
		idx, ok := index.(*Integer)
		if !ok {
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Index.Pos(), fmt.Errorf("array index must be an integer, got %s", index.Type()), "Type error")
		}
		if idx.Value < 0 || idx.Value >= int64(len(leftObj.Elements)) {
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Index.Pos(), fmt.Errorf("index out of bounds: %d for array of length %d", idx.Value, len(leftObj.Elements)), "Runtime error")
		}
		return leftObj.Elements[idx.Value], nil
	case *Slice:
		idx, ok := index.(*Integer)
		if !ok {
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Index.Pos(), fmt.Errorf("slice index must be an integer, got %s", index.Type()), "Type error")
		}
		if idx.Value < 0 || idx.Value >= int64(len(leftObj.Elements)) {
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Index.Pos(), fmt.Errorf("index out of bounds: %d for slice of length %d", idx.Value, len(leftObj.Elements)), "Runtime error")
		}
		return leftObj.Elements[idx.Value], nil
	case *Map:
		hashableKey, ok := index.(Hashable)
		if !ok {
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Index.Pos(), fmt.Errorf("map key type %s is not hashable", index.Type()), "Type error")
		}
		hk, err := hashableKey.HashKey()
		if err != nil {
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Index.Pos(), fmt.Errorf("error getting hash key for map access: %v", err), "Runtime error")
		}
		if pair, found := leftObj.Pairs[hk]; found {
			return pair.Value, nil
		}
		// Accessing a non-existent map key in Go returns the zero value of the value type.
		// For our interpreter, returning NULL is a simplification.
		// TODO: Consider returning typed zero values if the map's value type is known.
		return NULL, nil
	default:
		return nil, i.formatErrorWithContext(i.activeFileSet, node.X.Pos(), fmt.Errorf("type %s does not support indexing", left.Type()), "Type error")
	}
}

func (i *Interpreter) evalSliceExpression(ctx context.Context, node *ast.SliceExpr, env *Environment) (Object, error) {
	// Placeholder - TODO: Implement logic for slice expressions a[low:high]
	left, err := i.eval(ctx, node.X, env)
	if err != nil {
		return nil, err
	}

	var low, high, max int64 = 0, -1, -1 // Initialize high and max to -1 to indicate they are not set by default

	if node.Low != nil {
		lowObj, err := i.eval(ctx, node.Low, env)
		if err != nil {
			return nil, err
		}
		lowInt, ok := lowObj.(*Integer)
		if !ok {
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Low.Pos(), fmt.Errorf("slice index must be integer, got %s", lowObj.Type()), "Type error")
		}
		low = lowInt.Value
	}

	if node.High != nil {
		highObj, err := i.eval(ctx, node.High, env)
		if err != nil {
			return nil, err
		}
		highInt, ok := highObj.(*Integer)
		if !ok {
			return nil, i.formatErrorWithContext(i.activeFileSet, node.High.Pos(), fmt.Errorf("slice index must be integer, got %s", highObj.Type()), "Type error")
		}
		high = highInt.Value
	}

	if node.Max != nil {
		maxObj, err := i.eval(ctx, node.Max, env)
		if err != nil {
			return nil, err
		}
		maxInt, ok := maxObj.(*Integer)
		if !ok {
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Max.Pos(), fmt.Errorf("slice capacity index must be integer, got %s", maxObj.Type()), "Type error")
		}
		max = maxInt.Value
		if node.Slice3 { // Max is only used in 3-index slices
			// high must be set if max is set in a 3-index slice
			if node.High == nil {
				return nil, i.formatErrorWithContext(i.activeFileSet, node.Max.Pos(), fmt.Errorf("middle index required in 3-index slice"), "Syntax error")
			}
		} else {
			// If not a 3-index slice, max should not have been parsed or evaluated.
			// This indicates an AST structure we don't expect or handle for 2-index slices.
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Max.Pos(), fmt.Errorf("max capacity is only allowed in 3-index slices"), "Syntax error")
		}
	}


	switch subject := left.(type) {
	case *Array:
		arrLen := int64(len(subject.Elements))
		if node.High == nil { // a[low:]
			high = arrLen
		}
		// Bounds checks (simplified from Go spec for now)
		if low < 0 || low > arrLen || (node.High != nil && (high < low || high > arrLen)) {
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Lbrack, fmt.Errorf("slice bounds out of range for array (low:%d, high:%d, len:%d)", low, high, arrLen), "Runtime error")
		}
		if node.Slice3 {
			if max < high || max > arrLen {
				return nil, i.formatErrorWithContext(i.activeFileSet, node.Max.Pos(), fmt.Errorf("slice3 capacity out of range (max:%d, high:%d, len:%d)",max, high, arrLen), "Runtime error")
			}
		}

		// Create a new slice. For arrays, slicing always creates a new slice object that copies elements.
		// Go's slices share underlying array, but our simple model might copy.
		// For now, let's copy to keep it simple.
		newElements := make([]Object, high-low)
		copy(newElements, subject.Elements[low:high])
		return &Slice{Elements: newElements}, nil

	case *Slice:
		sliceLen := int64(len(subject.Elements))
		if node.High == nil { // a[low:]
			high = sliceLen
		}

		// Bounds checks (simplified)
		if low < 0 || low > sliceLen || (node.High != nil && (high < low || high > sliceLen)) {
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Lbrack, fmt.Errorf("slice bounds out of range for slice (low:%d, high:%d, len:%d)", low, high, sliceLen), "Runtime error")
		}
		if node.Slice3 {
			// Go spec: "For arrays or strings, the indices are in range if 0 <= low <= high <= max <= cap(a)"
			// Our slices don't have explicit capacity yet, so treat sliceLen as capacity for this check.
			if max < high || max > sliceLen { // Using sliceLen as effective capacity
				return nil, i.formatErrorWithContext(i.activeFileSet, node.Max.Pos(), fmt.Errorf("slice3 capacity out of range for slice (max:%d, high:%d, len:%d)",max, high, sliceLen), "Runtime error")
			}
		}


		// Slicing a slice: also copy for simplicity in this interpreter model.
		newElements := make([]Object, high-low)
		copy(newElements, subject.Elements[low:high])
		return &Slice{Elements: newElements}, nil

	default:
		return nil, i.formatErrorWithContext(i.activeFileSet, node.X.Pos(), fmt.Errorf("type %s does not support slicing", left.Type()), "Type error")
	}
}


func (i *Interpreter) evalBranchStmt(ctx context.Context, stmt *ast.BranchStmt, env *Environment) (Object, error) {
	if stmt.Label != nil {
		return nil, i.formatErrorWithContext(i.activeFileSet, stmt.Pos(), fmt.Errorf("labeled break/continue not supported"), "")
	}

	switch stmt.Tok {
	case token.BREAK:
		return BREAK, nil
	case token.CONTINUE:
		return CONTINUE, nil
	default:
		return nil, i.formatErrorWithContext(i.activeFileSet, stmt.Pos(), fmt.Errorf("unsupported branch statement: %s", stmt.Tok), "")
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
				return nil, i.formatErrorWithContext(i.activeFileSet, stmt.Cond.Pos(),
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
		foundValue, _, _, embErr := i.findFieldInEmbedded(structInstance, fieldName, i.activeFileSet, node.Sel.Pos())
		if embErr != nil { return nil, embErr }
		if foundValue != nil { return foundValue, nil }
		return nil, i.formatErrorWithContext(i.activeFileSet, node.Sel.Pos(), fmt.Errorf("type %s has no field %s", structInstance.Definition.Name, fieldName), "Field access error")
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
			return nil, i.formatErrorWithContext(i.FileSet, identX.Pos(), fmt.Errorf("%s: %s (not a struct instance and not a known package alias/name)", originalErrorMsg, localPkgName), "Selector error")
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
		return nil, i.formatErrorWithContext(i.activeFileSet, node.Sel.Pos(), fmt.Errorf("undefined: %s.%s (package %s, path %s, was loaded or loading attempted)", localPkgName, fieldName, localPkgName, resolvedImportPath), "Selector error")
	}

	if xObj != nil { // This check should be xObj from the initial eval, not a new one.
		return nil, i.formatErrorWithContext(i.activeFileSet, node.X.Pos(), fmt.Errorf("selector base must be a struct instance or package identifier, got %s", xObj.Type()), "Unsupported selector expression")
	}
	return nil, i.formatErrorWithContext(i.activeFileSet, node.Pos(), errors.New("internal error in selector evaluation"), "")
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
				return "", nil, i.formatErrorWithContext(activeFset, te.Pos(), fmt.Errorf("type '%s' (resolved as '%s') is not a struct definition, but %s", typeName, qualifiedTypeName, obj.Type()), "")
			}
			// If not found qualified, it might be an error, or it could be a global built-in type not yet handled above.
			// For now, let's fall through to the general lookup, which might be an error.
		}

		// General lookup for identifier type names (e.g., local struct, or if not external context)
		obj, found := resolutionEnv.Get(typeName)
		if !found {
			return "", nil, i.formatErrorWithContext(activeFset, te.Pos(), fmt.Errorf("undefined type: %s", typeName), "")
		}
		if structDef, ok := obj.(*StructDefinition); ok {
			return STRUCT_INSTANCE_OBJ, structDef, nil
		}
		return "", nil, i.formatErrorWithContext(activeFset, te.Pos(), fmt.Errorf("type '%s' is not a struct definition, but %s", typeName, obj.Type()), "")

	case *ast.SelectorExpr: // e.g., pkg.TypeName
		pkgIdent, ok := te.X.(*ast.Ident)
		if !ok {
			return "", nil, i.formatErrorWithContext(activeFset, te.X.Pos(), fmt.Errorf("package selector in type must be an identifier, got %T", te.X), "Type resolution error")
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
				return "", nil, i.formatErrorWithContext(activeFset, te.Pos(), fmt.Errorf("failed to load package '%s' for type resolution of '%s': %w", pkgName, qualifiedName, loadErr), "")
			}
			// Try fetching again after loading attempt
			obj, found = i.globalEnv.Get(qualifiedName)
			if !found {
				return "", nil, i.formatErrorWithContext(activeFset, te.Pos(), fmt.Errorf("undefined type: %s (after attempting package load)", qualifiedName), "")
			}
		}


		if structDef, ok := obj.(*StructDefinition); ok {
			return STRUCT_INSTANCE_OBJ, structDef, nil
		}
		return "", nil, i.formatErrorWithContext(activeFset, te.Pos(), fmt.Errorf("qualified type '%s' is not a struct definition, but %s", qualifiedName, obj.Type()), "")
	// TODO: Handle *ast.StarExpr for pointers, *ast.ArrayType, *ast.MapType, *ast.InterfaceType etc.
	default:
		return "", nil, i.formatErrorWithContext(activeFset, typeExpr.Pos(), fmt.Errorf("unsupported AST node type for type resolution: %T", typeExpr), "")
	}
}

// loadPackageIfNeeded handles the logic for loading symbols from an imported package
// if it hasn't been loaded yet. It populates the provided 'env' (expected to be global)
// with the package's exported symbols, qualified by pkgAlias.
func (i *Interpreter) loadPackageIfNeeded(ctx context.Context, pkgAlias string, env *Environment, errorPos token.Pos) (*scanner.PackageInfo, error) {
	// Ensure sharedScanner is available
	if i.sharedScanner == nil {
		return nil, i.formatErrorWithContext(i.activeFileSet, errorPos, errors.New("shared go-scan scanner (for imports) not initialized in interpreter"), "Internal error")
	}

	importPath, knownAlias := i.importAliasMap[pkgAlias]
	if !knownAlias {
		// This case should ideally be caught before calling loadPackageIfNeeded,
		// e.g., in evalSelectorExpr, if pkgAlias is not in importAliasMap.
		// However, if called directly, this provides a safeguard.
		return nil, i.formatErrorWithContext(i.activeFileSet, errorPos, fmt.Errorf("package alias %s not found in import map", pkgAlias), "Import error")
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
		return nil, i.formatErrorWithContext(i.sharedScanner.Fset(), errorPos, fmt.Errorf("package %q (aliased as %q) not found or failed to scan: %w", importPath, pkgAlias, errImport), "Import error")
	}

	if importPkgInfo == nil {
		// Should be covered by errImport != nil, but as a safeguard.
		return nil, i.formatErrorWithContext(i.sharedScanner.Fset(), errorPos, fmt.Errorf("ScanPackageByImport returned nil for %q (%s) without error", importPath, pkgAlias), "Internal error")
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
func (i *Interpreter) findFieldInEmbedded(instance *StructInstance, fieldName string, fset *token.FileSet, selPos token.Pos) (foundValue Object, found bool, foundIn string, err error) {
	// ... (no change to i.activeFileSet usage here as fset is explicit)
	var overallFoundValue Object
	var overallFoundInDefinitionName string
	var numFoundPaths int = 0

	for _, embDef := range instance.Definition.EmbeddedDefs {
		embInstance, embInstanceExists := instance.EmbeddedValues[embDef.Name]
		if !embInstanceExists {
			if _, isFieldInEmbDef := embDef.Fields[fieldName]; isFieldInEmbDef {
				if numFoundPaths > 0 && overallFoundInDefinitionName != embDef.Name {
					return nil, false, "", i.formatErrorWithContext(fset, selPos,
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
				return nil, false, "", i.formatErrorWithContext(fset, selPos,
					fmt.Errorf("ambiguous selector %s (found in %s and as set field in %s)", fieldName, overallFoundInDefinitionName, embDef.Name), "")
			}
			overallFoundValue = val
			overallFoundInDefinitionName = embDef.Name
			numFoundPaths++
			continue
		}
		if _, isDirectField := embDef.Fields[fieldName]; isDirectField {
			if numFoundPaths > 0 && overallFoundInDefinitionName != embDef.Name {
				return nil, false, "", i.formatErrorWithContext(fset, selPos,
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
		recVal, recFound, recIn, recErr := i.findFieldInEmbedded(embInstance, fieldName, fset, selPos)
		if recErr != nil {
			return nil, false, "", recErr // Propagate ambiguity error from deeper level
		}
		if recFound {
			if numFoundPaths > 0 && overallFoundInDefinitionName != embDef.Name { // `embDef.Name` here means "found via this path"
				// If already found via a different top-level embedded struct, it's ambiguous.
				return nil, false, "", i.formatErrorWithContext(fset, selPos,
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
		return nil, false, "", i.formatErrorWithContext(fset, selPos,
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
	return nil, i.formatErrorWithContext(i.FileSet, fd.Pos(), fmt.Errorf("function declaration must have a name"), "")
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
		return nil, i.formatErrorWithContext(i.FileSet, rs.Pos(), fmt.Errorf("multiple return values not supported"), "")
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
		return nil, i.formatErrorWithContext(i.FileSet, declStmt.Pos(), fmt.Errorf("unsupported declaration type: %T", declStmt.Decl), "")
	}

	switch genDecl.Tok {
	case token.VAR:
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				return nil, i.formatErrorWithContext(i.FileSet, spec.Pos(), fmt.Errorf("unsupported spec type in var declaration: %T", spec), "")
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
						return nil, i.formatErrorWithContext(i.FileSet, valueSpec.Pos(), fmt.Errorf("variable '%s' declared without initializer must have a type", varName), "")
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
									return nil, i.formatErrorWithContext(i.FileSet, T.Pos(), fmt.Errorf("unsupported type '%s' for uninitialized variable '%s' (not a known struct)", T.Name, varName), "")
								}
							} else {
								// Type not found in environment
								return nil, i.formatErrorWithContext(i.FileSet, T.Pos(), fmt.Errorf("undefined type '%s' for uninitialized variable '%s'", T.Name, varName), "")
							}
						}
					case *ast.InterfaceType:
						if T.Methods == nil || len(T.Methods.List) == 0 {
							zeroVal = NULL
						} else {
							return nil, i.formatErrorWithContext(i.FileSet, T.Pos(), fmt.Errorf("unsupported specific interface type for uninitialized variable '%s'", varName), "")
						}
					case *ast.ArrayType:
						if T.Len == nil {
							// This is a slice type declaration, e.g., var s []int
							// The zero value for a slice is nil. We'll represent this as an empty Slice object.
							zeroVal = &Slice{Elements: nil} // Or []Object{} - nil is closer to Go's nil slice
						} else {
							// This is an array type declaration, e.g., var a [3]int
							var arrayLength int64
							lenLit, ok := T.Len.(*ast.BasicLit)
							if !ok || lenLit.Kind != token.INT {
								// TODO: Support constant expressions for array length in var declarations
								return nil, i.formatErrorWithContext(i.FileSet, T.Len.Pos(), fmt.Errorf("array length in var declaration must be an integer literal, got %T", T.Len), "")
							}
							l, err := strconv.ParseInt(lenLit.Value, 10, 64)
							if err != nil {
								return nil, i.formatErrorWithContext(i.FileSet, lenLit.Pos(), fmt.Errorf("invalid array length '%s' in var declaration", lenLit.Value), "")
							}
							arrayLength = l
							if arrayLength < 0 {
								return nil, i.formatErrorWithContext(i.FileSet, lenLit.Pos(), fmt.Errorf("array length cannot be negative: %d", arrayLength), "")
							}

							elements := make([]Object, arrayLength)
							// TODO: Fill with typed zero values of T.Elt instead of NULL if type info is available and used.
							// For now, all uninitialized array elements are NULL.
							for k := range elements {
								elements[k] = NULL
							}
							zeroVal = &Array{Elements: elements}
						}
					case *ast.MapType:
						// The zero value for a map is nil. We'll represent this as a Map object with a nil Pairs map.
						zeroVal = &Map{Pairs: nil} // Or make(map[HashKey]MapPair) for an empty map
					default:
						return nil, i.formatErrorWithContext(i.FileSet, valueSpec.Type.Pos(), fmt.Errorf("unsupported type expression for zero value for variable '%s': %T", varName, valueSpec.Type), "")
					}
					env.Define(varName, zeroVal)
				}
			}
		}
	case token.TYPE:
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				return nil, i.formatErrorWithContext(i.FileSet, spec.Pos(), fmt.Errorf("unsupported spec type in type declaration: %T", spec), "")
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
							return nil, i.formatErrorWithContext(i.FileSet, field.Type.Pos(), fmt.Errorf("field type '%s' in struct '%s' uses SelectorExpr, which is not fully supported for struct field types or embedding yet", i.astNodeToString(typeExpr, i.FileSet), typeName), "Struct definition error")
						default:
							return nil, i.formatErrorWithContext(i.FileSet, field.Type.Pos(), fmt.Errorf("struct field in '%s' has unsupported type specifier %T", typeName, field.Type), "Struct definition error")
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
								return nil, i.formatErrorWithContext(i.FileSet, field.Type.Pos(), fmt.Errorf("undefined type '%s' embedded in struct '%s'", fieldTypeName, typeName), "Struct definition error")
							}
							embeddedDef, ok := obj.(*StructDefinition)
							if !ok {
								return nil, i.formatErrorWithContext(i.FileSet, field.Type.Pos(), fmt.Errorf("type '%s' embedded in struct '%s' is not a struct definition (got %s)", fieldTypeName, typeName, obj.Type()), "Struct definition error")
							}
							embeddedDefs = append(embeddedDefs, embeddedDef)
						} else {
							// Regular named field
							// Ensure fieldTypeIdent is *ast.Ident for type name
							fieldTypeIdent, ok := field.Type.(*ast.Ident)
							if !ok {
								// This case might be redundant if the switch above handles all typeExpr variants,
								// but good for safety if only *ast.Ident is supported for field types.
								return nil, i.formatErrorWithContext(i.FileSet, field.Type.Pos(), fmt.Errorf("struct field '%s' in '%s' has complex type specifier %T; only simple type names supported for fields", field.Names[0].Name, typeName, field.Type), "")
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
				return nil, i.formatErrorWithContext(i.FileSet, typeSpec.Type.Pos(), fmt.Errorf("unsupported type specifier in type declaration '%s': %T", typeName, typeSpec.Type), "")
			}
		}
	default:
		return nil, i.formatErrorWithContext(i.FileSet, genDecl.Pos(), fmt.Errorf("unsupported declaration token: %s (expected VAR or TYPE)", genDecl.Tok), "")
	}
	// Processing is done within the switch cases.
	return nil, nil
}

func (i *Interpreter) evalIdentifier(ident *ast.Ident, env *Environment, fset *token.FileSet) (Object, error) {
	switch ident.Name {
	case "true":
		return TRUE, nil
	case "false":
		return FALSE, nil
	}
	if val, ok := env.Get(ident.Name); ok {
		return val, nil
	}
	return nil, i.formatErrorWithContext(fset, ident.Pos(), fmt.Errorf("identifier not found: %s", ident.Name), "")
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
		return i.evalStringBinaryExpr(node.Op, left.(*String), right.(*String), node.Pos())
	case left.Type() == INTEGER_OBJ && right.Type() == INTEGER_OBJ:
		return i.evalIntegerBinaryExpr(node.Op, left.(*Integer), right.(*Integer), node.Pos())
	case left.Type() == BOOLEAN_OBJ && right.Type() == BOOLEAN_OBJ:
		// TODO: Implement short-circuiting for token.LAND and token.LOR
		// Currently, both left and right operands are evaluated before this point.
		// For true short-circuiting, the evaluation of the right operand
		// would need to be conditional within these cases.
		return i.evalBooleanBinaryExpr(node.Op, left.(*Boolean), right.(*Boolean), node.Pos())
	default:
		return nil, i.formatErrorWithContext(i.FileSet, node.Pos(),
			fmt.Errorf("type mismatch or unsupported operation for binary expression: %s %s %s (left: %s, right: %s)", left.Type(), node.Op, right.Type(), left.Inspect(), right.Inspect()), "")
	}
}

func (i *Interpreter) evalIntegerBinaryExpr(op token.Token, left, right *Integer, pos token.Pos) (Object, error) {
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
			return nil, i.formatErrorWithContext(i.activeFileSet, pos, fmt.Errorf("division by zero"), "")
		}
		return &Integer{Value: leftVal / rightVal}, nil
	case token.REM:
		if rightVal == 0 {
			return nil, i.formatErrorWithContext(i.activeFileSet, pos, fmt.Errorf("division by zero (modulo)"), "")
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
		return nil, i.formatErrorWithContext(i.activeFileSet, pos, fmt.Errorf("unknown operator for integers: %s", op), "")
	}
}

func (i *Interpreter) evalStringBinaryExpr(op token.Token, left, right *String, pos token.Pos) (Object, error) {
	switch op {
	case token.EQL:
		return nativeBoolToBooleanObject(left.Value == right.Value), nil
	case token.NEQ:
		return nativeBoolToBooleanObject(left.Value != right.Value), nil
	case token.ADD:
		return &String{Value: left.Value + right.Value}, nil
	default:
		return nil, i.formatErrorWithContext(i.activeFileSet, pos, fmt.Errorf("unknown operator for strings: %s (left: %q, right: %q)", op, left.Value, right.Value), "")
	}
}

func (i *Interpreter) evalBooleanBinaryExpr(op token.Token, left, right *Boolean, pos token.Pos) (Object, error) {
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
		return nil, i.formatErrorWithContext(i.activeFileSet, pos,
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
		return nil, i.formatErrorWithContext(i.FileSet, node.Fun.Pos(), fmt.Errorf("cannot call non-function type %s (for function '%s')", funcObj.Type(), funcName), "")
	}
}

func (i *Interpreter) evalAssignStmt(ctx context.Context, assignStmt *ast.AssignStmt, env *Environment) (Object, error) {
	if len(assignStmt.Lhs) != 1 || len(assignStmt.Rhs) != 1 {
		return nil, i.formatErrorWithContext(i.FileSet, assignStmt.Pos(),
			fmt.Errorf("unsupported assignment: expected 1 expression on LHS and 1 on RHS, got %d and %d", len(assignStmt.Lhs), len(assignStmt.Rhs)), "")
	}

	lhsExpr := assignStmt.Lhs[0]
	rhsValue, err := i.eval(ctx, assignStmt.Rhs[0], env)
	if err != nil {
		return nil, err
	}

	switch lhsNode := lhsExpr.(type) {
	case *ast.Ident: // Standard variable assignment: x = val or x := val
		varName := lhsNode.Name
		switch assignStmt.Tok {
		case token.DEFINE: // :=
			if env.ExistsInCurrentScope(varName) {
				return nil, i.formatErrorWithContext(i.FileSet, lhsNode.Pos(), fmt.Errorf("no new variables on left side of := (variable '%s' already declared in this scope)", varName), "")
			}
			env.Define(varName, rhsValue)
			return nil, nil
		case token.ASSIGN: // =
			if _, ok := env.Assign(varName, rhsValue); !ok {
				return nil, i.formatErrorWithContext(i.FileSet, lhsNode.Pos(), fmt.Errorf("cannot assign to undeclared variable '%s'", varName), "")
			}
			return nil, nil
		default: // Augmented assignments for identifiers: x += val, etc.
			existingVal, ok := env.Get(varName)
			if !ok {
				return nil, i.formatErrorWithContext(i.FileSet, lhsNode.Pos(), fmt.Errorf("cannot use %s on undeclared variable '%s'", assignStmt.Tok, varName), "")
			}
			newVal, err := i.performAugmentedAssignOperation(assignStmt.Tok, existingVal, rhsValue, assignStmt.Pos())
			if err != nil {
				// Error from performAugmentedAssignOperation is already formatted.
				// Add context specific to the variable and operation type.
				return nil, i.formatErrorWithContext(i.FileSet, lhsNode.Pos(), err, fmt.Sprintf("Error in augmented assignment '%s' for variable '%s'", assignStmt.Tok, varName))
			}
			if _, ok := env.Assign(varName, newVal); !ok {
				// This should ideally not happen if 'existingVal' was successfully fetched.
				return nil, i.formatErrorWithContext(i.FileSet, lhsNode.Pos(), fmt.Errorf("internal error: failed to assign back to variable '%s' after augmented assignment", varName), "")
			}
			return nil, nil
		}

	case *ast.IndexExpr: // Index assignment: arr[idx] = val, map[key] = val
		// TODO: Support augmented assignments for index expressions e.g. arr[0] += 1
		if assignStmt.Tok != token.ASSIGN {
			return nil, i.formatErrorWithContext(i.FileSet, assignStmt.Pos(), fmt.Errorf("augmented assignment (e.g., +=) not yet supported for index expressions"), "Unsupported operation")
		}

		// Evaluate the object being indexed (e.g., array, slice, map)
		collectionObj, err := i.eval(ctx, lhsNode.X, env)
		if err != nil {
			return nil, err
		}

		// Evaluate the index/key
		indexObj, err := i.eval(ctx, lhsNode.Index, env)
		if err != nil {
			return nil, err
		}

		switch col := collectionObj.(type) {
		case *Array:
			idx, ok := indexObj.(*Integer)
			if !ok {
				return nil, i.formatErrorWithContext(i.FileSet, lhsNode.Index.Pos(), fmt.Errorf("array index must be an integer, got %s", indexObj.Type()), "Type error")
			}
			if idx.Value < 0 || idx.Value >= int64(len(col.Elements)) {
				return nil, i.formatErrorWithContext(i.FileSet, lhsNode.Index.Pos(), fmt.Errorf("index out of bounds: %d for array of length %d", idx.Value, len(col.Elements)), "Runtime error")
			}
			col.Elements[idx.Value] = rhsValue // Assign new value
			return nil, nil
		case *Slice:
			idx, ok := indexObj.(*Integer)
			if !ok {
				return nil, i.formatErrorWithContext(i.FileSet, lhsNode.Index.Pos(), fmt.Errorf("slice index must be an integer, got %s", indexObj.Type()), "Type error")
			}
			if idx.Value < 0 || idx.Value >= int64(len(col.Elements)) {
				// Note: Go allows assignment to one past the end if slice has capacity (append semantic).
				// Our simple slices don't have separate capacity yet, so this is strict bounds check.
				return nil, i.formatErrorWithContext(i.FileSet, lhsNode.Index.Pos(), fmt.Errorf("index out of bounds: %d for slice of length %d", idx.Value, len(col.Elements)), "Runtime error")
			}
			col.Elements[idx.Value] = rhsValue // Assign new value
			return nil, nil
		case *Map:
			hashableKey, ok := indexObj.(Hashable)
			if !ok {
				return nil, i.formatErrorWithContext(i.FileSet, lhsNode.Index.Pos(), fmt.Errorf("map key type %s is not hashable for assignment", indexObj.Type()), "Type error")
			}
			hk, err := hashableKey.HashKey()
			if err != nil {
				return nil, i.formatErrorWithContext(i.FileSet, lhsNode.Index.Pos(), fmt.Errorf("error getting hash key for map assignment: %v", err), "Runtime error")
			}
			if col.Pairs == nil { // Should be initialized by literal eval or make
				col.Pairs = make(map[HashKey]MapPair)
			}
			col.Pairs[hk] = MapPair{Key: indexObj, Value: rhsValue}
			return nil, nil
		default:
			return nil, i.formatErrorWithContext(i.FileSet, lhsNode.X.Pos(), fmt.Errorf("cannot assign to index of type %s, not an array, slice, or map", collectionObj.Type()), "Type error")
		}

	default:
		return nil, i.formatErrorWithContext(i.FileSet, lhsExpr.Pos(), fmt.Errorf("unsupported assignment LHS: expected identifier or index expression, got %T", lhsExpr), "")
	}
}

// performAugmentedAssignOperation is a helper for handling operations like x += y.
// It takes the assignment token (e.g., token.ADD_ASSIGN), the existing value of x,
// the value of y (rhsVal), and the position for error reporting.
// It returns the new value for x or an error.
func (i *Interpreter) performAugmentedAssignOperation(assignOpToken token.Token, existingVal, rhsVal Object, pos token.Pos) (Object, error) {
	var binaryOpToken token.Token // The corresponding binary operator (e.g., token.ADD for token.ADD_ASSIGN)

	switch assignOpToken {
	case token.ADD_ASSIGN:
		binaryOpToken = token.ADD
	case token.SUB_ASSIGN:
		binaryOpToken = token.SUB
	case token.MUL_ASSIGN:
		binaryOpToken = token.MUL
	case token.QUO_ASSIGN:
		binaryOpToken = token.QUO
	case token.REM_ASSIGN:
		binaryOpToken = token.REM
	// TODO: Add bitwise augmented assignments if bitwise operators are supported:
	// case token.AND_ASSIGN: binaryOpToken = token.AND
	// case token.OR_ASSIGN: binaryOpToken = token.OR
	// case token.XOR_ASSIGN: binaryOpToken = token.XOR
	// case token.SHL_ASSIGN: binaryOpToken = token.SHL
	// case token.SHR_ASSIGN: binaryOpToken = token.SHR
	// case token.AND_NOT_ASSIGN: binaryOpToken = token.AND_NOT
	default:
		return nil, i.formatErrorWithContext(i.FileSet, pos, fmt.Errorf("unsupported augmented assignment operator %s", assignOpToken), "")
	}

	// Now, use the binaryOpToken to perform the operation, similar to evalBinaryExpr.
	// This assumes that the types are compatible for the operation.
	switch leftTyped := existingVal.(type) {
	case *Integer:
		if rightTyped, ok := rhsVal.(*Integer); ok {
			// Re-use evalIntegerBinaryExpr logic. pos here is the position of the assignment statement.
			return i.evalIntegerBinaryExpr(binaryOpToken, leftTyped, rightTyped, pos)
		}
		return nil, i.formatErrorWithContext(i.FileSet, pos, fmt.Errorf("type mismatch for augmented assignment: existing is INTEGER, right-hand side is %s for op %s", rhsVal.Type(), assignOpToken), "")
	case *String:
		// Only string concatenation (+=) is typically supported for augmented assignment.
		if binaryOpToken == token.ADD {
			if rightTyped, ok := rhsVal.(*String); ok {
				// Re-use evalStringBinaryExpr logic for '+'.
				return i.evalStringBinaryExpr(binaryOpToken, leftTyped, rightTyped, pos)
			}
			return nil, i.formatErrorWithContext(i.FileSet, pos, fmt.Errorf("type mismatch for string concatenation (+=): right-hand side is %s", rhsVal.Type()), "")
		}
		return nil, i.formatErrorWithContext(i.FileSet, pos, fmt.Errorf("unsupported operator %s for augmented string assignment (only += is allowed)", assignOpToken), "")
	// Add cases for other types if they support augmented assignments (e.g., floats, custom types later).
	default:
		return nil, i.formatErrorWithContext(i.FileSet, pos, fmt.Errorf("unsupported type %s for augmented assignment operator %s", existingVal.Type(), assignOpToken), "")
	}
}

func (i *Interpreter) evalIfStmt(ctx context.Context, ifStmt *ast.IfStmt, env *Environment) (Object, error) {
	condition, err := i.eval(ctx, ifStmt.Cond, env)
	if err != nil {
		return nil, err
	}

	boolCond, ok := condition.(*Boolean)
	if !ok {
		return nil, i.formatErrorWithContext(i.FileSet, ifStmt.Cond.Pos(),
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
		return nil, i.formatErrorWithContext(i.FileSet, node.Pos(), fmt.Errorf("unsupported type for negation: %s", operand.Type()), "")
	case token.NOT:
		switch operand {
		case TRUE:
			return FALSE, nil
		case FALSE:
			return TRUE, nil
		default:
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Pos(), fmt.Errorf("unsupported type for logical NOT: %s", operand.Type()), "")
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
			return nil, i.formatErrorWithContext(i.activeFileSet, xNode.Pos(), fmt.Errorf("cannot take address of identifier '%s' (type %s), not a struct instance", xNode.Name, operand.Type()), "")
		case *ast.CompositeLit:
			// If & is applied to a composite literal, e.g., &Point{X:1}.
			// The `operand` variable here is the result of i.eval(ctx, node.X, env),
			// which means the composite literal (node.X) has already been evaluated.
			// So, `operand` should be the StructInstance.
			if _, isStruct := operand.(*StructInstance); isStruct {
				return operand, nil // Return the struct instance from the composite literal.
			}
			// This case should ideally not be reached if composite lit eval fails or returns non-struct.
			return nil, i.formatErrorWithContext(i.activeFileSet, xNode.Pos(), fmt.Errorf("operator & on composite literal did not yield a struct instance, got %s", operand.Type()), "")
		default:
			// Other cases like &MyFunctionCall() or &someSelector.field might need more complex handling
			// if they are to be supported. For now, restrict to identifiers and composite literals.
			return nil, i.formatErrorWithContext(i.activeFileSet, node.Pos(), fmt.Errorf("operator & only supported on identifiers or composite literals for now, got %T", node.X), "")
		}
	default:
		return nil, i.formatErrorWithContext(i.activeFileSet, node.Pos(), fmt.Errorf("unsupported unary operator: %s", node.Op), "")
	}
}
