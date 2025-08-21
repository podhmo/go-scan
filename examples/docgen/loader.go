package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"github.com/podhmo/go-scan/minigo"
)

// LoadPatternsFromConfig loads custom analysis patterns from a Go configuration file.
// It is a wrapper around LoadPatternsFromSource.
func LoadPatternsFromConfig(filePath string, logger *slog.Logger) ([]patterns.Pattern, error) {
	configSource, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read patterns config file %q: %w", filePath, err)
	}
	return LoadPatternsFromSource(configSource, logger)
}

// LoadPatternsFromSource loads custom analysis patterns from a Go configuration source.
func LoadPatternsFromSource(source []byte, logger *slog.Logger) ([]patterns.Pattern, error) {
	// Step 1: Set up the minigo interpreter.
	interp, err := minigo.NewInterpreter()
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

	// Step 4: Unmarshal the minigo object into a Go slice of maps.
	var mapConfigs []map[string]any
	result := minigo.Result{Value: patternsObj}
	if err := result.As(&mapConfigs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal 'Patterns' variable from config: %w", err)
	}

	// Step 5: Manually convert the maps to PatternConfig structs.
	configs, err := convertMapsToPatternConfigs(mapConfigs)
	if err != nil {
		return nil, fmt.Errorf("error in pattern config structure: %w", err)
	}

	// Step 6: Convert the data-only configs into executable patterns.
	return convertConfigsToPatterns(configs, logger)
}

func convertMapsToPatternConfigs(mapConfigs []map[string]any) ([]patterns.PatternConfig, error) {
	configs := make([]patterns.PatternConfig, len(mapConfigs))
	for i, m := range mapConfigs {
		key, ok := m["Key"].(string)
		if !ok {
			return nil, fmt.Errorf("pattern %d: 'Key' must be a string", i)
		}
		typStr, ok := m["Type"].(string)
		if !ok {
			// This also handles the case where minigo passes the enum value directly.
			// The underlying type of the enum is string, so we can cast it.
			typ, ok := m["Type"].(patterns.PatternType)
			if !ok {
				return nil, fmt.Errorf("pattern %d: 'Type' must be a string or patterns.PatternType", i)
			}
			typStr = string(typ)
		}
		argIndex, ok := m["ArgIndex"].(int64) // minigo unmarshals numbers as int64
		if !ok {
			return nil, fmt.Errorf("pattern %d: 'ArgIndex' must be an integer", i)
		}

		// Validate the pattern type string.
		switch patterns.PatternType(typStr) {
		case patterns.RequestBody, patterns.ResponseBody:
			// valid
		default:
			return nil, fmt.Errorf("pattern %d: unknown 'Type' value %q", i, typStr)
		}

		configs[i] = patterns.PatternConfig{
			Key:      key,
			Type:     patterns.PatternType(typStr),
			ArgIndex: int(argIndex),
		}
	}
	return configs, nil
}

// convertConfigsToPatterns translates the user-defined pattern configurations
// into the internal Pattern format with executable Apply functions.
func convertConfigsToPatterns(configs []patterns.PatternConfig, logger *slog.Logger) ([]patterns.Pattern, error) {
	result := make([]patterns.Pattern, len(configs))
	for i, config := range configs {
		c := config // capture loop variable
		result[i].Key = c.Key

		switch c.Type {
		case patterns.RequestBody:
			result[i].Apply = patterns.HandleCustomRequestBody(c.ArgIndex)
		case patterns.ResponseBody:
			result[i].Apply = patterns.HandleCustomResponseBody(c.ArgIndex)
		default:
			// This case should be unreachable due to validation in convertMapsToPatternConfigs
			return nil, fmt.Errorf("unknown pattern type %q for key %q", c.Type, c.Key)
		}
		logger.Debug("loaded custom pattern", "key", c.Key, "type", c.Type, "argIndex", c.ArgIndex)
	}
	return result, nil
}
