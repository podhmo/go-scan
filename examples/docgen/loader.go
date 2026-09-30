package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"github.com/podhmo/go-scan/minigo2"
	"github.com/podhmo/go-scan/minigo2/runtime"
)

// LoadPatternsFromConfig loads custom analysis patterns from a Go configuration file.
// The file is interpreted by minigo2 as a standalone entry point, so imports in the
// config resolve against the go.mod in the config file's own directory.
func LoadPatternsFromConfig(filePath string, logger *slog.Logger, scanner *goscan.Scanner) ([]patterns.Pattern, error) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not resolve patterns config path %q: %w", filePath, err)
	}

	// The engine's resolver anchors at the config file's directory so that
	// module-local imports and `replace` directives resolve correctly.
	engine := minigo2.NewEngine(filepath.Dir(abs))

	// Evaluate the script: LoadFile indexes the file, EnsureReady runs the
	// package-level var/const initializers that populate `Patterns`.
	pkg, err := engine.LoadFile(context.Background(), abs)
	if err != nil {
		return nil, fmt.Errorf("failed to load patterns config source: %w", err)
	}
	if err := pkg.EnsureReady(); err != nil {
		return nil, fmt.Errorf("failed to evaluate patterns config source: %w", err)
	}

	// Extract the 'Patterns' variable from the package globals.
	patternsVal, ok := pkg.Globals.Get("Patterns")
	if !ok {
		return nil, fmt.Errorf("could not find 'Patterns' variable in config source")
	}

	// Unmarshal the minigo2 value into a slice of PatternConfig structs.
	var configs []patterns.PatternConfig
	result := minigo2.Result{V: patternsVal}
	if err := result.As(&configs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal 'Patterns' variable into []patterns.PatternConfig: %w", err)
	}

	// Convert the data-only configs into executable patterns.
	return convertConfigsToPatterns(configs, logger, scanner)
}

// convertConfigsToPatterns translates the user-defined pattern configurations
// into the internal Pattern format with executable Apply functions.
func convertConfigsToPatterns(configs []patterns.PatternConfig, logger *slog.Logger, scanner *goscan.Scanner) ([]patterns.Pattern, error) {
	result := make([]patterns.Pattern, len(configs))
	for i, config := range configs {
		c := config // capture loop variable

		var key string
		// If Fn is provided, derive the key from the function object.
		if fn, ok := c.Fn.(*runtime.Function); ok && fn != nil {
			key = fmt.Sprintf("%s.%s", fn.Pkg.Path, fn.Name)
		} else if bm, ok := c.Fn.(*runtime.BoundMethod); ok && bm != nil {
			// Handle method references: method values on an instance
			// (var v MyType; v.MyMethod) and on a typed nil
			// ((*MyType)(nil).MyMethod) both arrive as a BoundMethod.
			key = buildKeyForMethod(bm.Fn)
		} else {
			key = c.Key
		}

		if key == "" {
			return nil, fmt.Errorf("pattern %q requires either a 'Key' string or a 'Fn' reference", c.Name)
		}

		// Validate the pattern type string and required fields.
		switch c.Type {
		case patterns.RequestBody, patterns.ResponseBody, patterns.DefaultResponse:
			// valid
		case patterns.CustomResponse:
			if c.StatusCode == "" {
				return nil, fmt.Errorf("pattern %q: 'StatusCode' is required for type %q", c.Name, c.Type)
			}
		case patterns.PathParameter, patterns.QueryParameter, patterns.HeaderParameter:
			// We can't easily validate that NameArgIndex and ArgIndex are set
			// because 0 is a valid value. The runtime will handle incorrect indices.
		default:
			return nil, fmt.Errorf("pattern %q: unknown 'Type' value %q", c.Name, c.Type)
		}

		result[i].Key = key

		switch c.Type {
		case patterns.RequestBody:
			result[i].Apply = patterns.HandleCustomRequestBody(c.ArgIndex)
		case patterns.ResponseBody:
			result[i].Apply = patterns.HandleCustomResponseBody(c.ArgIndex)
		case patterns.CustomResponse:
			result[i].Apply = patterns.HandleCustomResponse(c.StatusCode, c.ArgIndex)
		case patterns.DefaultResponse:
			result[i].Apply = patterns.HandleDefaultResponse(c.ArgIndex)
		case patterns.PathParameter, patterns.QueryParameter, patterns.HeaderParameter:
			result[i].Apply = patterns.HandleCustomParameter(string(c.Type), c.Description, c.NameArgIndex, c.ArgIndex)
		default:
			// This case should be unreachable due to the validation above
			logger.Warn("unreachable: unknown pattern type", "type", c.Type, "key", key)
			return nil, fmt.Errorf("unknown pattern type %q for key %q", c.Type, key)
		}
		logger.Debug("loaded custom pattern", "key", key, "type", c.Type, "argIndex", c.ArgIndex)
	}
	return result, nil
}

// buildKeyForMethod constructs the fully qualified key for a method.
// Materialized methods carry the receiver's type name in Recv (the Name
// is "Type.Method" — the split fallback covers methods built by other
// paths, e.g. embedded/promoted bindings). The analyzer looks calls up
// as "(pkg.Type).Method" / "(*pkg.Type).Method" — parentheses wrap the
// whole receiver type — so the key is rebuilt from Pkg.Path, Recv, and
// PtrRecv. The package's Path is already the import path resolved by the
// engine, so no filesystem-to-module conversion is needed.
func buildKeyForMethod(fn *runtime.Function) string {
	typeName, methodName := fn.Recv, fn.Name
	if typeName != "" {
		methodName = strings.TrimPrefix(methodName, typeName+".")
	} else if i := strings.LastIndexByte(methodName, '.'); i >= 0 {
		typeName, methodName = methodName[:i], methodName[i+1:]
	}
	recv := fn.Pkg.Path + "." + typeName
	if fn.PtrRecv {
		recv = "(*" + recv + ")"
	} else {
		recv = "(" + recv + ")"
	}
	return recv + "." + methodName
}
