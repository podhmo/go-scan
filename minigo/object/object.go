package object

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"hash/fnv"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/podhmo/go-scan/scanner"
)

// ObjectType is a string representation of an object's type.
type ObjectType string

// DeclWithScope is a helper struct to associate a declaration with its file scope.
type DeclWithScope struct {
	Decl  ast.Decl
	Scope *FileScope
}

// Define the basic object types. More will be added later.
const (
	INTEGER_OBJ              ObjectType = "INTEGER"
	FLOAT_OBJ                ObjectType = "FLOAT"
	COMPLEX_OBJ              ObjectType = "COMPLEX"
	BOOLEAN_OBJ              ObjectType = "BOOLEAN"
	STRING_OBJ               ObjectType = "STRING"
	NIL_OBJ                  ObjectType = "NIL"
	BREAK_OBJ                ObjectType = "BREAK"
	CONTINUE_OBJ             ObjectType = "CONTINUE"
	RETURN_VALUE_OBJ         ObjectType = "RETURN_VALUE"
	PANIC_OBJ                ObjectType = "PANIC"
	FUNCTION_OBJ             ObjectType = "FUNCTION"
	BUILTIN_OBJ              ObjectType = "BUILTIN"
	SPECIAL_FORM_OBJ         ObjectType = "SPECIAL_FORM"
	STRUCT_DEFINITION_OBJ    ObjectType = "STRUCT_DEFINITION"
	STRUCT_INSTANCE_OBJ      ObjectType = "STRUCT_INSTANCE"
	INTERFACE_DEFINITION_OBJ ObjectType = "INTERFACE_DEFINITION"
	INTERFACE_INSTANCE_OBJ   ObjectType = "INTERFACE_INSTANCE"
	BOUND_METHOD_OBJ         ObjectType = "BOUND_METHOD"
	POINTER_OBJ              ObjectType = "POINTER"
	POINTER_TYPE_OBJ         ObjectType = "POINTER_TYPE"
	ARRAY_OBJ                ObjectType = "ARRAY"
	MAP_OBJ                  ObjectType = "MAP"
	TUPLE_OBJ                ObjectType = "TUPLE"
	PACKAGE_OBJ              ObjectType = "PACKAGE"
	GO_VALUE_OBJ             ObjectType = "GO_VALUE"
	GO_TYPE_OBJ              ObjectType = "GO_TYPE"
	GO_METHOD_OBJ            ObjectType = "GO_METHOD"
	GO_SOURCE_FUNCTION_OBJ   ObjectType = "GO_SOURCE_FUNCTION"
	TYPED_NIL_OBJ            ObjectType = "TYPED_NIL"
	ERROR_OBJ                ObjectType = "ERROR"
	AST_NODE_OBJ             ObjectType = "AST_NODE"

	// Generics related
	TYPE_OBJ              ObjectType = "TYPE"
	TYPE_ALIAS_OBJ        ObjectType = "TYPE_ALIAS"
	INSTANTIATED_TYPE_OBJ ObjectType = "INSTANTIATED_TYPE"

	// Type Kinds
	ARRAY_TYPE_OBJ ObjectType = "ARRAY_TYPE"
	MAP_TYPE_OBJ   ObjectType = "MAP_TYPE"
	FUNC_TYPE_OBJ  ObjectType = "FUNC_TYPE"
)

// Hashable is an interface for objects that can be used as map keys.
type Hashable interface {
	HashKey() HashKey
}

// HashKey is used as a key in the internal hash map for Map objects.
type HashKey struct {
	Type  ObjectType
	Value uint64
}

// CallFrame represents a single frame in the call stack.
type CallFrame struct {
	Pos          token.Pos
	Function     string
	Fn           *Function
	IsBuiltin    bool
	Defers       []*DeferredCall
	NamedReturns *Environment
}

// DeferredCall represents a deferred function call.
type DeferredCall struct {
	Call *ast.CallExpr
	Env  *Environment
}

