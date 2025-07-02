package main

import (
	"bufio"
	"context" // Added for go-scan context
	"errors"
	"fmt"
	"go/ast"
	"go/parser" // Will be replaced by go-scan
	"go/token"
	"os"
	"path/filepath" // Added for go-scan
	"strconv"
	"strings"

	"github.com/podhmo/go-scan" // Using top-level go-scan
	// "github.com/podhmo/go-scan/scanner" // No longer directly needed for minigo's use of ConstantInfo
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
}

func NewInterpreter() *Interpreter {
	env := NewEnvironment(nil)
	fset := token.NewFileSet()

	i := &Interpreter{
		globalEnv:        env,
		FileSet:          fset,
		scn:              nil, // Will be initialized in LoadAndRun
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
func (i *Interpreter) LoadAndRun(filename string, entryPoint string) error {
	absFilePath, err := filepath.Abs(filename)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %s: %w", filename, err)
	}
	i.currentFileDir = filepath.Dir(absFilePath)

	if i.scn == nil { // Only initialize scanner if it hasn't been set (e.g., by a test)
		gs, errGs := goscan.New(i.currentFileDir)
		if errGs != nil {
			return fmt.Errorf("failed to create go-scan scanner for dir %s: %w", i.currentFileDir, errGs)
		}
		i.scn = gs
		if i.FileSet == nil { // If FileSet wasn't also preset by test, use the one from new scanner
			// Ensure gs.Fset() is not nil before assigning
			if gs.Fset() == nil {
				// This case should ideally not happen if goscan.New succeeds and returns a valid scanner
				return fmt.Errorf("internal error: new scanner created by goscan.New for dir %s has a nil FileSet", i.currentFileDir)
			}
			i.FileSet = gs.Fset()
		}
	} else {
		// If i.scn was preset (by test), ensure i.FileSet is also valid.
		// Tests should ideally set both i.scn and i.FileSet.
		if i.FileSet == nil {
			if i.scn.Fset() != nil {
				i.FileSet = i.scn.Fset()
			} else {
				// Fallback if preset scn has no Fset (should not happen with valid goscan.Scanner)
				i.FileSet = token.NewFileSet()
				// It might be better to error out if a pre-set scanner has no FileSet
				// return fmt.Errorf("internal error: pre-set interpreter.scn has a nil FileSet")
				fmt.Fprintf(os.Stderr, "Warning: interpreter.scn was set, but its FileSet was nil. Created new FileSet for interpreter.\n")
			}
		}
	}

	if i.FileSet == nil {
		// This is a safeguard. If after all logic i.FileSet is still nil, it's an issue.
		return fmt.Errorf("internal error: Interpreter.FileSet is nil before scanning files")
	}


	// Parse the main script file directly using go/parser.
	mainFileAst, parseErr := parser.ParseFile(i.FileSet, absFilePath, nil, parser.ParseComments)
	if parseErr != nil {
		return formatErrorWithContext(i.FileSet, token.NoPos, parseErr, fmt.Sprintf("Error parsing main script file %s: %v", filename, parseErr))
	}

	// Process import declarations from the parsed AST to populate importAliasMap
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
						// Blank import, record for completeness but it won't be usable for selection.
						// Or, decide to error here if blank imports are not to be "recorded".
						// For now, let's just skip adding it to alias map as it cannot be selected.
						// Alternatively, map it to a special value or handle in evalSelectorExpr.
						// Plan says: ブランクインポートは...`evalSelectorExpr` で使おうとしても解決できない...ことを確認
						// So, we can add it, and evalSelectorExpr won't find symbols via it.
						// Let's add it with the path for now, so we know it was "seen".
						// i.importAliasMap["_"] = importPath // This is problematic if multiple blank imports
						// Let's decide to disallow blank and dot imports for now as per plan.
						return formatErrorWithContext(i.FileSet, impSpec.Name.Pos(), errors.New("blank imports are not supported"), "")
					}
					if localName == "." {
						return formatErrorWithContext(i.FileSet, impSpec.Name.Pos(), errors.New("dot imports are not supported"), "")
					}
				} else {
					// No explicit alias, use the base of the import path
					// e.g. "path/to/pkg" -> localName is "pkg"
					// This can be tricky if path is complex e.g. "my.repo/pkg/v2" -> "v2"
					// Go's default is the package name declared in the imported package's files,
					// which we don't know until we scan it.
					// For simplicity, go-scan itself uses filepath.Base for default alias if needed.
					// Let's use filepath.Base on the import path.
					localName = filepath.Base(importPath)
				}

				if existingPath, ok := i.importAliasMap[localName]; ok && existingPath != importPath {
					return formatErrorWithContext(i.FileSet, impSpec.Pos(), fmt.Errorf("import alias/name %q already used for %q, cannot reuse for %q", localName, existingPath, importPath), "")
				}
				i.importAliasMap[localName] = importPath
			}
	}

	// Process other top-level declarations (functions and global vars) from the AST
	// Store all function declarations first
	for _, declNode := range mainFileAst.Decls {
		if fnDecl, ok := declNode.(*ast.FuncDecl); ok {
			_, evalErr := i.evalFuncDecl(fnDecl, i.globalEnv)
			if evalErr != nil {
				return evalErr // evalFuncDecl should use i.FileSet for errors
			}
		}
	}
	// Then evaluate global variable declarations
	for _, declNode := range mainFileAst.Decls {
		if genDecl, ok := declNode.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			tempDeclStmt := &ast.DeclStmt{Decl: genDecl}
			_, evalErr := i.eval(tempDeclStmt, i.globalEnv)
				if evalErr != nil {
					// formatErrorWithContext will use i.FileSet
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
	result, errApply := i.applyUserDefinedFunction(userEntryFunc, []Object{}, token.NoPos)
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


func (i *Interpreter) applyUserDefinedFunction(fn *UserDefinedFunction, args []Object, callPos token.Pos) (Object, error) {
	if len(args) != len(fn.Parameters) {
		errMsg := fmt.Sprintf("wrong number of arguments for function %s: expected %d, got %d", fn.Name, len(fn.Parameters), len(args))
		return nil, formatErrorWithContext(i.FileSet, callPos, errors.New(errMsg), "Function call error")
	}

	funcEnv := NewEnvironment(fn.Env) // Closure: fn.Env is the lexical scope

	for idx, paramIdent := range fn.Parameters {
		funcEnv.Define(paramIdent.Name, args[idx])
	}

	evaluated, errEval := i.evalBlockStatement(fn.Body, funcEnv)
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

func (i *Interpreter) eval(node ast.Node, env *Environment) (Object, error) {
	switch n := node.(type) {
	case *ast.File:
		var result Object
		var err error
		for _, decl := range n.Decls {
			if fnDecl, ok := decl.(*ast.FuncDecl); ok && fnDecl.Name.Name == "main" {
				result, err = i.evalBlockStatement(fnDecl.Body, env)
				if err != nil {
					return nil, err
				}
			}
		}
		return result, nil

	case *ast.BlockStmt:
		return i.evalBlockStatement(n, env)

	case *ast.ExprStmt:
		return i.eval(n.X, env)

	case *ast.Ident:
		return evalIdentifier(n, env, i.FileSet)

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
		return i.evalDeclStmt(n, env)

	case *ast.BinaryExpr:
		return i.evalBinaryExpr(n, env)

	case *ast.UnaryExpr:
		return i.evalUnaryExpr(n, env)

	case *ast.ParenExpr:
		return i.eval(n.X, env)

	case *ast.IfStmt:
		return i.evalIfStmt(n, env)

	case *ast.AssignStmt:
		return i.evalAssignStmt(n, env)

	case *ast.CallExpr:
		return i.evalCallExpr(n, env)

	case *ast.SelectorExpr:
		return i.evalSelectorExpr(n, env)

	case *ast.ReturnStmt:
		return i.evalReturnStmt(n, env)

	case *ast.FuncDecl:
		return i.evalFuncDecl(n, env)

	case *ast.FuncLit:
		return i.evalFuncLit(n, env)

	case *ast.ForStmt:
		return i.evalForStmt(n, env)

	case *ast.BranchStmt:
		return i.evalBranchStmt(n, env)

	case *ast.LabeledStmt:
		// Labels are handled by specific statements that use them (like break/continue).
		// For other statements, the label itself doesn't change evaluation.
		// We just evaluate the statement the label is attached to.
		// If a break/continue needs this label, its ast.BranchStmt.Label will be non-nil.
		return i.eval(n.Stmt, env)

	default:
		return nil, formatErrorWithContext(i.FileSet, n.Pos(), fmt.Errorf("unsupported AST node type: %T", n), fmt.Sprintf("Unsupported AST node value: %+v", n))
	}
}

func (i *Interpreter) evalBranchStmt(stmt *ast.BranchStmt, env *Environment) (Object, error) {
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

func (i *Interpreter) evalForStmt(stmt *ast.ForStmt, env *Environment) (Object, error) {
	// For loops create a new scope for their initialization, condition, post, and body.
	loopEnv := NewEnvironment(env)

	// 1. Initialization
	if stmt.Init != nil {
		if _, err := i.eval(stmt.Init, loopEnv); err != nil {
			return nil, err
		}
	}

	for {
		// 2. Condition
		if stmt.Cond != nil {
			condition, err := i.eval(stmt.Cond, loopEnv)
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
		bodyResult, err := i.evalBlockStatement(stmt.Body, loopEnv)
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
				if _, postErr := i.eval(stmt.Post, loopEnv); postErr != nil {
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
			if _, err := i.eval(stmt.Post, loopEnv); err != nil {
				return nil, err
			}
		}
	}

	return NULL, nil // For statement itself doesn't produce a value
}

func (i *Interpreter) evalSelectorExpr(node *ast.SelectorExpr, env *Environment) (Object, error) {
	// node.X might be an identifier (package name) or another expression.
	// For now, we only support simple package.Selector
	xIdent, ok := node.X.(*ast.Ident)
	if !ok {
		// TODO: Handle more complex selectors like structField.AnotherField or funcCall().Field
		return nil, formatErrorWithContext(i.FileSet, node.X.Pos(), fmt.Errorf("selector base X must be an identifier (package name), got %T", node.X), "Unsupported selector expression")
	}

	localPkgName := xIdent.Name // This is the alias or local name used in minigo script
	symbolName := node.Sel.Name
	qualifiedNameInEnv := localPkgName + "." + symbolName // How it's stored in minigo's env

	// Check if the symbol is already in the environment (e.g. from a previous import of this pkg)
	if val, ok := env.Get(qualifiedNameInEnv); ok {
		return val, nil
	}

	// If not in env, check if we have an import path for this localPkgName
	importPath, knownAlias := i.importAliasMap[localPkgName]
	if !knownAlias {
		// If localPkgName is not in importAliasMap, it might be a built-in "package" like "fmt"
		// or an error (undeclared package or variable).
		// For now, assume builtins are directly in globalEnv, not via selector.
		// So, if not in importAliasMap, it's an undefined package alias.
		return nil, formatErrorWithContext(i.FileSet, xIdent.Pos(), fmt.Errorf("undefined package alias/name: %s", localPkgName), "Import error")
	}

	// We have an importPath. Now check if this importPath has already been processed (scanned and constants loaded).
	if _, alreadyImported := i.importedPackages[importPath]; !alreadyImported {
		if i.scn == nil {
			return nil, formatErrorWithContext(i.FileSet, node.X.Pos(), errors.New("go-scan scanner not initialized in interpreter"), "Internal error")
		}

		ctx := context.Background()
		pkgInfo, err := i.scn.ScanPackageByImport(ctx, importPath)
		if err != nil {
			return nil, formatErrorWithContext(i.FileSet, xIdent.Pos(), fmt.Errorf("package %q (aliased as %q) not found or failed to scan: %w", importPath, localPkgName, err), "Import error")
		}

		// Successfully scanned. Populate environment with its exported constants.
		// Constants will be stored in env as "localPkgName.ConstantName".
		for _, c := range pkgInfo.Constants { // c is of type *scanner.ConstantInfo
			if !c.IsExported { // Corrected: IsExported is a field, not a method
				continue
			}
			var constObj Object
			if c.Type != nil { // Ensure type information is present
				switch c.Type.Name {
				case "int", "int64", "int32", "uint", "uint64", "uint32", "rune", "byte":
					val, err := parseInt64(c.Value)
					if err == nil {
						constObj = &Integer{Value: val}
					} else {
						fmt.Fprintf(os.Stderr, "Warning: Could not parse external const integer %s.%s from package %s (value: %s): %v\n", c.Name, localPkgName, importPath, c.Value, err)
					}
				case "string":
					unquotedVal, err := strconv.Unquote(c.Value)
					if err == nil {
						constObj = &String{Value: unquotedVal}
					} else {
						fmt.Fprintf(os.Stderr, "Warning: Could not unquote external const string %s.%s from package %s (value: %s): %v\n", c.Name, localPkgName, importPath, c.Value, err)
					}
				case "bool":
					switch c.Value {
					case "true":
						constObj = TRUE
					case "false":
						constObj = FALSE
					default:
						fmt.Fprintf(os.Stderr, "Warning: Could not parse external const bool %s.%s from package %s (value: %s)\n", c.Name, localPkgName, importPath, c.Value)
					}
				default:
					fmt.Fprintf(os.Stderr, "Warning: Unsupported external const type %s for %s.%s from package %s\n", c.Type.Name, c.Name, localPkgName, importPath)
				}
			} else {
				fmt.Fprintf(os.Stderr, "Warning: External const %s.%s from package %s has no type info from go-scan, cannot determine type for value: %s\n", c.Name, localPkgName, importPath, c.Value)
			}

			if constObj != nil {
				env.Define(localPkgName+"."+c.Name, constObj) // Use localPkgName for env key
			}
		}
		i.importedPackages[importPath] = struct{}{} // Mark the importPath as processed
	}

	// After attempting import and processing, try getting the symbol again
	if val, ok := env.Get(qualifiedNameInEnv); ok {
		return val, nil
	}

	// If still not found.
	return nil, formatErrorWithContext(i.FileSet, node.Pos(), fmt.Errorf("undefined selector: %s (package %s, path %s)", qualifiedNameInEnv, localPkgName, importPath), "")
}

func (i *Interpreter) evalBlockStatement(block *ast.BlockStmt, env *Environment) (Object, error) {
	var result Object
	var err error

	for _, stmt := range block.List {
		result, err = i.eval(stmt, env)
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

func (i *Interpreter) evalFuncDecl(fd *ast.FuncDecl, env *Environment) (Object, error) {
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
	}

	if fd.Name != nil && fd.Name.Name != "" {
		env.Define(fd.Name.Name, function)
		return nil, nil
	}
	return nil, formatErrorWithContext(i.FileSet, fd.Pos(), fmt.Errorf("function declaration must have a name"), "")
}

func (i *Interpreter) evalFuncLit(fl *ast.FuncLit, env *Environment) (Object, error) {
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
	}, nil
}

func (i *Interpreter) evalReturnStmt(rs *ast.ReturnStmt, env *Environment) (Object, error) {
	if len(rs.Results) == 0 {
		return &ReturnValue{Value: NULL}, nil
	}

	if len(rs.Results) > 1 {
		return nil, formatErrorWithContext(i.FileSet, rs.Pos(), fmt.Errorf("multiple return values not supported"), "")
	}

	val, err := i.eval(rs.Results[0], env)
	if err != nil {
		return nil, err
	}
	return &ReturnValue{Value: val}, nil
}

func (i *Interpreter) evalDeclStmt(declStmt *ast.DeclStmt, env *Environment) (Object, error) {
	genDecl, ok := declStmt.Decl.(*ast.GenDecl)
	if !ok {
		return nil, formatErrorWithContext(i.FileSet, declStmt.Pos(), fmt.Errorf("unsupported declaration type: %T", declStmt.Decl), "")
	}

	if genDecl.Tok != token.VAR {
		return nil, formatErrorWithContext(i.FileSet, genDecl.Pos(), fmt.Errorf("unsupported declaration token: %s (expected VAR)", genDecl.Tok), "")
	}

	for _, spec := range genDecl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			return nil, formatErrorWithContext(i.FileSet, spec.Pos(), fmt.Errorf("unsupported spec type in var declaration: %T", spec), "")
		}

		for idx, nameIdent := range valueSpec.Names {
			varName := nameIdent.Name
			if len(valueSpec.Values) > idx {
				val, err := i.eval(valueSpec.Values[idx], env)
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

func (i *Interpreter) evalBinaryExpr(node *ast.BinaryExpr, env *Environment) (Object, error) {
	left, err := i.eval(node.X, env)
	if err != nil {
		return nil, err
	}
	right, err := i.eval(node.Y, env)
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

func (i *Interpreter) evalCallExpr(node *ast.CallExpr, env *Environment) (Object, error) {
	funcObj, err := i.eval(node.Fun, env)
	if err != nil {
		return nil, err
	}

	args := make([]Object, len(node.Args))
	for idx, argExpr := range node.Args {
		argVal, err := i.eval(argExpr, env)
		if err != nil {
			return nil, err
		}
		args[idx] = argVal
	}

	switch fn := funcObj.(type) {
	case *BuiltinFunction:
		return fn.Fn(env, args...)
	case *UserDefinedFunction:
		return i.applyUserDefinedFunction(fn, args, node.Fun.Pos())
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

func (i *Interpreter) evalAssignStmt(assignStmt *ast.AssignStmt, env *Environment) (Object, error) {
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

	val, err := i.eval(assignStmt.Rhs[0], env)
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

func (i *Interpreter) evalIfStmt(ifStmt *ast.IfStmt, env *Environment) (Object, error) {
	condition, err := i.eval(ifStmt.Cond, env)
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
		return i.evalBlockStatement(ifStmt.Body, ifBodyEnv)
	} else if ifStmt.Else != nil {
		// Else block also creates a new scope if it's a block statement
		// If it's another IfStmt (else if), that IfStmt will handle its own scope.
		switch elseNode := ifStmt.Else.(type) {
		case *ast.BlockStmt:
			elseBodyEnv := NewEnvironment(env)
			return i.evalBlockStatement(elseNode, elseBodyEnv)
		case *ast.IfStmt: // else if
			return i.eval(elseNode, env) // The nested if will handle its own new scope creation
		default: // Should not happen with a valid Go AST for if-else
			return i.eval(ifStmt.Else, env)
		}
	}
	return nil, nil
}

func (i *Interpreter) evalUnaryExpr(node *ast.UnaryExpr, env *Environment) (Object, error) {
	operand, err := i.eval(node.X, env)
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
