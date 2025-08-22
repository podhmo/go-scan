package main

import (
	"fmt"
	"go/ast"
	"log/slog"
	"path/filepath"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
)

// LoadPatternsFromConfig loads custom analysis patterns from a Go configuration file.
// It is a wrapper around LoadPatternsFromPath.
func LoadPatternsFromConfig(filePath string, logger *slog.Logger, scanner *goscan.Scanner) ([]patterns.Pattern, error) {
	// The underlying implementation now scans the whole package, so we just need the path.
	return LoadPatternsFromPath(filePath, logger, scanner)
}

// LoadPatternsFromPath evaluates the Go package containing the given file
// and loads the custom analysis patterns defined in a `Patterns` variable.
func LoadPatternsFromPath(filePath string, logger *slog.Logger, scanner *goscan.Scanner) ([]patterns.Pattern, error) {
	// Step 1: Set up the minigo interpreter.
	interp, err := minigo.NewInterpreter(scanner)
	if err != nil {
		return nil, fmt.Errorf("failed to create minigo interpreter: %w", err)
	}

	// Step 2: Evaluate the entire package containing the patterns file.
	// This ensures all functions and types in the package are available to the script.
	dir := filepath.Dir(filePath)
	if err := interp.EvalPackage(dir); err != nil {
		return nil, fmt.Errorf("failed to evaluate patterns package in %q: %w", dir, err)
	}

	// Step 3: Extract the 'Patterns' variable from the global environment.
	patternsObj, ok := interp.GlobalEnvForTest().Get("Patterns")
	if !ok {
		return nil, fmt.Errorf("could not find 'Patterns' variable in config source")
	}

	// Step 4: Manually iterate through the array of struct objects from minigo.
	// The `As()` helper has limitations with fields of type `any` that hold complex objects.
	patternsArray, ok := patternsObj.(*object.Array)
	if !ok {
		return nil, fmt.Errorf("expected 'Patterns' to be an array, but got %s", patternsObj.Type())
	}

	var configs []patterns.PatternConfig
	for _, item := range patternsArray.Elements {
		structInstance, ok := item.(*object.StructInstance)
		if !ok {
			continue // Or return an error
		}

		var config patterns.PatternConfig

		// Extract 'Fn' field
		if fnObj, ok := structInstance.Fields["Fn"]; ok {
			config.Fn = fnObj // Keep the minigo object
		}

		// Extract 'Type' field
		if typeObj, ok := structInstance.Fields["Type"]; ok {
			if s, ok := typeObj.(*object.String); ok {
				config.Type = patterns.PatternType(s.Value)
			}
		}

		// Extract 'ArgIndex' field
		if argIndexObj, ok := structInstance.Fields["ArgIndex"]; ok {
			if i, ok := argIndexObj.(*object.Integer); ok {
				config.ArgIndex = int(i.Value)
			}
		}

		// Extract 'StatusCode' field
		if statusCodeObj, ok := structInstance.Fields["StatusCode"]; ok {
			if s, ok := statusCodeObj.(*object.String); ok {
				config.StatusCode = s.Value
			}
		}

		// Extract 'Description' field
		if descObj, ok := structInstance.Fields["Description"]; ok {
			if s, ok := descObj.(*object.String); ok {
				config.Description = s.Value
			}
		}

		// Extract 'NameArgIndex' field
		if nameArgIndexObj, ok := structInstance.Fields["NameArgIndex"]; ok {
			if i, ok := nameArgIndexObj.(*object.Integer); ok {
				config.NameArgIndex = int(i.Value)
			}
		}

		configs = append(configs, config)
	}

	// Step 5: Convert the data-only configs into executable patterns.
	return convertConfigsToPatterns(configs, logger)
}

