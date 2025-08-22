// Package internal contains the core logic for the convert-define tool.
package internal

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"log/slog"
	"os"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/convert/model"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/evaluator"
	"github.com/podhmo/go-scan/minigo/object"
	"github.com/podhmo/go-scan/scanner"
)

const definePkgPath = "github.com/podhmo/go-scan/examples/convert-define/define"

// Runner manages the execution of a minigo script for conversion definitions.
type Runner struct {
	interp *minigo.Interpreter
	Info   *model.ParsedInfo
}

// NewRunner creates a new interpreter runner.
func NewRunner(scannerOpts ...goscan.ScannerOption) (*Runner, error) {
	// Pass scanner options to create a scanner.
	scanner, err := goscan.New(scannerOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating scanner: %w", err)
	}
	// Pass the scanner to the interpreter.
	interp, err := minigo.NewInterpreter(scanner)
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

	return r, nil
}

// Scanner returns the underlying scanner instance.
func (r *Runner) Scanner() *goscan.Scanner {
	return r.interp.Scanner()
}

// PackageName returns the package name of the loaded define file.
// It assumes the first loaded file's package is the one we want.
func (r *Runner) PackageName() string {
	files := r.interp.Files()
	if len(files) == 0 {
		return ""
	}
	return files[0].AST.Name.Name
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
	if len(args) != 1 {
		return e.NewError(pos, "Convert() expects 1 argument (the mapping function), got %d", len(args))
	}
	fnLit, ok := args[0].(*ast.FuncLit)
	if !ok {
		return e.NewError(pos, "argument to Convert() must be a function literal")
	}
	if fnLit.Type == nil || fnLit.Type.Params == nil || len(fnLit.Type.Params.List) != 3 {
		return e.NewError(pos, "mapping function must have the signature func(c *Config, dst *DstType, src *SrcType)")
	}

	// Infer types from function signature: func(c *Config, dst *Dst, src *Src)
	// Param 1 is dst, Param 2 is src (after skipping config)
	dstTypeExpr := fnLit.Type.Params.List[1].Type
	srcTypeExpr := fnLit.Type.Params.List[2].Type

	// The types will be *ast.StarExpr, we need to get the underlying type expr.
	if star, ok := dstTypeExpr.(*ast.StarExpr); ok {
		dstTypeExpr = star.X
	} else {
		return e.NewError(pos, "destination type in mapping function must be a pointer")
	}
	if star, ok := srcTypeExpr.(*ast.StarExpr); ok {
		srcTypeExpr = star.X
	} else {
		return e.NewError(pos, "source type in mapping function must be a pointer")
	}

	srcType, err := r.resolveTypeFromExpr(e, fscope, srcTypeExpr)
	if err != nil {
		return e.NewError(pos, "could not resolve source type from mapping function: %v", err)
	}
	r.ensureStructInfo(srcType)

	dstType, err := r.resolveTypeFromExpr(e, fscope, dstTypeExpr)
	if err != nil {
		return e.NewError(pos, "could not resolve destination type from mapping function: %v", err)
	}
	r.ensureStructInfo(dstType)

	slog.Info("found conversion pair", "src", srcType.Name, "dst", dstType.Name)

	pair := model.ConversionPair{
		SrcTypeName: srcType.Name,
		DstTypeName: dstType.Name,
		SrcTypeInfo: srcType,
		DstTypeInfo: dstType,
	}

	// Walk the function body to find Map/Convert/Compute calls
	walker := &mappingWalker{
		evaluator: e,
		fscope:    fscope,
		pair:      &pair,
		srcInfo:   r.Info.Structs[srcType.Name],
	}

	ast.Walk(walker, fnLit.Body)
	if walker.err != nil {
		return e.NewError(pos, "error while parsing mapping function: %v", walker.err)
	}

	r.Info.ConversionPairs = append(r.Info.ConversionPairs, pair)
	slog.Info("registered conversion pair", "src", pair.SrcTypeName, "dst", pair.DstTypeName)

	return object.NIL
}

