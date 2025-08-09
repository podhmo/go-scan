package minigo2

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"reflect"
	"strings"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo2/evaluator"
	"github.com/podhmo/go-scan/minigo2/object"
)

// Interpreter is the main entry point for the minigo2 language.
// It holds the state of the interpreter, including the scanner for package resolution
// and the root environment for script execution.
type Interpreter struct {
	scanner  *goscan.Scanner
	Env      *object.Environment
	Registry *object.SymbolRegistry
}

// NewInterpreter creates a new interpreter instance.
// It initializes a scanner and a root environment.
func NewInterpreter(options ...goscan.ScannerOption) (*Interpreter, error) {
	scanner, err := goscan.New(options...)
	if err != nil {
		return nil, fmt.Errorf("initializing scanner: %w", err)
	}
	return &Interpreter{
		scanner:  scanner,
		Env:      object.NewEnvironment(),
		Registry: object.NewSymbolRegistry(),
	}, nil
}

// Register makes Go symbols (variables or functions) available for import by a script.
// For example, `interp.Register("strings", map[string]any{"ToUpper": strings.ToUpper})`
// allows a script to `import "strings"` and call `strings.ToUpper()`.
func (i *Interpreter) Register(pkgPath string, symbols map[string]any) {
	i.Registry.Register(pkgPath, symbols)
}

// Options configures the interpreter environment.
type Options struct {
	// Globals allows injecting Go variables into the script's global scope.
	// The map key is the variable name in the script.
	// The value can be any Go variable, which will be made available via reflection.
	Globals map[string]any

	// Source is the script content.
	Source []byte

	// Filename is the name of the script file, used for error messages.
	Filename string
}

// Result holds the outcome of a script execution.
type Result struct {
	// Value is the raw minigo2 object returned by the script.
	Value object.Object
}

// As unmarshals the result of the script execution into a Go variable.
// The target must be a pointer to a Go variable.
// It uses reflection to populate the fields of the target, similar to how
// `json.Unmarshal` works.
func (r *Result) As(target any) error {
	if target == nil {
		return fmt.Errorf("target cannot be nil")
	}

	dstVal := reflect.ValueOf(target)
	if dstVal.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer, but got %T", target)
	}
	if dstVal.IsNil() {
		return fmt.Errorf("target pointer cannot be nil")
	}

	return unmarshal(r.Value, dstVal.Elem())
}

// unmarshal is a recursive helper function that populates a Go `reflect.Value` (dst)
// from a minigo2 `object.Object` (src).
func unmarshal(src object.Object, dst reflect.Value) error {
	if !dst.CanSet() {
		return fmt.Errorf("cannot set destination value of type %s", dst.Type())
	}

	// Dereference pointers until we reach a non-pointer or a nil pointer.
	for dst.Kind() == reflect.Ptr {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst = dst.Elem()
	}

	switch s := src.(type) {
	case *object.Null:
		// Set the destination to its zero value (e.g., nil for slices/maps/pointers).
		dst.Set(reflect.Zero(dst.Type()))
		return nil

	case *object.Integer:
		switch dst.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			dst.SetInt(s.Value)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			dst.SetUint(uint64(s.Value))
		case reflect.Float32, reflect.Float64:
			dst.SetFloat(float64(s.Value))
		default:
			return fmt.Errorf("cannot unmarshal integer into %s", dst.Type())
		}
		return nil

	case *object.String:
		if dst.Kind() != reflect.String {
			return fmt.Errorf("cannot unmarshal string into %s", dst.Type())
		}
		dst.SetString(s.Value)
		return nil

	case *object.Boolean:
		if dst.Kind() != reflect.Bool {
			return fmt.Errorf("cannot unmarshal boolean into %s", dst.Type())
		}
		dst.SetBool(s.Value)
		return nil

	case *object.GoValue:
		if !s.Value.Type().AssignableTo(dst.Type()) {
			// Allow conversion if possible (e.g., from `int` to `int64`)
			if s.Value.Type().ConvertibleTo(dst.Type()) {
				dst.Set(s.Value.Convert(dst.Type()))
				return nil
			}
			return fmt.Errorf("cannot assign Go value of type %s to %s", s.Value.Type(), dst.Type())
		}
		dst.Set(s.Value)
		return nil

	case *object.Array:
		if dst.Kind() != reflect.Slice {
			return fmt.Errorf("cannot unmarshal array into non-slice type %s", dst.Type())
		}
		sliceType := dst.Type()
		newSlice := reflect.MakeSlice(sliceType, len(s.Elements), len(s.Elements))
		for i, elem := range s.Elements {
			if err := unmarshal(elem, newSlice.Index(i)); err != nil {
				return fmt.Errorf("error in slice element %d: %w", i, err)
			}
		}
		dst.Set(newSlice)
		return nil

	case *object.Map:
		if dst.Kind() != reflect.Map {
			return fmt.Errorf("cannot unmarshal map into non-map type %s", dst.Type())
		}
		mapType := dst.Type()
		keyType := mapType.Key()
		valType := mapType.Elem()
		newMap := reflect.MakeMap(mapType)
		for _, pair := range s.Pairs {
			key := reflect.New(keyType).Elem()
			if err := unmarshal(pair.Key, key); err != nil {
				return fmt.Errorf("error in map key: %w", err)
			}
			val := reflect.New(valType).Elem()
			if err := unmarshal(pair.Value, val); err != nil {
				return fmt.Errorf("error in map value for key %v: %w", key, err)
			}
			newMap.SetMapIndex(key, val)
		}
		dst.Set(newMap)
		return nil

	case *object.StructInstance:
		if dst.Kind() != reflect.Struct {
			return fmt.Errorf("cannot unmarshal struct instance into non-struct type %s", dst.Type())
		}
		// Create a map of destination fields for easy lookup (case-insensitive).
		dstFields := make(map[string]reflect.Value)
		for i := 0; i < dst.NumField(); i++ {
			field := dst.Type().Field(i)
			// Skip unexported fields.
			if field.PkgPath != "" {
				continue
			}
			dstFields[strings.ToLower(field.Name)] = dst.Field(i)
		}

		for fieldName, srcFieldVal := range s.Fields {
			if dstField, ok := dstFields[strings.ToLower(fieldName)]; ok {
				if err := unmarshal(srcFieldVal, dstField); err != nil {
					return fmt.Errorf("error in struct field %q: %w", fieldName, err)
				}
			}
		}
		return nil

	default:
		return fmt.Errorf("unsupported object type for unmarshaling: %s", src.Type())
	}
}

// Eval executes a minigo2 script. It evaluates the entire script from top to bottom
// within the interpreter's persistent environment.
func (i *Interpreter) Eval(ctx context.Context, opts Options) (*Result, error) {
	// Inject global variables from Go into the interpreter's environment.
	for name, value := range opts.Globals {
		i.Env.Set(name, &object.GoValue{Value: reflect.ValueOf(value)})
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, opts.Filename, opts.Source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing script: %w", err)
	}

	eval := evaluator.New(fset, i.scanner, i.Registry)
	var lastVal object.Object
	for _, decl := range node.Decls {
		lastVal = eval.Eval(decl, i.Env)
		if err, ok := lastVal.(*object.Error); ok {
			// The error object's Inspect() method now returns a fully formatted string.
			return nil, fmt.Errorf("%s", err.Inspect())
		}
	}

	return &Result{Value: lastVal}, nil
}
