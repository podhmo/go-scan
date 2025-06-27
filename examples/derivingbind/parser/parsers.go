// Package parser provides a set of standard parser functions for use with
// the binding package. These parser convert strings to common Go types.
package parser

import (
	"strconv"
)

// String is a parser for the string type.
// It always succeeds and returns the input string as is.
func String(s string) (string, error) {
	return s, nil
}

// Int is a parser for the int type.
// It uses strconv.Atoi for conversion.
func Int(s string) (int, error) {
	return strconv.Atoi(s)
}

// Bool is a parser for the bool type.
// It uses strconv.ParseBool, which accepts "1", "t", "T", "TRUE", "true", "True",
// "0", "f", "F", "FALSE", "false", "False".
func Bool(s string) (bool, error) {
	return strconv.ParseBool(s)
}

// Float64 is a parser for the float64 type.
// It uses strconv.ParseFloat for conversion.
func Float64(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
