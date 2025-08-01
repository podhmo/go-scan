package parser

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/convert/model"
	"github.com/podhmo/go-scan/scantest"
)

func TestParse(t *testing.T) {
	source := `
package sample
import "time"

// @derivingconvert("Destination")
type Source struct {
	ID   int
	Name string
}

type Destination struct {
	ID   int
	Name string
}

// convert:rule "time.Time" -> "string", using=TimeToString
`
	files := map[string]string{
		"go.mod":  "module example.com/m\ngo 1.24",
		"main.go": source,
	}

	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(tmpdir))
	if err != nil {
		t.Fatalf("goscan.New() failed: %v", err)
	}

	pkg, err := s.ScanPackage(context.Background(), tmpdir)
	if err != nil {
		t.Fatalf("ScanPackage() failed: %v", err)
	}

	got, err := Parse(context.Background(), s, pkg)
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	// Assertions
	if got.PackageName != "sample" {
		t.Errorf("expected package name 'sample', got %q", got.PackageName)
	}
	if len(got.ConversionPairs) != 1 {
		t.Fatalf("expected 1 conversion pair, got %d", len(got.ConversionPairs))
	}
	if len(got.GlobalRules) != 1 {
		t.Fatalf("expected 1 global rule, got %d", len(got.GlobalRules))
	}

	rule := got.GlobalRules[0]
	if rule.SrcTypeName != "time.Time" {
		t.Errorf("expected rule source type name to be 'time.Time', got %q", rule.SrcTypeName)
	}
	if rule.SrcTypeInfo == nil {
		t.Fatal("expected rule SrcTypeInfo to be resolved, but it was nil")
	}
	if rule.SrcTypeInfo.Name != "Time" {
		t.Errorf("expected resolved type name to be 'Time', got %q", rule.SrcTypeInfo.Name)
	}
	if rule.SrcTypeInfo.PkgPath != "time" {
		t.Errorf("expected resolved type package path to be 'time', got %q", rule.SrcTypeInfo.PkgPath)
	}
}

func TestParseConvertTag(t *testing.T) {
	cases := []struct {
		name    string
		tag     string
		want    model.ConvertTag
		wantErr bool
	}{
		{name: "empty", tag: ``, want: model.ConvertTag{RawValue: ""}},
		{name: "only destination name", tag: `convert:"DestName"`, want: model.ConvertTag{RawValue: `DestName`, DstFieldName: "DestName"}},
		{name: "skip field", tag: `convert:"-"`, want: model.ConvertTag{RawValue: `-`, DstFieldName: "-"}},
		{name: "only using", tag: `convert:",using=myFunc"`, want: model.ConvertTag{RawValue: `,using=myFunc`, UsingFunc: "myFunc"}},
		{name: "destination name and using", tag: `convert:"DestName,using=myFunc"`, want: model.ConvertTag{RawValue: `DestName,using=myFunc`, DstFieldName: "DestName", UsingFunc: "myFunc"}},
		{name: "only required", tag: `convert:",required"`, want: model.ConvertTag{RawValue: `,required`, Required: true}},
		{name: "destination name and required", tag: `convert:"DestName,required"`, want: model.ConvertTag{RawValue: `DestName,required`, DstFieldName: "DestName", Required: true}},
		{name: "using and required", tag: `convert:",using=myFunc,required"`, want: model.ConvertTag{RawValue: `,using=myFunc,required`, UsingFunc: "myFunc", Required: true}},
		{name: "all options", tag: `convert:"DestName,using=myFunc,required"`, want: model.ConvertTag{RawValue: `DestName,using=myFunc,required`, DstFieldName: "DestName", UsingFunc: "myFunc", Required: true}},
		{name: "all options with spaces", tag: `convert:" DestName , using=myFunc , required "`, want: model.ConvertTag{RawValue: ` DestName , using=myFunc , required `, DstFieldName: "DestName", UsingFunc: "myFunc", Required: true}},
		{name: "malformed using", tag: `convert:"DestName,using="`, want: model.ConvertTag{RawValue: `DestName,using=`, DstFieldName: "DestName"}},
		{name: "just a comma", tag: `convert:","`, want: model.ConvertTag{RawValue: `,`}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tag := reflect.StructTag(tc.tag)
			got, err := parseConvertTag(tag)

			if (err != nil) != tc.wantErr {
				t.Fatalf("parseConvertTag() error = %v, wantErr %v", err, tc.wantErr)
			}

			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(model.ConvertTag{})); diff != "" {
				t.Errorf("parseConvertTag() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
