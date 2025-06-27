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

// Int8 is a parser for the int8 type.
func Int8(s string) (int8, error) {
	n, err := strconv.ParseInt(s, 10, 8)
	if err != nil {
		return 0, err
	}
	return int8(n), nil
}

// Int16 is a parser for the int16 type.
func Int16(s string) (int16, error) {
	n, err := strconv.ParseInt(s, 10, 16)
	if err != nil {
		return 0, err
	}
	return int16(n), nil
}

// Int32 is a parser for the int32 type.
func Int32(s string) (int32, error) {
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

// Int64 is a parser for the int64 type.
func Int64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// Uint is a parser for the uint type.
func Uint(s string) (uint, error) {
	n, err := strconv.ParseUint(s, 10, 0) // 0 means infer bit size from type
	if err != nil {
		return 0, err
	}
	return uint(n), nil
}

// Uint8 is a parser for the uint8 type.
func Uint8(s string) (uint8, error) {
	n, err := strconv.ParseUint(s, 10, 8)
	if err != nil {
		return 0, err
	}
	return uint8(n), nil
}

// Uint16 is a parser for the uint16 type.
func Uint16(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(n), nil
}

// Uint32 is a parser for the uint32 type.
func Uint32(s string) (uint32, error) {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(n), nil
}

// Uint64 is a parser for the uint64 type.
func Uint64(s string) (uint64, error) {
	return strconv.ParseUint(s, 10, 64)
}

// Float32 is a parser for the float32 type.
func Float32(s string) (float32, error) {
	n, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return 0, err
	}
	return float32(n), nil
}

// Uintptr is a parser for the uintptr type.
func Uintptr(s string) (uintptr, error) {
	n, err := strconv.ParseUint(s, 10, 64) // uintptr size is architecture-dependent, parse as uint64 then cast
	if err != nil {
		return 0, err
	}
	return uintptr(n), nil
}

// Complex64 is a parser for the complex64 type.
// If s is an empty string, it returns complex(0,0) and no error.
func Complex64(s string) (complex64, error) {
	if s == "" {
		return complex(0, 0), nil
	}
	c, err := strconv.ParseComplex(s, 64)
	if err != nil {
		return 0, err
	}
	return complex64(c), nil
}

// Complex128 is a parser for the complex128 type.
// If s is an empty string, it returns complex(0,0) and no error.
func Complex128(s string) (complex128, error) {
	if s == "" {
		return complex(0, 0), nil
	}
	return strconv.ParseComplex(s, 128)
}
