// Code generated by convert. DO NOT EDIT.
package tags

import (
	"context"
	"errors"
	"fmt"

	"github.com/podhmo/go-scan/examples/convert/model"
)

func convertSrcWithTagsToDstWithTags(ctx context.Context, ec *model.ErrorCollector, src *SrcWithTags) *DstWithTags {
	if src == nil {
		return nil
	}
	dst := &DstWithTags{}
	if ec.MaxErrorsReached() {
		return dst
	}
	ec.Enter("ID")
	dst.ID = src.ID
	ec.Leave()
	if ec.MaxErrorsReached() {
		return dst
	}
	ec.Enter("UserAge")
	dst.UserAge = src.Age
	ec.Leave()
	if ec.MaxErrorsReached() {
		return dst
	}
	ec.Enter("Profile")
	dst.Profile = convertProfile(ctx, src.Profile)
	ec.Leave()
	if ec.MaxErrorsReached() {
		return dst
	}
	ec.Enter("ManagerID")
	if src.ManagerID == nil {
		ec.Add(fmt.Errorf("ManagerID is required"))
	} else {
		dst.ManagerID = src.ManagerID // Cannot convert pointer types, element type is nil
	}
	ec.Leave()
	if ec.MaxErrorsReached() {
		return dst
	}
	ec.Enter("TeamID")
	if src.TeamID == nil {
		ec.Add(fmt.Errorf("TeamID is required"))
	} else {
		dst.TeamID = src.TeamID // Cannot convert pointer types, element type is nil
	}
	ec.Leave()
	return dst
}

// ConvertSrcWithTagsToDstWithTags converts SrcWithTags to DstWithTags.
func ConvertSrcWithTagsToDstWithTags(ctx context.Context, src *SrcWithTags) (*DstWithTags, error) {
	if src == nil {
		return nil, nil
	}
	ec := model.NewErrorCollector(1)
	dst := convertSrcWithTagsToDstWithTags(ctx, ec, src)
	if ec.HasErrors() {
		return dst, errors.Join(ec.Errors()...)
	}
	return dst, nil
}
