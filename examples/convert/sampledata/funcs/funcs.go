package funcs

import (
	"context"
	"time"

	"github.com/podhmo/go-scan/examples/convert/model"
	"github.com/podhmo/go-scan/examples/convert/sampledata/destination"
	"github.com/podhmo/go-scan/examples/convert/sampledata/source"
)

// ConvertTimeToString provides a global rule for converting time.Time to string.
func ConvertTimeToString(ctx context.Context, ec *model.ErrorCollector, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// ConvertPtrTimeToString provides a specific converter for *time.Time to string.
func ConvertPtrTimeToString(ctx context.Context, ec *model.ErrorCollector, t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// ConvertSliceOfPtrs provides a specific converter for []*source.SubSource to []*destination.SubTarget.
func ConvertSliceOfPtrs(ctx context.Context, ec *model.ErrorCollector, s []*source.SubSource) []*destination.SubTarget {
	if s == nil {
		return nil
	}
	dst := make([]*destination.SubTarget, len(s))
	for i, item := range s {
		if item == nil {
			dst[i] = nil
			continue
		}
		// This conversion would ideally be generated, but we call it manually here for the example.
		// In a real scenario, you might have a ConvertSubSourceToSubTarget function available.
		dst[i] = &destination.SubTarget{Value: item.Value}
	}
	return dst
}

// ConvertMapOfPtrs provides a specific converter for map[string]*source.SubSource to map[string]*destination.SubTarget.
func ConvertMapOfPtrs(ctx context.Context, ec *model.ErrorCollector, m map[string]*source.SubSource) map[string]*destination.SubTarget {
	if m == nil {
		return nil
	}
	dst := make(map[string]*destination.SubTarget, len(m))
	for k, v := range m {
		if v == nil {
			dst[k] = nil
			continue
		}
		dst[k] = &destination.SubTarget{Value: v.Value}
	}
	return dst
}
