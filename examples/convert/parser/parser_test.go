package parser

import (
	"context"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/convert/model"
	"github.com/podhmo/go-scan/scanner"
)

// MockResolver is a mock implementation of PackageResolver for tests.
type MockResolver struct {
	ScanPackageFromImportPathFunc func(ctx context.Context, importPath string) (*scanner.PackageInfo, error)
}

func (m *MockResolver) ScanPackageFromImportPath(ctx context.Context, importPath string) (*scanner.PackageInfo, error) {
	if m.ScanPackageFromImportPathFunc != nil {
		return m.ScanPackageFromImportPathFunc(ctx, importPath)
	}
	return nil, nil // Default mock behavior
}

func TestParse(t *testing.T) {
	source := `
package sample

import "time"

// @derivingconvert(Destination)
type Source struct {
	ID   int
	Name string
	tags []string
}

type Destination struct {
	ID      int
	Name    string
	Tags    []string
	private bool
}

// @derivingconvert(DestinationWithOption, max_errors=5)
type SourceWithOption struct {
	ID int
}
type DestinationWithOption struct {
	ID int
}


// convert:rule "time.Time" -> "string", using=TimeToString
// convert:rule "string" -> "time.Time", using=StringToTime
type MyTime time.Time
`

	overlay := map[string][]byte{
		"source.go": []byte(source),
	}

	overrides := scanner.ExternalTypeOverride{
		"time.Time": {
			Name:    "Time",
			PkgPath: "time",
			Kind:    scanner.StructKind,
		},
	}

	// Create a temporary directory for the test to act as a module root.
	tmpdir, err := os.MkdirTemp("", "parser-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)
	if err := os.WriteFile(filepath.Join(tmpdir, "go.mod"), []byte("module example.com/sample\ngo 1.22"), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}
	srcPath := filepath.Join(tmpdir, "source.go")
	if err := os.WriteFile(srcPath, []byte(source), 0644); err != nil {
		t.Fatalf("failed to write source.go: %v", err)
	}

	s, err := goscan.New(
		goscan.WithWorkDir(tmpdir),
		goscan.WithExternalTypeOverrides(overrides),
		goscan.WithOverlay(overlay),
	)
	if err != nil {
		t.Fatalf("goscan.New() failed: %v", err)
	}

	pkg, err := s.ScanPackageFromFilePath(context.Background(), tmpdir)
	if err != nil {
		t.Fatalf("ScanPackageFromFilePath() failed: %v", err)
	}

	got, err := Parse(context.Background(), s, pkg)
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	want := &model.ParsedInfo{
		PackageName: "sample",
		PackagePath: "example.com/sample",
		Imports:     make(map[string]string),
		ConversionPairs: []model.ConversionPair{
			{SrcTypeName: "Source", DstTypeName: "Destination", MaxErrors: 0, Variables: nil},
			{SrcTypeName: "SourceWithOption", DstTypeName: "DestinationWithOption", MaxErrors: 5, Variables: nil},
		},
		GlobalRules: []model.TypeRule{
			{SrcTypeName: "time.Time", DstTypeName: "string", UsingFunc: "TimeToString"},
			{SrcTypeName: "string", DstTypeName: "time.Time", UsingFunc: "StringToTime"},
		},
		ProcessedPackages: map[string]bool{"example.com/sample": true},
		Structs: map[string]*model.StructInfo{
			"Source": {
				Name: "Source",
				Fields: []model.FieldInfo{
					{Name: "ID", OriginalName: "ID"},
					{Name: "Name", OriginalName: "Name"},
					{Name: "tags", OriginalName: "tags"},
				},
			},
			"Destination": {
				Name: "Destination",
				Fields: []model.FieldInfo{
					{Name: "ID", OriginalName: "ID"},
					{Name: "Name", OriginalName: "Name"},
					{Name: "Tags", OriginalName: "Tags"},
					{Name: "private", OriginalName: "private"},
				},
			},
			"SourceWithOption": {
				Name: "SourceWithOption",
				Fields: []model.FieldInfo{
					{Name: "ID", OriginalName: "ID"},
				},
			},
			"DestinationWithOption": {
				Name: "DestinationWithOption",
				Fields: []model.FieldInfo{
					{Name: "ID", OriginalName: "ID"},
				},
			},
		},
		NamedTypes: map[string]*scanner.TypeInfo{
			"Source":                {Name: "Source"},
			"Destination":           {Name: "Destination"},
			"SourceWithOption":      {Name: "SourceWithOption"},
			"DestinationWithOption": {Name: "DestinationWithOption"},
			"MyTime":                {Name: "MyTime"},
		},
	}

	// Set the Fset on the want NamedTypes to match the parsed package's Fset
	for _, ti := range want.NamedTypes {
		ti.Fset = pkg.Fset
	}

	opts := []cmp.Option{
		cmp.AllowUnexported(model.ParsedInfo{}, model.ConversionPair{}, model.StructInfo{}, model.FieldInfo{}),
		cmpopts.IgnoreUnexported(scanner.TypeInfo{}, token.FileSet{}),
		cmpopts.IgnoreFields(scanner.TypeInfo{}, "PkgPath", "FilePath", "Doc", "Kind", "Node", "Struct", "Func", "Interface", "Underlying", "TypeParams", "Inspect", "Logger", "ResolutionContext"),
		cmpopts.IgnoreFields(model.ParsedInfo{}, "NamedTypes", "Structs"), // check them separately
		cmpopts.IgnoreFields(model.ConversionPair{}, "SrcTypeInfo", "DstTypeInfo"),
		cmpopts.IgnoreFields(model.TypeRule{}, "SrcTypeInfo", "DstTypeInfo"),
		cmpopts.IgnoreFields(model.StructInfo{}, "Type", "IsAlias", "UnderlyingAlias"),
		cmpopts.IgnoreFields(model.FieldInfo{}, "JSONTag", "TypeInfo", "FieldType", "Tag", "ParentStruct"),
	}

	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
	}

	// check structs separately because cmp has trouble with recursive structures
	if diff := cmp.Diff(want.Structs, got.Structs, opts...); diff != "" {
		t.Errorf("Parse() Structs mismatch (-want +got):\n%s", diff)
	}
	// check named types separately
	if diff := cmp.Diff(want.NamedTypes, got.NamedTypes, opts...); diff != "" {
		t.Errorf("Parse() NamedTypes mismatch (-want +got):\n%s", diff)
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
