package model

// ParsedInfo holds all parsed conversion rules and type information.
type ParsedInfo struct {
	PackageName     string
	PackagePath     string // Import path of the package being parsed
	ConversionPairs []ConversionPair
}

// ConversionPair defines a top-level conversion between two types.
// Corresponds to: // convert:pair <SrcType> -> <DstType>
type ConversionPair struct {
	SrcTypeName string
	DstTypeName string
}