// patternKeyFromFunc generates a string key for a pattern from a minigo function object.
// This key is used by the symbolic execution engine to match function calls.
// e.g., "github.com/user/project/api.ListUsers" or "(*github.com/user/project/models.User).TableName"
func patternKeyFromFunc(fn any) (string, error) {
	switch f := fn.(type) {
	case *object.GoSourceFunction:
		// Regular function: "pkg/path.FuncName"
		return fmt.Sprintf("%s.%s", f.PkgPath, f.Info.Name), nil

	case *object.GoMethodValue:
		// Method value: "(*pkg/path.TypeName).MethodName"
		method := f.Fn
		if method.Recv == nil || len(method.Recv.List) == 0 {
			return "", fmt.Errorf("method value %q has no receiver info", method.Name)
		}
		recvField := method.Recv.List[0]

		// The receiver type is stored in the method's definition environment.
		// We need to extract its name and package path.
		var recvTypeName string
		var recvIsPointer bool
		var recvTypeExpr ast.Expr

		// The AST node for the receiver type, e.g., `*User` or `User`.
		recvTypeNode := recvField.Type
		if star, ok := recvTypeNode.(*ast.StarExpr); ok {
			recvIsPointer = true
			recvTypeExpr = star.X
		} else {
			recvTypeExpr = recvTypeNode
		}

		// The type name itself, e.g., `User`.
		ident, ok := recvTypeExpr.(*ast.Ident)
		if !ok {
			return "", fmt.Errorf("unsupported receiver type expression: %T", recvTypeExpr)
		}
		recvTypeName = ident.Name

		// Now, find the package path. This is the trickiest part.
		// The method's `Env` is the environment where the struct was defined.
		// We need to find the `object.Package` that corresponds to this environment.
		// This requires a new mechanism or passing more context.
		//
		// HACK/TODO: For now, we assume the type is in the same package as the method declaration.
		// This is often true but not always (e.g., methods on types from other packages).
		// The `GoSourceFunction` has a `PkgPath`, but the `Function` object inside `GoMethodValue` does not.
		// This part of the logic will be completed after enhancing the `minigo` object model
		// to make the receiver's package path more accessible.
		// For now, we will leave a placeholder. A proper implementation needs to be added.
		// We'll simulate finding it for now to make progress.
		var pkgPath string
		if method.FScope != nil {
			// This is a temporary and incorrect assumption.
			// We are trying to find a package path from the file scope's aliases.
			// This will not work reliably.
			for _, path := range method.FScope.Aliases {
				pkgPath = path // Just grab the first one we see.
				break
			}
		}
		if pkgPath == "" {
			// As a last resort, maybe it's a dot import.
			if method.FScope != nil && len(method.FScope.DotImports) > 0 {
				pkgPath = method.FScope.DotImports[0]
			}
		}

		if pkgPath == "" {
			return "", fmt.Errorf("TODO: could not determine package path for receiver type %s", recvTypeName)
		}

		if recvIsPointer {
			return fmt.Sprintf("(*%s.%s).%s", pkgPath, recvTypeName, method.Name.Name), nil
		}
		return fmt.Sprintf("(%s.%s).%s", pkgPath, recvTypeName, method.Name.Name), nil

	default:
		return "", fmt.Errorf("unsupported type for Fn field: %T", fn)
	}
}

// convertConfigsToPatterns translates the user-defined pattern configurations
// into the internal Pattern format with executable Apply functions.
func convertConfigsToPatterns(configs []patterns.PatternConfig, logger *slog.Logger) ([]patterns.Pattern, error) {
	result := make([]patterns.Pattern, len(configs))
	for i, config := range configs {
		c := config // capture loop variable

		key, err := patternKeyFromFunc(c.Fn)
		if err != nil {
			// If key generation fails, we cannot proceed with this pattern.
			// For now, we'll log a warning and skip it. A stricter implementation might return an error.
			logger.Warn("could not generate key for pattern, skipping", "pattern_index", i, "error", err)
			continue
		}

		// Validate the pattern type string and required fields.
		switch c.Type {
		case patterns.RequestBody, patterns.ResponseBody, patterns.DefaultResponse:
			// valid
		case patterns.CustomResponse:
			if c.StatusCode == "" {
				return nil, fmt.Errorf("pattern %d: 'StatusCode' is required for type %q", i, c.Type)
			}
		case patterns.PathParameter, patterns.QueryParameter, patterns.HeaderParameter:
			// We can't easily validate that NameArgIndex and ArgIndex are set
			// because 0 is a valid value. The runtime will handle incorrect indices.
		default:
			return nil, fmt.Errorf("pattern %d: unknown 'Type' value %q for key %q", i, c.Type, key)
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
