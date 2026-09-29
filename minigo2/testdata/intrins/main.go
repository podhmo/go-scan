package main

import (
	"errors"
	"fmt"
	goruntime "runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unsafe"
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

// Prints writes to the engine's configured output (WithOutput) — the
// assertion lives in the test, which captures the buffer.
func Prints() int {
	fmt.Println("hello", 42)
	return 0
}

// SortSearch finds the first index satisfying the predicate.
func SortSearch() int {
	return sort.Search(10, func(i int) bool { return i*i >= 30 }) // 6
}

// SortStableByLen: equal-length elements keep their input order.
func SortStableByLen() string {
	s := []string{"bb", "a", "cc", "d"}
	sort.SliceStable(s, func(i, j int) bool { return len(s[i]) < len(s[j]) })
	return s[0] + s[1] + s[2] + s[3] // "adbbcc" — bb stays before cc
}

// SortStableFuncByLen exercises slices.SortStableFunc with a script cmp.
func SortStableFuncByLen() string {
	s := []string{"bb", "a", "cc", "d"}
	slices.SortStableFunc(s, func(a, b string) int { return len(a) - len(b) })
	return s[0] + s[1] + s[2] + s[3] // "adbbcc"
}

// BinarySearchHit exercises the (index, found) tuple.
func BinarySearchHit() int {
	i, ok := slices.BinarySearch([]int{1, 3, 5, 7}, 5)
	if !ok {
		return -1
	}
	return i // 2
}

// BinarySearchMiss returns the insertion point when absent.
func BinarySearchMiss() int {
	i, ok := slices.BinarySearch([]int{1, 3, 5, 7}, 4)
	if ok {
		return -1
	}
	return i // 2
}

// BinarySearchFunc uses a script comparator.
func BinarySearchFunc() int {
	i, ok := slices.BinarySearchFunc([]string{"a", "bb", "ccc"}, "zzzz",
		func(x, t string) int { return len(x) - len(t) })
	if ok {
		return -1
	}
	return i // 3
}

// RuntimeGOOS reports the host GOOS via intrinsics — non-empty everywhere.
func RuntimeGOOS() bool { return goruntime.GOOS != "" }

// RuntimeGoroutines is pinned to 1: the VM is single-threaded by design.
func RuntimeGoroutines() int { return goruntime.NumGoroutine() }

// RuntimeGOMAXPROCS honors the setter argument and returns the new value.
func RuntimeGOMAXPROCS() int {
	old := goruntime.GOMAXPROCS(2)
	cur := goruntime.GOMAXPROCS(0)
	goruntime.GOMAXPROCS(old) // restore
	return cur                // 2
}

// UnsafeSizeofInt approximates unsafe.Sizeof over the boxed value.
func UnsafeSizeofInt() int { return int(unsafe.Sizeof(int64(0))) } // 8

// UnsafeSizeofSlice reports the 3-word slice header approximation.
func UnsafeSizeofSlice() int { return int(unsafe.Sizeof([]int{})) } // 24

// UnsafeAlignofEmpty: alignment is at least 1 even for the empty struct.
func UnsafeAlignofEmpty() int { return int(unsafe.Alignof(struct{}{})) } // 1

func main() {}
