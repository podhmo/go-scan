// Package internal contains the core logic for the convert-define tool.
package internal

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"log/slog"
	"os"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/convert/model"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/evaluator"
	"github.com/podhmo/go-scan/minigo/object"
	"github.com/podhmo/go-scan/scanner"
)

const definePkgPath = "github.com/podhmo/go-scan/examples/convert/define"

// Runner manages the execution of a minigo script for conversion definitions.
type Runner struct {
	interp *minigo.Interpreter
	Info   *model.ParsedInfo
}

// NewRunner creates a new interpreter runner.
func NewRunner(scannerOpts ...goscan.ScannerOption) (*Runner, error) {
	interp, err := minigo.NewInterpreter(minigo.WithScannerOptions(scannerOpts...))
	if err != nil {
		return nil, fmt.Errorf("creating minigo interpreter: %w", err)
	}

	r := &Runner{
		interp: interp,
		Info: &model.ParsedInfo{
			Imports:           make(map[string]string),
			Structs:           make(map[string]*model.StructInfo),
			ConversionPairs:   []model.ConversionPair{},
			GlobalRules:       []model.TypeRule{},
			ProcessedPackages: make(map[string]bool),
		},
	}

	// Register special forms with their fully qualified names.
	r.interp.RegisterSpecial(fmt.Sprintf("%s.Convert", definePkgPath), r.handleConvert)
	r.interp.RegisterSpecial(fmt.Sprintf("%s.Rule", definePkgPath), r.handleRule)
	r.interp.RegisterSpecial(fmt.Sprintf("%s.Mapping", definePkgPath), r.handleMapping)

	return r, nil
}

// Run loads and executes the definition script.
func (r *Runner) Run(ctx context.Context, filename string) error {
	slog.InfoContext(ctx, "Executing define script", "filename", filename)
	source, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("reading define file %q: %w", filename, err)
	}

	if err := r.interp.LoadFile(filename, source); err != nil {
		return fmt.Errorf("loading define file into interpreter: %w", err)
	}

	if _, err := r.interp.Eval(ctx); err != nil {
		return fmt.Errorf("evaluating define file: %w", err)
	}

	return nil
}

func (r *Runner) handleConvert(e *evaluator.Evaluator, fscope *object.FileScope, pos token.Pos, args []ast.Expr) object.Object {
	slog.Info("handleConvert called", "arg_count", len(args))
	if len(args) != 3 {
		return e.NewError(pos, "Convert() expects 3 arguments, got %d", len(args))
	}

	// Arg 0: Source Type
	srcType, err := r.resolveTypeFromExpr(e, fscope, args[0])
	if err != nil {
		return e.NewError(pos, "could not resolve source type: %v", err)
	}
	if srcType.Struct == nil {
		return e.NewError(pos, "source type %s is not a struct", srcType.Name)
	}

	// Arg 1: Destination Type
	dstType, err := r.resolveTypeFromExpr(e, fscope, args[1])
	if err != nil {
		return e.NewError(pos, "could not resolve destination type: %v", err)
	}

	slog.Info("found conversion pair", "src", srcType.Name, "dst", dstType.Name)
	for _, field := range srcType.Struct.Fields {
		slog.Info("  - src field", "name", field.Name, "type", field.Type.String())
	}

	// TODO: Store this information in r.Info

	// Arg 2: Mapping
	// TODO: Process the mapping argument.

	return object.NIL
}

// resolveTypeFromExpr takes an AST expression (like `pkg.MyType{}` or `pkg.MyType`)
// and resolves it to a scanner.TypeInfo.
func (r *Runner) resolveTypeFromExpr(e *evaluator.Evaluator, fscope *object.FileScope, expr ast.Expr) (*scanner.TypeInfo, error) {
	// Unwrap composite literals to get to the type expression, e.g., `pkg.MyType` from `pkg.MyType{}`.
	if cl, ok := expr.(*ast.CompositeLit); ok {
		expr = cl.Type
	}

	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return nil, fmt.Errorf("expected a selector expression (pkg.Type), but got %T", expr)
	}

	pkgIdent, ok := selector.X.(*ast.Ident)
	if !ok {
		return nil, fmt.Errorf("selector must be on a package identifier")
	}
	typeName := selector.Sel.Name

	pkgPath, ok := fscope.Aliases[pkgIdent.Name]
	if !ok {
		return nil, fmt.Errorf("package alias %q not found in imports", pkgIdent.Name)
	}

	// Use the scanner to get the package info
	pkgInfo, err := e.Scanner().ScanPackageByImport(context.Background(), pkgPath)
	if err != nil {
		return nil, fmt.Errorf("could not scan package %q: %w", pkgPath, err)
	}

	// Find the type within the package
	for _, t := range pkgInfo.Types {
		if t.Name == typeName {
			return t, nil
		}
	}

	return nil, fmt.Errorf("type %q not found in package %q", typeName, pkgPath)
}