func (cf *CallFrame) Format(fset *token.FileSet) string {
	position := fset.Position(cf.Pos)
	funcName := cf.Function
	if funcName == "" {
		funcName = "<script>"
	}
	sourceLine, err := getSourceLine(position.Filename, position.Line)
	formattedSourceLine := ""
	if err == nil && sourceLine != "" {
		formattedSourceLine = "\n\t\t" + sourceLine
	}
	return fmt.Sprintf("\t%s:%d:%d:\tin %s%s", position.Filename, position.Line, position.Column, funcName, formattedSourceLine)
}

// Object is the interface that all value types in our interpreter will implement.
type Object interface {
	Type() ObjectType
	Inspect() string
}

type Integer struct{ Value int64 }
func (i *Integer) Type() ObjectType { return INTEGER_OBJ }
func (i *Integer) Inspect() string  { return fmt.Sprintf("%d", i.Value) }
func (i *Integer) HashKey() HashKey { return HashKey{Type: i.Type(), Value: uint64(i.Value)} }

type Float struct{ Value float64 }
func (f *Float) Type() ObjectType { return FLOAT_OBJ }
func (f *Float) Inspect() string  { return fmt.Sprintf("%g", f.Value) }
func (f *Float) HashKey() HashKey {
	h := fnv.New64a()
	fmt.Fprintf(h, "%f", f.Value)
	return HashKey{Type: f.Type(), Value: h.Sum64()}
}

type Complex struct{ Real, Imag float64 }
func (c *Complex) Type() ObjectType { return COMPLEX_OBJ }
func (c *Complex) Inspect() string  { return fmt.Sprintf("(%g+%gi)", c.Real, c.Imag) }

type String struct{ Value string }
func (s *String) Type() ObjectType { return STRING_OBJ }
func (s *String) Inspect() string  { return s.Value }
func (s *String) HashKey() HashKey {
	h := fnv.New64a()
	h.Write([]byte(s.Value))
	return HashKey{Type: s.Type(), Value: h.Sum64()}
}

type Boolean struct{ Value bool }
func (b *Boolean) Type() ObjectType { return BOOLEAN_OBJ }
func (b *Boolean) Inspect() string  { return fmt.Sprintf("%t", b.Value) }
func (b *Boolean) HashKey() HashKey {
	var value uint64
	if b.Value {
		value = 1
	}
	return HashKey{Type: b.Type(), Value: value}
}

type Nil struct{}
func (n *Nil) Type() ObjectType { return NIL_OBJ }
func (n *Nil) Inspect() string  { return "nil" }

type TypedNil struct{ TypeInfo *scanner.TypeInfo }
func (tn *TypedNil) Type() ObjectType { return TYPED_NIL_OBJ }
func (tn *TypedNil) Inspect() string  { return fmt.Sprintf("nil (%s)", tn.TypeInfo.Name) }

type GoMethod struct {
	Recv *scanner.TypeInfo
	Func *scanner.FunctionInfo
}
func (gm *GoMethod) Type() ObjectType { return GO_METHOD_OBJ }
func (gm *GoMethod) Inspect() string {
	return fmt.Sprintf("method %s.%s", gm.Recv.Name, gm.Func.Name)
}

type GoSourceFunction struct {
	Func    *scanner.FunctionInfo
	PkgPath string
	DefEnv  *Environment
}
func (gsf *GoSourceFunction) Type() ObjectType { return GO_SOURCE_FUNCTION_OBJ }
func (gsf *GoSourceFunction) Inspect() string {
	return fmt.Sprintf("go function %s", gsf.Func.Name)
}

type BreakStatement struct{}
func (bs *BreakStatement) Type() ObjectType { return BREAK_OBJ }
func (bs *BreakStatement) Inspect() string  { return "break" }

type ContinueStatement struct{}
func (cs *ContinueStatement) Type() ObjectType { return CONTINUE_OBJ }
func (cs *ContinueStatement) Inspect() string  { return "continue" }

type ReturnValue struct{ Value Object }
func (rv *ReturnValue) Type() ObjectType { return RETURN_VALUE_OBJ }
func (rv *ReturnValue) Inspect() string  { return rv.Value.Inspect() }

type Panic struct{ Value Object }
func (p *Panic) Type() ObjectType { return PANIC_OBJ }
func (p *Panic) Inspect() string  { return p.Value.Inspect() }

