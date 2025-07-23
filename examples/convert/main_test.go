package main

import (
	"context"
	"go/format"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
)

func TestGenerate(t *testing.T) {
	type want struct {
		Code string
	}
	cases := []struct {
		name  string
		files map[string]string
		want  want
	}{
		{
			name: "simple",
			files: map[string]string{
				"go.mod": `
module example.com/convert
go 1.22.4
`,
				"models/source/source.go": `
package source

import "time"

// @derivingconvert("example.com/convert/models/destination.DstUser")
type SrcUser struct {
	ID        int64
	FirstName string
	LastName  string
	SrcAddress
	ContactInfo SrcContact
	Details     []SrcInternalDetail
	CreatedAt   time.Time
	UpdatedAt   *time.Time
}

type SrcAddress struct {
	Street string
	City   string
}

type SrcContact struct {
	Email string
	Phone *string
}

type SrcInternalDetail struct {
	Code        int
	Description string
}

// @derivingconvert("example.com/convert/models/destination.DstOrder")
type SrcOrder struct {
	OrderID string
	Amount  float64
	Items   []SrcItem
}

type SrcItem struct {
	SKU      string
	Quantity int
}
`,
				"models/destination/destination.go": `
package destination

type DstUser struct {
	UserID    string
	FullName  string
	Address   DstAddress
	Contact   DstContact
	Details   []DstInternalDetail
	CreatedAt string
	UpdatedAt string
}

type DstAddress struct {
	FullStreet string
	CityName   string
}

type DstContact struct {
	EmailAddress string
	PhoneNumber  string
}

type DstInternalDetail struct {
	ItemCode      int
	LocalizedDesc string
}

type DstOrder struct {
	ID          string
	TotalAmount float64
	LineItems   []DstItem
}

type DstItem struct {
	ProductCode string
	Count       int
}
`,
			},
			want: want{
				Code: `
package converter

import (
	"context"
	"example.com/convert/models/destination"
	"example.com/convert/models/source"
)

// ConvertSrcUserToDstUser converts SrcUser to DstUser.
func ConvertSrcUserToDstUser(ctx context.Context, src source.SrcUser) (destination.DstUser, error) {
	dst := convertSrcUserToDstUser(ctx, src)
	return dst, nil
}

func convertSrcUserToDstUser(ctx context.Context, src source.SrcUser) destination.DstUser {
	var dst destination.DstUser
	dst.Details = src.Details
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
	return dst
}

// ConvertSrcOrderToDstOrder converts SrcOrder to DstOrder.
func ConvertSrcOrderToDstOrder(ctx context.Context, src source.SrcOrder) (destination.DstOrder, error) {
	dst := convertSrcOrderToDstOrder(ctx, src)
	return dst, nil
}

func convertSrcOrderToDstOrder(ctx context.Context, src source.SrcOrder) destination.DstOrder {
	var dst destination.DstOrder
	return dst
}
`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpdir, cleanup := scantest.WriteFiles(t, tc.files)
			defer cleanup()

			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*scanner.PackageInfo) error {
				for _, pkg := range pkgs {
					if err := Generate(ctx, s, pkg); err != nil {
						return err
					}
				}
				return nil
			}

			result, err := scantest.Run(t, tmpdir, []string{"./models/source"}, action)
			if err != nil {
				t.Fatalf("scantest.Run failed: %+v", err)
			}

			if result == nil {
				t.Fatal("scantest.Run result is nil")
			}
			if len(result.Outputs) != 1 {
				t.Fatalf("unexpected number of outputs, got %d, want 1", len(result.Outputs))
			}

			var got string
			// The output file name is 'generated_converters.go'.
			filename := "generated_converters.go"
			gotBytes, ok := result.Outputs[filename]
			if !ok {
				// Collect available keys for a better error message
				availableKeys := make([]string, 0, len(result.Outputs))
				for k := range result.Outputs {
					availableKeys = append(availableKeys, k)
				}
				t.Fatalf("expected output file %q not found; available files: %v", filename, availableKeys)
			}
			got = string(gotBytes)

			// Format both got and want code for a consistent comparison
			formattedGot, err := format.Source([]byte(got))
			if err != nil {
				t.Logf("failed to format generated code: %+v\n--- raw output ---\n%s", err, got)
				// Fallback to comparing raw strings if formatting fails
				formattedGot = []byte(got)
			}


			formattedWant, err := format.Source([]byte(tc.want.Code))
			if err != nil {
				t.Fatalf("failed to format want code: %+v", err)
			}

			if diff := cmp.Diff(strings.TrimSpace(string(formattedWant)), strings.TrimSpace(string(formattedGot))); diff != "" {
				t.Errorf("generated code mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
