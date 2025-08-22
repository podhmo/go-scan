package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
)

// LoadPatternsFromConfig loads custom analysis patterns from a Go configuration file.
func LoadPatternsFromConfig(filePath string, logger *slog.Logger) ([]patterns.Pattern, error) {
	configSource, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read patterns config file %q: %w", filePath, err)
	}
	return LoadPatternsFromSource(configSource, logger)
}

// computeKey generates the analyzer's internal string key from a minigo object.
func computeKey(fn any) (string, error) {
	switch f := fn.(type) {
	case *object.GoSourceFunction:
		return fmt.Sprintf("%s.%s", f.PkgPath, f.Func.Name), nil

	case *object.GoMethod:
		if f.Func.Receiver == nil {
			return "", fmt.Errorf("GoMethod object is not a method (missing receiver)")
		}
		recvTypeName := f.Func.Receiver.Type.Name
		pkgPath := f.Recv.PkgPath
		var fullRecvName string
		if strings.HasPrefix(recvTypeName, "*") {
			typeName := strings.TrimPrefix(recvTypeName, "*")
			fullRecvName = fmt.Sprintf("(*%s.%s)", pkgPath, typeName)
		} else {
			fullRecvName = fmt.Sprintf("(%s.%s)", pkgPath, recvTypeName)
		}
		return fmt.Sprintf("%s.%s", fullRecvName, f.Func.Name), nil

	default:
		return "", fmt.Errorf("unsupported function type for key computation: %T", fn)
	}
}

// unmarshalPatternConfig manually unmarshals an *object.StructInstance into a patterns.PatternConfig.
func unmarshalPatternConfig(structObj *object.StructInstance) (patterns.PatternConfig, error) {
	var config patterns.PatternConfig

	fnObj, ok := structObj.Fields["Fn"]
	if !ok {
		return config, fmt.Errorf("pattern struct is missing 'Fn' field")
	}
	config.Fn = fnObj

	if typeObj, ok := structObj.Fields["Type"]; ok {
		if s, ok := typeObj.(*object.String); ok {
			config.Type = patterns.PatternType(s.Value)
		}
	}
	if argIndexObj, ok := structObj.Fields["ArgIndex"]; ok {
		if i, ok := argIndexObj.(*object.Integer); ok {
			config.ArgIndex = int(i.Value)
		}
	}
	if nameArgIndexObj, ok := structObj.Fields["NameArgIndex"]; ok {
		if i, ok := nameArgIndexObj.(*object.Integer); ok {
			config.NameArgIndex = int(i.Value)
		}
	}
	if statusCodeObj, ok := structObj.Fields["StatusCode"]; ok {
		if s, ok := statusCodeObj.(*object.String); ok {
			config.StatusCode = s.Value
		}
	}
	if descriptionObj, ok := structObj.Fields["Description"]; ok {
		if s, ok := descriptionObj.(*object.String); ok {
			config.Description = s.Value
		}
	}
	if nameObj, ok := structObj.Fields["Name"]; ok {
		if s, ok := nameObj.(*object.String); ok {
			config.Name = s.Value
		}
	}
	if methodNameObj, ok := structObj.Fields["MethodName"]; ok {
		if s, ok := methodNameObj.(*object.String); ok {
			config.MethodName = s.Value
		}
	}

	return config, nil
}

// LoadPatternsFromSource loads custom analysis patterns from a Go configuration source.
func LoadPatternsFromSource(source []byte, logger *slog.Logger) ([]patterns.Pattern, error) {
	interp, err := minigo.NewInterpreter()
	if err != nil {
		return nil, fmt.Errorf("failed to create minigo interpreter: %w", err)
	}

	if _, err := interp.EvalString(string(source)); err != nil {
		return nil, fmt.Errorf("failed to evaluate patterns config source: %w", err)
	}

	patternsObj, ok := interp.GlobalEnvForTest().Get("Patterns")
	if !ok {
		return nil, fmt.Errorf("could not find 'Patterns' variable in config source")
	}

	slice, ok := patternsObj.(*object.Array)
	if !ok {
		return nil, fmt.Errorf("'Patterns' variable is not a slice, but %T", patternsObj)
	}

	var configs []patterns.PatternConfig
	for i, item := range slice.Elements {
		structObj, ok := item.(*object.StructInstance)
		if !ok {
			return nil, fmt.Errorf("pattern item %d is not a struct, but %T", i, item)
		}

		config, err := unmarshalPatternConfig(structObj)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal pattern item %d: %w", i, err)
		}
		configs = append(configs, config)
	}

	return convertConfigsToPatterns(configs, logger)
}

func convertConfigsToPatterns(configs []patterns.PatternConfig, logger *slog.Logger) ([]patterns.Pattern, error) {
	result := make([]patterns.Pattern, len(configs))
	for i, config := range configs {
		c := config

		key, err := computeKey(c.Fn)
		if err != nil {
			return nil, fmt.Errorf("pattern %d: %w", i, err)
		}

		switch c.Type {
		case patterns.RequestBody, patterns.ResponseBody, patterns.DefaultResponse:
		case patterns.CustomResponse:
			if c.StatusCode == "" {
				return nil, fmt.Errorf("pattern %d: 'StatusCode' is required for type %q", i, c.Type)
			}
		case patterns.PathParameter, patterns.QueryParameter, patterns.HeaderParameter:
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
			result[i].Apply = patterns.HandleCustomParameter(string(c.Type), c.Description, c.Name, c.NameArgIndex, c.ArgIndex)
		default:
			logger.Warn("unreachable: unknown pattern type", "type", c.Type, "key", key)
			return nil, fmt.Errorf("unknown pattern type %q for key %q", c.Type, key)
		}
		logger.Debug("loaded custom pattern", "key", key, "type", c.Type, "argIndex", c.ArgIndex)
	}
	return result, nil
}