type Function struct {
	Name       *ast.Ident
	Recv       *ast.FieldList
	TypeParams *ast.FieldList
	Parameters *ast.FieldList
	Results    *ast.FieldList
	Body       *ast.BlockStmt
	Env        *Environment
	FScope     *FileScope
}
func (f *Function) IsVariadic() bool {
	if f.Parameters == nil || len(f.Parameters.List) == 0 {
		return false
	}
	lastParam := f.Parameters.List[len(f.Parameters.List)-1]
	_, ok := lastParam.Type.(*ast.Ellipsis)
	return ok
}
func (f *Function) HasNamedReturns() bool {
	return f.Results != nil && len(f.Results.List) > 0 && len(f.Results.List[0].Names) > 0
}
func (f *Function) Type() ObjectType { return FUNCTION_OBJ }
func (f *Function) Inspect() string {
	var out bytes.Buffer
	params := []string{}
	if f.Parameters != nil {
		for _, p := range f.Parameters.List {
			paramStr := []string{}
			for _, name := range p.Names {
				paramStr = append(paramStr, name.String())
			}
			params = append(params, strings.Join(paramStr, ", "))
		}
	}
	out.WriteString("func")
	if f.Name != nil {
		out.WriteString(" ")
		out.WriteString(f.Name.String())
	}
	out.WriteString("(")
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") { ... }")
	return out.String()
}

type ArrayType struct{ ElementType Object }
func (at *ArrayType) Type() ObjectType { return ARRAY_TYPE_OBJ }
func (at *ArrayType) Inspect() string  { return "[]" + at.ElementType.Inspect() }

type MapType struct{ KeyType, ValueType Object }
func (mt *MapType) Type() ObjectType { return MAP_TYPE_OBJ }
func (mt *MapType) Inspect() string {
	return fmt.Sprintf("map[%s]%s", mt.KeyType.Inspect(), mt.ValueType.Inspect())
}

type BuiltinContext struct {
	Stdin            io.Reader
	Stdout           io.Writer
	Stderr           io.Writer
	Fset             *token.FileSet
	Env              *Environment
	FScope           *FileScope
	IsExecutingDefer func() bool
	GetPanic         func() *Panic
	ClearPanic       func()
	NewError         func(pos token.Pos, format string, args ...interface{}) *Error
}
type BuiltinFunction func(ctx *BuiltinContext, pos token.Pos, args ...Object) Object
type Builtin struct{ Fn BuiltinFunction }
func (b *Builtin) Type() ObjectType { return BUILTIN_OBJ }
func (b *Builtin) Inspect() string  { return "builtin function" }

type StructDefinition struct {
	Name       *ast.Ident
	TypeParams *ast.FieldList
	Fields     []*ast.Field
	Methods    map[string]*Function
	GoMethods  map[string]*scanner.FunctionInfo
	FieldTags  map[string]string
	Env        *Environment
}
func (sd *StructDefinition) Type() ObjectType { return STRUCT_DEFINITION_OBJ }
func (sd *StructDefinition) Inspect() string  { return fmt.Sprintf("struct %s", sd.Name.String()) }

type InterfaceDefinition struct {
	Name     *ast.Ident
	Methods  *ast.FieldList
	TypeList []ast.Expr
}
func (id *InterfaceDefinition) Type() ObjectType { return INTERFACE_DEFINITION_OBJ }
func (id *InterfaceDefinition) Inspect() string {
	var out bytes.Buffer
	methods := []string{}
	if id.Methods != nil {
		for _, method := range id.Methods.List {
			if len(method.Names) > 0 {
				methods = append(methods, method.Names[0].Name+"()")
			}
		}
	}
	out.WriteString("interface { ")
	out.WriteString(strings.Join(methods, "; "))
	out.WriteString(" }")
	return out.String()
}

type InterfaceInstance struct {
	Def   *InterfaceDefinition
	Value Object
}
func (ii *InterfaceInstance) Type() ObjectType { return INTERFACE_INSTANCE_OBJ }
func (ii *InterfaceInstance) Inspect() string {
	if ii.Value == nil || ii.Value.Type() == NIL_OBJ {
		return "nil"
	}
	return ii.Value.Inspect()
}

