package symgotest

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/symgo/object"
)

// AssertSuccess fails the test if the RunResult contains an error.
func AssertSuccess(t *testing.T, result *RunResult) {
	t.Helper()
	if result.Error != nil {
		t.Fatalf("expected success, but got runtime error: %v", result.Error)
	}
	if err, ok := result.ReturnValue.(*object.Error); ok {
		t.Fatalf("expected success, but got error return value: %s", err.Message)
	}
}

// AssertError fails the test if the RunResult does not contain an error.
// If `contains` has one or more elements, it also checks if the error message contains each of them.
func AssertError(t *testing.T, result *RunResult, contains ...string) {
	t.Helper()

	var errMsg string
	if result.Error != nil {
		errMsg = result.Error.Error()
	} else if err, ok := result.ReturnValue.(*object.Error); ok {
		errMsg = err.Message
	} else {
		t.Fatalf("expected an error, but got successful result with return value: %T", result.ReturnValue)
	}

	for _, c := range contains {
		if !strings.Contains(errMsg, c) {
			t.Errorf("error message %q does not contain %q", errMsg, c)
		}
	}
}

// AssertCalled fails the test if the given function name is not in the list of called functions.
// This assertion is only useful when TrackCalls() is enabled on the Runner.
func AssertCalled(t *testing.T, result *RunResult, functionName string) {
	t.Helper()
	for _, called := range result.FunctionsCalled {
		if called == functionName {
			return // success
		}
	}
	t.Errorf("expected function %q to be called, but it was not.", functionName)
	if len(result.FunctionsCalled) > 0 {
		t.Logf("Functions called:\n - %s", strings.Join(result.FunctionsCalled, "\n - "))
	} else {
		t.Log("No functions were tracked as called.")
	}
}

// AssertNotCalled fails the test if the given function name is found in the list of called functions.
// This assertion is only useful when TrackCalls() is enabled on the Runner.
func AssertNotCalled(t *testing.T, result *RunResult, functionName string) {
	t.Helper()
	for _, called := range result.FunctionsCalled {
		if called == functionName {
			t.Errorf("expected function %q NOT to be called, but it was.", functionName)
			return
		}
	}
}

// AssertEqual uses go-cmp to compare two values and fails the test if they are not equal.
func AssertEqual(t *testing.T, want, got any, opts ...cmp.Option) {
	t.Helper()
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Errorf("values are not equal (-want +got):\n%s", diff)
	}
}