// ensureStructInfo checks if a model.StructInfo exists for the given scanner.TypeInfo,
// creating it from the scanner info if it doesn't.
func (r *Runner) ensureStructInfo(typeInfo *scanner.TypeInfo) {
	if _, exists := r.Info.Structs[typeInfo.Name]; exists {
		return
	}
	if typeInfo.Struct == nil {
		return
	}

	slog.Debug("creating new model.StructInfo", "name", typeInfo.Name)
	structInfo := &model.StructInfo{
		Name: typeInfo.Name,
		Type: typeInfo,
	}
	for _, f := range typeInfo.Struct.Fields {
		fieldInfo := model.FieldInfo{
			Name:         f.Name,
			OriginalName: f.Name,
			FieldType:    f.Type,
			ParentStruct: structInfo,
		}
		structInfo.Fields = append(structInfo.Fields, fieldInfo)
	}
	r.Info.Structs[typeInfo.Name] = structInfo
}

// resolveTypeFromExpr resolves a type expression to a scanner.TypeInfo.
func (r *Runner) resolveTypeFromExpr(e *evaluator.Evaluator, fscope *object.FileScope, expr ast.Expr) (*scanner.TypeInfo, error) {
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

	pkgInfo, err := e.Scanner().ScanPackageByImport(context.Background(), pkgPath)
	if err != nil {
		return nil, fmt.Errorf("could not scan package %q: %w", pkgPath, err)
	}

	for _, t := range pkgInfo.Types {
		if t.Name == typeName {
			return t, nil
		}
	}
	return nil, fmt.Errorf("type %q not found in package %q", typeName, pkgPath)
}

func (r *Runner) handleRule(e *evaluator.Evaluator, fscope *object.FileScope, pos token.Pos, args []ast.Expr) object.Object {
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
	// A valid rule function has at least one parameter and exactly one result.
	// The source type is the last parameter.
	if len(foundFunc.Parameters) == 0 || len(foundFunc.Results) != 1 {
		return e.NewError(pos, "rule function %s must have at least one parameter and exactly one result", foundFunc.Name)
	}

	srcField := foundFunc.Parameters[len(foundFunc.Parameters)-1]
	dstField := foundFunc.Results[0]
	srcTypeInfo, err := srcField.Type.Resolve(ctx)
	if err != nil {
		return e.NewError(pos, "could not resolve source type for rule: %v", err)
	}
	dstTypeInfo, err := dstField.Type.Resolve(ctx)
	if err != nil {
		return e.NewError(pos, "could not resolve destination type for rule: %v", err)
	}
	if srcTypeInfo == nil && !srcField.Type.IsBuiltin {
		return e.NewError(pos, "could not resolve source type definition for rule: %s", srcField.Type.String())
	}
	if dstTypeInfo == nil && !dstField.Type.IsBuiltin {
		return e.NewError(pos, "could not resolve destination type definition for rule: %s", dstField.Type.String())
	}

	usingFunc := fmt.Sprintf("%s.%s", pkgIdent.Name, funcName)
	rule := model.TypeRule{
		SrcTypeName: srcField.Type.String(),
		DstTypeName: dstField.Type.String(),
		SrcTypeInfo: srcTypeInfo,
		DstTypeInfo: dstTypeInfo,
		UsingFunc:   usingFunc,
	}
	r.Info.GlobalRules = append(r.Info.GlobalRules, rule)
	if _, ok := r.Info.Imports[pkgIdent.Name]; !ok {
		r.Info.Imports[pkgIdent.Name] = pkgPath
	}
	return object.NIL
}

type mappingWalker struct {
	evaluator *evaluator.Evaluator
	fscope    *object.FileScope
	pair      *model.ConversionPair
	srcInfo   *model.StructInfo
	err       error
}

func (w *mappingWalker) Visit(node ast.Node) ast.Visitor {
	if w.err != nil || node == nil {
		return nil
	}
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return w
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return w
	}

	switch sel.Sel.Name {
	case "Map":
		w.err = w.parseMapCall(call)
	case "Convert":
		w.err = w.parseConvertCall(call)
	case "Compute":
		w.err = w.parseComputeCall(call)
	}
	return w
}

