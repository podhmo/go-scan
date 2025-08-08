// Package commentof provides structures for holding extracted comment data.
package commentof

import "go/token"

// Field represents a single documented entity, such as a function parameter,
// a return value, or a struct field.
type Field struct {
	// Names holds the identifier(s) for the field. It's a slice because
	// declarations can be grouped, e.g., `x, y int`.
	Names []string `json:"names,omitempty"`

	// Type holds the string representation of the field's type.
	Type string `json:"type,omitempty"`

	// Doc contains the cleaned, combined documentation for this field.
	// It includes both preceding comments (Doc) and same-line comments (Comment).
	Doc string `json:"doc,omitempty"`
}

// Function holds the documentation for a single function declaration.
type Function struct {
	// Name is the name of the function.
	Name string `json:"name,omitempty"`

	// Doc contains the documentation associated with the function definition.
	Doc string `json:"doc,omitempty"`

	// Params is a list of documented function parameters.
	Params []*Field `json:"params,omitempty"`

	// Results is a list of documented function return values.
	Results []*Field `json:"results,omitempty"`
}

// Struct holds the documentation for a struct type.
type Struct struct {
	// Fields is a list of documented fields within the struct.
	Fields []*Field `json:"fields,omitempty"`
}

// TypeSpec holds the documentation for a type declaration (type definition or alias).
type TypeSpec struct {
	// Name is the name of the type.
	Name string `json:"name,omitempty"`

	// Doc contains the documentation associated with the type specification.
	Doc string `json:"doc,omitempty"`

	// Definition holds the detailed definition of the type, which could be
	// a Struct or another type definition.
	Definition interface{} `json:"definition,omitempty"`
}

// ValueSpec holds the documentation for a const or var declaration.
type ValueSpec struct {
	// Names holds the identifier(s) for the constant(s) or variable(s).
	Names []string `json:"names,omitempty"`

	// Doc contains the documentation associated with the value specification.
	Doc string `json:"doc,omitempty"`

	// Kind is the token type of the declaration (e.g., token.CONST, token.VAR).
	Kind token.Token `json:"kind,omitempty"`
}