package main

import (
	"fmt"
	"log/slog"
	"os"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"github.com/podhmo/go-scan/minigo"
)

// LoadPatternsFromConfig loads custom analysis patterns from a Go configuration file.
// It is a wrapper around LoadPatternsFromSource.
func LoadPatternsFromConfig(filePath string, logger *slog.Logger, s *goscan.Scanner) ([]patterns.Pattern, error) {
	configSource, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read patterns config file %q: %w", filePath, err)
	}
	return LoadPatternsFromSource(configSource, logger, s)
}

// LoadPatternsFromSource loads custom analysis patterns from a Go configuration source.
func LoadPatternsFromSource(source []byte, logger *slog.Logger, s *goscan.Scanner) ([]patterns.Pattern, error) {
	// Step 1: Set up the minigo interpreter.
	// We pass the host's scanner to the interpreter. This ensures that the interpreter
	// uses the same context (including module information, visited files, etc.) as the host tool.
	interp, err := minigo.NewInterpreter(minigo.WithScanner(s))
	if err != nil {
		return nil, fmt.Errorf("failed to create minigo interpreter: %w", err)
	}

	// Step 2: Evaluate the script.
	if _, err := interp.EvalString(string(source)); err != nil {
		return nil, fmt.Errorf("failed to evaluate patterns config source: %w", err)
	}

	// Step 3: Extract the 'Patterns' variable from the global environment.
	patternsObj, ok := interp.GlobalEnvForTest().Get("Patterns")
	if !ok {
		return nil, fmt.Errorf("could not find 'Patterns' variable in config source")
	}

	// Step 4: Unmarshal the minigo object directly into a slice of PatternConfig structs.
	// This is now possible because the script returns a typed slice instead of []map[string]any.
	var configs []patterns.PatternConfig
	result := minigo.Result{Value: patternsObj}
	if err := result.As(&configs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal 'Patterns' variable into []patterns.PatternConfig: %w", err)
	}

	// Step 5: Convert the data-only configs into executable patterns.
	return convertConfigsToPatterns(configs, logger)
}

// convertConfigsToPatterns translates the user-defined pattern configurations
// into the internal Pattern format with executable Apply functions.
func convertConfigsToPatterns(configs []patterns.PatternConfig, logger *slog.Logger) ([]patterns.Pattern, error) {
	result := make([]patterns.Pattern, len(configs))
	for i, config := range configs {
		c := config // capture loop variable

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
			return nil, fmt.Errorf("pattern %d: unknown 'Type' value %q for key %q", i, c.Type, c.Key)
		}

		result[i].Key = c.Key

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
			logger.Warn("unreachable: unknown pattern type", "type", c.Type, "key", c.Key)
			return nil, fmt.Errorf("unknown pattern type %q for key %q", c.Type, c.Key)
		}
		logger.Debug("loaded custom pattern", "key", c.Key, "type", c.Type, "argIndex", c.ArgIndex)
	}
	return result, nil
}