type BoundMethod struct {
	Fn       *Function
	Receiver Object
}
func (bm *BoundMethod) Type() ObjectType { return BOUND_METHOD_OBJ }
func (bm *BoundMethod) Inspect() string  { return fmt.Sprintf("method %s()", bm.Fn.Name.String()) }

type StructInstance struct {
	Def      *StructDefinition
	TypeArgs []Object
	Fields   map[string]Object
}
func (si *StructInstance) Type() ObjectType { return STRUCT_INSTANCE_OBJ }
func (si *StructInstance) Inspect() string {
	var out bytes.Buffer
	fields := []string{}
	for k, v := range si.Fields {
		fields = append(fields, fmt.Sprintf("%s: %s", k, v.Inspect()))
	}
	out.WriteString(si.Def.Name.String())
	out.WriteString("{")
	out.WriteString(strings.Join(fields, ", "))
	out.WriteString("}")
	return out.String()
}
func (si *StructInstance) Copy() *StructInstance {
	newFields := make(map[string]Object, len(si.Fields))
	for k, v := range si.Fields {
		newFields[k] = v
	}
	return &StructInstance{Def: si.Def, Fields: newFields}
}

type Pointer struct{ Element *Object }
func (p *Pointer) Type() ObjectType { return POINTER_OBJ }
func (p *Pointer) Inspect() string  { return fmt.Sprintf("0x%x", p.Element) }

type PointerType struct{ ElementType Object }
func (pt *PointerType) Type() ObjectType { return POINTER_TYPE_OBJ }
func (pt *PointerType) Inspect() string  { return "*" + pt.ElementType.Inspect() }

type Array struct {
	SliceType *ArrayType
	Elements  []Object
}
func (a *Array) Type() ObjectType { return ARRAY_OBJ }
func (a *Array) Inspect() string {
	var out bytes.Buffer
	elements := []string{}
	for _, e := range a.Elements {
		elements = append(elements, e.Inspect())
	}
	out.WriteString("[")
	out.WriteString(strings.Join(elements, " "))
	out.WriteString("]")
	return out.String()
}

type MapPair struct{ Key, Value Object }
type Map struct {
	MapType *MapType
	Pairs   map[HashKey]MapPair
}
func (m *Map) Type() ObjectType { return MAP_OBJ }
func (m *Map) Inspect() string {
	var out bytes.Buffer
	pairs := []string{}
	for _, pair := range m.Pairs {
		pairs = append(pairs, fmt.Sprintf("%s: %s", pair.Key.Inspect(), pair.Value.Inspect()))
	}
	out.WriteString("{")
	out.WriteString(strings.Join(pairs, ", "))
	out.WriteString("}")
	return out.String()
}

type Tuple struct{ Elements []Object }
func (t *Tuple) Type() ObjectType { return TUPLE_OBJ }
func (t *Tuple) Inspect() string {
	var out bytes.Buffer
	elements := []string{}
	for _, e := range t.Elements {
		elements = append(elements, e.Inspect())
	}
	out.WriteString("(")
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString(")")
	return out.String()
}

type FileScope struct {
	AST        *ast.File
	Aliases    map[string]string
	DotImports []string
}
func NewFileScope(ast *ast.File) *FileScope {
	return &FileScope{AST: ast, Aliases: make(map[string]string), DotImports: make([]string, 0)}
}

type Package struct {
	Name    string
	Path    string
	Info    *scanner.PackageInfo
	Env     *Environment
	FScope  *FileScope
	Members map[string]Object
}
func (p *Package) Type() ObjectType { return PACKAGE_OBJ }
func (p *Package) Inspect() string  { return fmt.Sprintf("package %s (%q)", p.Name, p.Path) }

type GoValue struct{ Value reflect.Value }
func (g *GoValue) Type() ObjectType { return GO_VALUE_OBJ }
func (g *GoValue) Inspect() string {
	if !g.Value.IsValid() {
		return "<invalid Go value>"
	}
	if g.Value.Kind() == reflect.Ptr && g.Value.IsNil() {
		return "nil"
	}
	return fmt.Sprintf("%v", g.Value.Interface())
}

