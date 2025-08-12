// Package define provides the API for defining conversion rules.
package define

// Mapping is a placeholder for mapping configuration.
type Mapping struct{}

// Config is the configurator for defining field-level mapping exceptions.
type Config struct{}

// Convert defines a conversion between two struct types, specifying custom mapping logic.
// The src and dst parameters are zero-value expressions of the source and destination structs.
// The mapping parameter defines the exceptional mapping rules.
func Convert(src any, dst any, mapping Mapping) {
	// This is a stub function for the parser.
}

// Rule defines a global, reusable conversion rule for a specific type-to-type conversion.
// The customFunc parameter is a function identifier (e.g., convutil.TimeToString).
func Rule(customFunc any) {
	// This is a stub function for the parser.
}

// NewMapping creates the mapping configuration for a Convert call.
// The mapFunc parameter is a function literal with a specific signature,
// e.g., func(c *Config, dst *DestType, src *SrcType).
func NewMapping(mapFunc any) Mapping {
	// This is a stub function for the parser.
	return Mapping{}
}

func (c *Config) Map(dstField any, srcField any) {
	// This is a stub function for the parser.
}
func (c *Config) Convert(
	dstField any, srcField any,
	convertFunc any, /* func(ctx context.Context, ec *model.ErrorCollector, src any) any */
) {
	// This is a stub function for the parser.
}
func (c *Config) Compute(
	dstField any,
	computeFunc any,
) {
	// This is a stub function for the parser.
}
