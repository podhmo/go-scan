package fixture

import (
	"context"
)

// F is function @FUN0
func F(x int, y string, args ...interface{}) (string, error) {
	// inner function :IGNORED:
	return "", nil
} // F is function @FUN1 :IGNORED:

// F2 is function @FUN2
func F2(
	x int, // x is int @arg1
	y string, // y is int @arg2
	args ...interface{}, // args is int @arg3
) (string, // result of F2 @ret1
	error, // error of F2 @ret2
) {
	return "", nil
}

// F3 is function @FUN3
func F3(
	context.Context,
	string,
	...interface{},
) (result string, err error) {
	return "", nil
}

// F4 is function @FUN4
func F4(x int /* x of F4 @arg4 */ /* x of F4 @arg5 */, y /* y of F4 @arg6 */ string /* y of F4 @arg7 */, args ...interface{} /* arg of F4 @arg8 */) ( /* result if F4 @ret3 */ string /* result if F4 @ret4 */ /* ret of F4 @ret5 */ /* err of F4 @ret6 */, error /* err of F4 @ret7 */) {
	return "", nil
}

// F5 is function @FUN5
func F5() {

}

// F6 is function (anonymous) @FUN6
var F6 = func(x int, y int) error { return nil }

// F7 is function @FUN7
func F7(ctx context.Context, x, y int, z string) (x, y error) {
	return
}

// F8 is function @FUN8
func F8(
	ctx context.Context,
	x, y int,
	pretty *bool, // pretty output or not
) []int /* ret */ {
	return nil
}

// F9 is function @FUN9
func F9(
	ctx context.Context, x, y int, pretty *bool /* pretty output or not */) ([]int /* ret */, error /* error */) {
	return nil, nil
}