type Error struct {
	Pos       token.Pos
	Message   string
	CallStack []*CallFrame
	fset      *token.FileSet
}
func (e *Error) Type() ObjectType { return ERROR_OBJ }
func (e *Error) Inspect() string {
	var out bytes.Buffer
	out.WriteString("runtime error: ")
	out.WriteString(e.Message)
	if e.fset != nil && e.Pos.IsValid() {
		position := e.fset.Position(e.Pos)
		sourceLine, err := getSourceLine(position.Filename, position.Line)
		out.WriteString(fmt.Sprintf("\n\t%s:%d:%d:", position.Filename, position.Line, position.Column))
		if err == nil && sourceLine != "" {
			out.WriteString("\n\t\t" + sourceLine)
		}
	}
	out.WriteString("\n")
	if e.fset != nil {
		for i := len(e.CallStack) - 1; i >= 0; i-- {
			out.WriteString(e.CallStack[i].Format(e.fset))
			out.WriteString("\n")
		}
	}
	return out.String()
}
func (e *Error) AttachFileSet(fset *token.FileSet) { e.fset = fset }
func (e *Error) Error() string                      { return e.Message }

type AstNode struct{ Node ast.Node }
func (an *AstNode) Type() ObjectType { return AST_NODE_OBJ }
func (an *AstNode) Inspect() string  { return fmt.Sprintf("ast.Node[%T]", an.Node) }

type Type struct{ Name string }
func (t *Type) Type() ObjectType { return TYPE_OBJ }
func (t *Type) Inspect() string  { return t.Name }

type GoType struct{ GoType reflect.Type }
func (gt *GoType) Type() ObjectType { return GO_TYPE_OBJ }
func (gt *GoType) Inspect() string  { return gt.GoType.String() }

type TypeAlias struct {
	Name         *ast.Ident
	TypeParams   *ast.FieldList
	Underlying   ast.Expr
	Env          *Environment
	ResolvedType Object
}
func (ta *TypeAlias) Type() ObjectType { return TYPE_ALIAS_OBJ }
func (ta *TypeAlias) Inspect() string {
	var b strings.Builder
	b.WriteString("type ")
	b.WriteString(ta.Name.Name)
	if ta.TypeParams != nil && len(ta.TypeParams.List) > 0 {
		b.WriteString("[...]")
	}
	b.WriteString(" = ...")
	return b.String()
}

type InstantiatedType struct {
	GenericDef Object
	TypeArgs   []Object
}
func (it *InstantiatedType) Type() ObjectType { return INSTANTIATED_TYPE_OBJ }
func (it *InstantiatedType) Inspect() string {
	var out bytes.Buffer
	switch def := it.GenericDef.(type) {
	case *StructDefinition:
		out.WriteString(def.Name.Name)
	case *Function:
		out.WriteString(def.Name.Name)
	default:
		out.WriteString("<generic>")
	}
	out.WriteString("[")
	args := []string{}
	for _, arg := range it.TypeArgs {
		args = append(args, arg.Inspect())
	}
	out.WriteString(strings.Join(args, ", "))
	out.WriteString("]")
	return out.String()
}