func (w *mappingWalker) parseMapCall(call *ast.CallExpr) error {
	if len(call.Args) != 2 {
		return fmt.Errorf("c.Map() expects 2 arguments, got %d", len(call.Args))
	}
	dst, err := w.exprToString(call.Args[0])
	if err != nil {
		return fmt.Errorf("could not parse dst in c.Map(): %w", err)
	}
	src, err := w.exprToString(call.Args[1])
	if err != nil {
		return fmt.Errorf("could not parse src in c.Map(): %w", err)
	}

	dstName := strings.SplitN(dst, ".", 2)[1]
	srcName := strings.SplitN(src, ".", 2)[1]

	return w.setFieldTag(srcName, dstName, "")
}

func (w *mappingWalker) parseConvertCall(call *ast.CallExpr) error {
	if len(call.Args) != 3 {
		return fmt.Errorf("c.Convert() expects 3 arguments, got %d", len(call.Args))
	}
	dst, err := w.exprToString(call.Args[0])
	if err != nil {
		return fmt.Errorf("could not parse dst in c.Convert(): %w", err)
	}
	src, err := w.exprToString(call.Args[1])
	if err != nil {
		return fmt.Errorf("could not parse src in c.Convert(): %w", err)
	}
	converter, err := w.exprToString(call.Args[2])
	if err != nil {
		return fmt.Errorf("could not parse converter in c.Convert(): %w", err)
	}

	dstName := strings.SplitN(dst, ".", 2)[1]
	srcName := strings.SplitN(src, ".", 2)[1]

	return w.setFieldTag(srcName, dstName, converter)
}

func (w *mappingWalker) setFieldTag(srcFieldName, dstFieldName, converter string) error {
	for i := range w.srcInfo.Fields {
		if w.srcInfo.Fields[i].Name == srcFieldName {
			w.srcInfo.Fields[i].Tag.DstFieldName = dstFieldName
			w.srcInfo.Fields[i].Tag.UsingFunc = converter
			slog.Debug("updated field tag", "src", srcFieldName, "dst", dstFieldName, "converter", converter)
			return nil
		}
	}
	return fmt.Errorf("source field %q not found in struct %s", srcFieldName, w.srcInfo.Name)
}

func (w *mappingWalker) parseComputeCall(call *ast.CallExpr) error {
	if len(call.Args) != 2 {
		return fmt.Errorf("c.Compute() expects 2 arguments, got %d", len(call.Args))
	}
	dst, err := w.exprToString(call.Args[0])
	if err != nil {
		return fmt.Errorf("could not parse dst in c.Compute(): %w", err)
	}
	expr, err := w.exprToString(call.Args[1])
	if err != nil {
		return fmt.Errorf("could not parse expression in c.Compute(): %w", err)
	}

	dstName := strings.SplitN(dst, ".", 2)[1]
	computed := model.ComputedField{
		DstName: dstName,
		Expr:    expr,
	}
	w.pair.Computed = append(w.pair.Computed, computed)
	slog.Debug("added computed field", "dst", dstName, "expr", expr)
	return nil
}

func (w *mappingWalker) exprToString(expr ast.Expr) (string, error) {
	switch n := expr.(type) {
	case *ast.SelectorExpr:
		x, err := w.exprToString(n.X)
		if err != nil {
			return "", err
		}
		return x + "." + n.Sel.Name, nil
	case *ast.Ident:
		return n.Name, nil
	case *ast.CallExpr:
		fun, err := w.exprToString(n.Fun)
		if err != nil {
			return "", err
		}
		var args []string
		for _, arg := range n.Args {
			argStr, err := w.exprToString(arg)
			if err != nil {
				return "", err
			}
			args = append(args, argStr)
		}
		return fmt.Sprintf("%s(%s)", fun, strings.Join(args, ", ")), nil
	default:
		return "", fmt.Errorf("unsupported expression type: %T", expr)
	}
}
