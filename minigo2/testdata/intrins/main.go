package main

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Sprintf runs the real fmt.Sprintf via intrinsics — no GOROOT parse.
func Sprintf() string { return fmt.Sprintf("hi %d", 7) }

// StrconvAtoi exercises the (value, err) tuple convention.
func StrconvAtoi() int {
	n, err := strconv.Atoi("42")
	if err != nil {
		return -1
	}
	return n
}

// StringsJoin uses the native strings.Join.
func StringsJoin() string { return strings.Join([]string{"a", "b", "c"}, ",") }

// ErrorsNew wraps errors.New; Error() returns the message via method call.
func ErrorsNew() string {
	err := errors.New("oops")
	if err == nil {
		return "no error"
	}
	return err.Error()
}

// SortIntsInPlace: sort.Ints mutates the caller's slice, not a copy.
func SortIntsInPlace() int {
	s := []int{3, 1, 2}
	sort.Ints(s)
	return s[0]*100 + s[1]*10 + s[2] // 123
}

// SlicesSortInPlace: slices.Sort mutates the caller's slice.
func SlicesSortInPlace() string {
	s := []string{"b", "a", "c"}
	slices.Sort(s)
	return s[0] + s[1] + s[2] // "abc"
}

func main() {}
