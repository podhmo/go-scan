package fixture

// toplevel comment 0  :IGNORED:

// S is struct @S0
type S struct {
	// in struct comment 0  :IGNORED:

	// ExportedString is exported string @F0
	ExportedString string

	// in struct comment 1  :IGNORED:

	ExportedString2 string // ExportedString2 is exported string @F1

	// ExportedString3 is exported string @F2
	ExportedString3 string // ExportedString3 is exported string @F3

	// Nested is struct @SS0
	Nested struct { // in struct comment 2  :IGNORED:

		// ExportedString is exported string @FF0
		ExportedString string // ExportedString is exported string @FF1

		// in struct comment 3  :IGNORED:
	} // Nested is struct @SS1
	// in struct comment 4  :IGNORED:

	// unexportedString is unexported string @U1  :IGNORED:
	unexportedString string
} // S is struct @S1

// S2 is struct @S2
type S2 S

// S3 is struct @S3
type S3 = S