package converters

import (
	"context"
	"time"
)

func TimeToString(ctx context.Context, t time.Time) string {
	return t.Format(time.RFC3339)
}