func (r *Runner) handleRule(e *evaluator.Evaluator, fscope *object.FileScope, pos token.Pos, args []ast.Expr) object.Object {
	slog.Info("handleRule called", "arg_count", len(args))
	if len(args) != 1 {
		return e.NewError(pos, "Rule() expects 1 argument, got %d", len(args))
	}

	funcExpr, ok := args[0].(*ast.SelectorExpr)
	if !ok {
		return e.NewError(pos, "argument to Rule() must be a function selector (e.g., pkg.Func)")
	}

	pkgIdent, ok := funcExpr.X.(*ast.Ident)
	if !ok {
		return e.NewError(pos, "receiver of function selector must be a package identifier")
	}
	funcName := funcExpr.Sel.Name

	pkgPath, ok := fscope.Aliases[pkgIdent.Name]
	if !ok {
		return e.NewError(pos, "package alias %q not found in imports", pkgIdent.Name)
	}

	slog.Info("resolving rule function", "pkg", pkgPath, "func", funcName)

	// Use the scanner to get the package info
	ctx := context.Background()
	pkgInfo, err := e.Scanner().ScanPackageByImport(ctx, pkgPath)
	if err != nil {
		return e.NewError(pos, "could not scan package %q: %v", pkgPath, err)
	}

	var foundFunc *scanner.FunctionInfo
	for _, f := range pkgInfo.Functions {
		if f.Name == funcName {
			foundFunc = f
			break
		}
	}

	if foundFunc == nil {
		return e.NewError(pos, "function %q not found in package %q", funcName, pkgPath)
	}

	// Basic validation: must be a conversion function with 1 input and 1 output.
	if len(foundFunc.Parameters) != 1 || len(foundFunc.Results) != 1 {
		return e.NewError(pos, "rule function %s must have signature func(T) S", foundFunc.Name)
	}

	srcField := foundFunc.Parameters[0]
	dstField := foundFunc.Results[0]

	srcTypeInfo, err := srcField.Type.Resolve(ctx)
	if err != nil {
		return e.NewError(pos, "could not resolve source type for rule: %v", err)
	}

	dstTypeInfo, err := dstField.Type.Resolve(ctx)
	if err != nil {
		return e.NewError(pos, "could not resolve destination type for rule: %v", err)
	}

	// This can happen for built-in types like 'string' which don't have a full TypeInfo definition.
	if srcTypeInfo == nil && !srcField.Type.IsBuiltin {
		return e.NewError(pos, "could not resolve source type definition for rule: %s", srcField.Type.String())
	}
	if dstTypeInfo == nil && !dstField.Type.IsBuiltin {
		return e.NewError(pos, "could not resolve destination type definition for rule: %s", dstField.Type.String())
	}

	slog.Info("found rule", "src", srcField.Type.String(), "dst", dstField.Type.String())

	// e.g. "convutil.TimeToString"
	usingFunc := fmt.Sprintf("%s.%s", pkgIdent.Name, funcName)

	rule := model.TypeRule{
		SrcTypeName: srcField.Type.String(),
		DstTypeName: dstField.Type.String(),
		SrcTypeInfo: srcTypeInfo,
		DstTypeInfo: dstTypeInfo,
		UsingFunc:   usingFunc,
	}
	r.Info.GlobalRules = append(r.Info.GlobalRules, rule)

	// ensure the import is registered
	if _, ok := r.Info.Imports[pkgIdent.Name]; !ok {
		r.Info.Imports[pkgIdent.Name] = pkgPath
	}

	return object.NIL
}

func (r *Runner) handleMapping(e *evaluator.Evaluator, fscope *object.FileScope, pos token.Pos, args []ast.Expr) object.Object {
	slog.Info("handleMapping called", "arg_count", len(args))
	// Step 4 will implement the logic here.
	// This handler needs to return a value that can be passed to `Convert`.
	// For now, we return a special marker or just NIL.
	return object.NIL
}