func getSourceLine(filename string, lineNum int) (string, error) {
	if filename == "" || lineNum <= 0 {
		return "", nil
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	currentLine := 1
	for scanner.Scan() {
		if currentLine == lineNum {
			return strings.TrimSpace(scanner.Text()), nil
		}
		currentLine++
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}

type FuncType struct {
	Parameters []Object
	Results    []Object
}
func (ft *FuncType) Type() ObjectType { return FUNC_TYPE_OBJ }
func (ft *FuncType) Inspect() string {
	var out bytes.Buffer
	out.WriteString("func(")
	params := []string{}
	for _, p := range ft.Parameters {
		params = append(params, p.Inspect())
	}
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(")")
	if len(ft.Results) > 0 {
		out.WriteString(" ")
		if len(ft.Results) > 1 {
			out.WriteString("(")
		}
		results := []string{}
		for _, r := range ft.Results {
			results = append(results, r.Inspect())
		}
		out.WriteString(strings.Join(results, ", "))
		if len(ft.Results) > 1 {
			out.WriteString(")")
		}
	}
	return out.String()
}

var (
	TRUE     = &Boolean{Value: true}
	FALSE    = &Boolean{Value: false}
	NIL      = &Nil{}
	BREAK    = &BreakStatement{}
	CONTINUE = &ContinueStatement{}
)

type SymbolRegistry struct {
	packages map[string]map[string]any
	types    map[string]map[string]reflect.Type
}
func NewSymbolRegistry() *SymbolRegistry {
	return &SymbolRegistry{
		packages: make(map[string]map[string]any),
		types:    make(map[string]map[string]reflect.Type),
	}
}
func (r *SymbolRegistry) Register(pkgPath string, symbols map[string]any) {
	if _, ok := r.packages[pkgPath]; !ok {
		r.packages[pkgPath] = make(map[string]any)
	}
	if _, ok := r.types[pkgPath]; !ok {
		r.types[pkgPath] = make(map[string]reflect.Type)
	}
	for name, symbol := range symbols {
		if t, ok := symbol.(reflect.Type); ok {
			r.types[pkgPath][name] = t
		} else {
			r.packages[pkgPath][name] = symbol
		}
	}
}
func (r *SymbolRegistry) Lookup(pkgPath, name string) (any, bool) {
	if pkg, ok := r.packages[pkgPath]; ok {
		if symbol, ok := pkg[name]; ok {
			return symbol, true
		}
	}
	return nil, false
}
func (r *SymbolRegistry) LookupType(pkgPath, name string) (reflect.Type, bool) {
	if pkg, ok := r.types[pkgPath]; ok {
		if t, ok := pkg[name]; ok {
			return t, true
		}
	}
	return nil, false
}
func (r *SymbolRegistry) GetAllFor(pkgPath string) (map[string]any, bool) {
	pkg, ok := r.packages[pkgPath]
	if !ok {
		return nil, false
	}
	clone := make(map[string]any, len(pkg))
	for k, v := range pkg {
		clone[k] = v
	}
	return clone, true
}

type Environment struct {
	store             map[string]*Object
	consts            map[string]Object
	typeParamBindings map[string]Object
	outer             *Environment
}
func NewEnvironment() *Environment {
	s := make(map[string]*Object)
	c := make(map[string]Object)
	t := make(map[string]Object)
	return &Environment{store: s, consts: c, typeParamBindings: t, outer: nil}
}
func NewEnclosedEnvironment(outer *Environment) *Environment {
	env := NewEnvironment()
	env.outer = outer
	return env
}
func (e *Environment) Get(name string) (Object, bool) {
	if obj, ok := e.typeParamBindings[name]; ok {
		return obj, true
	}
	if obj, ok := e.consts[name]; ok {
		return obj, true
	}
	if objPtr, ok := e.store[name]; ok {
		return *objPtr, true
	}
	if e.outer != nil {
		return e.outer.Get(name)
	}
	return nil, false
}
func (e *Environment) SetType(name string, val Object) {
	e.typeParamBindings[name] = val
}
func (e *Environment) GetAddress(name string) (*Object, bool) {
	if objPtr, ok := e.store[name]; ok {
		return objPtr, true
	}
	if e.outer != nil {
		return e.outer.GetAddress(name)
	}
	return nil, false
}
func (e *Environment) GetConstant(name string) (Object, bool) {
	if obj, ok := e.consts[name]; ok {
		return obj, true
	}
	if e.outer != nil {
		return e.outer.GetConstant(name)
	}
	return nil, false
}
func (e *Environment) Set(name string, val Object) Object {
	e.store[name] = &val
	return val
}
func (e *Environment) SetConstant(name string, val Object) Object {
	e.consts[name] = val
	return val
}
func (e *Environment) Outer() *Environment {
	return e.outer
}
func (e *Environment) IsEmpty() bool {
	return len(e.store) == 0 && len(e.consts) == 0 && len(e.typeParamBindings) == 0
}
func (e *Environment) GetAll() map[string]Object {
	all := make(map[string]Object)
	for k, v := range e.store {
		all[k] = *v
	}
	for k, v := range e.consts {
		all[k] = v
	}
	return all
}
func (e *Environment) Assign(name string, val Object) bool {
	if _, ok := e.consts[name]; ok {
		return false
	}
	if objPtr, ok := e.store[name]; ok {
		*objPtr = val
		return true
	}
	if e.outer != nil {
		return e.outer.Assign(name, val)
	}
	return false
}
