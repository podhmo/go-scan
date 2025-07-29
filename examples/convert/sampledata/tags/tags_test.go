package tags

import (
	"context"
	"strings"
	"testing"
)

func TestConvertWithTags_MaxErrors(t *testing.T) {
	ctx := context.Background()
	src := SrcWithTags{
		ID:        "test-id",
		Name:      "should-be-skipped",
		Age:       30,
		Profile:   "test-profile",
		ManagerID: nil, // First required field that is nil
		TeamID:    nil, // Second required field that is nil
	}

	_, err := ConvertSrcWithTagsToDstWithTags(ctx, &src)

	if err == nil {
		t.Fatalf("expected an error, but got nil")
	}

	// We expect only one error because max_errors=1 is set in the annotation.
	errorString := err.Error()
	if got := strings.Count(errorString, "\n") + 1; got > 1 {
		t.Errorf("expected 1 error, but got %d. error: %s", got, errorString)
	}

	if !strings.Contains(errorString, "ManagerID is required") {
		t.Errorf("expected error message to contain 'ManagerID is required', but it was: %s", errorString)
	}
	if strings.Contains(errorString, "TeamID is required") {
		t.Errorf("expected error message NOT to contain 'TeamID is required' due to max_errors=1, but it was: %s", errorString)
	}
}
