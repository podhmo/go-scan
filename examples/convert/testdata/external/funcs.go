package external

import (
	"context"
	"fmt"
	"time"

	"github.com/podhmo/go-scan/examples/convert/model"
)

func TimeToString(ctx context.Context, t time.Time) string {
	return t.Format(time.RFC3339)
}

func StringToTime(ctx context.Context, s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func ValidateString(ctx context.Context, ec *model.ErrorCollector, s string) {
	if s == "" {
		ec.Add(fmt.Errorf("string is empty"))
	}
}
