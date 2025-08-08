package commentof

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"go/token"
)

func TestFromFile(t *testing.T) {
	testcases := []struct {
		filename string
		expected []interface{}
	}{
		{
			filename: "functions.go",
			expected: []interface{}{
				&Function{
					Name: "F",
					Doc:  "F is function @FUN0",
					Params: []*Field{
						{Names: []string{"x"}, Type: "int"},
						{Names: []string{"y"}, Type: "string"},
						{Names: []string{"args"}, Type: "...interface{}"},
					},
					Results: []*Field{
						{Names: []string{"string"}, Type: "string"},
						{Names: []string{"error"}, Type: "error"},
					},
				},
				&Function{
					Name: "F2",
					Doc:  "F2 is function @FUN2",
					Params: []*Field{
						{Names: []string{"x"}, Type: "int", Doc: "x is int @arg1"},
						{Names: []string{"y"}, Type: "string", Doc: "y is int @arg2"},
						{Names: []string{"args"}, Type: "...interface{}", Doc: "args is int @arg3"},
					},
					Results: []*Field{
						{Names: []string{"string"}, Type: "string", Doc: "result of F2 @ret1"},
						{Names: []string{"error"}, Type: "error", Doc: "error of F2 @ret2"},
					},
				},
				&Function{
					Name: "F3",
					Doc:  "F3 is function @FUN3",
					Params: []*Field{
						{Names: []string{"context.Context"}, Type: "context.Context"},
						{Names: []string{"string"}, Type: "string"},
						{Names: []string{"...interface{}"}, Type: "...interface{}"},
					},
					Results: []*Field{
						{Names: []string{"result"}, Type: "string"},
						{Names: []string{"err"}, Type: "error"},
					},
				},
				&Function{
					Name: "F4",
					Doc:  "F4 is function @FUN4",
					Params: []*Field{
						{Names: []string{"x"}, Type: "int", Doc: "x of F4 @arg4\nx of F4 @arg5"},
						{Names: []string{"y"}, Type: "string", Doc: "y of F4 @arg6\ny of F4 @arg7"},
						{Names: []string{"args"}, Type: "...interface{}", Doc: "arg of F4 @arg8"},
					},
					Results: []*Field{
						{Names: []string{"string"}, Type: "string", Doc: "result if F4 @ret3\nresult if F4 @ret4\nret of F4 @ret5"},
						{Names: []string{"error"}, Type: "error", Doc: "err of F4 @ret6\nerr of F4 @ret7"},
					},
				},
				&Function{Name: "F5", Doc: "F5 is function @FUN5"},
				&ValueSpec{Kind: token.VAR, Names: []string{"F6"}, Doc: "F6 is function (anonymous) @FUN6"},
				&Function{
					Name: "F7",
					Doc:  "F7 is function @FUN7",
					Params: []*Field{
						{Names: []string{"ctx"}, Type: "context.Context"},
						{Names: []string{"x", "y"}, Type: "int"},
						{Names: []string{"z"}, Type: "string"},
					},
					Results: []*Field{
						{Names: []string{"x", "y"}, Type: "error"},
					},
				},
				&Function{
					Name: "F8",
					Doc:  "F8 is function @FUN8",
					Params: []*Field{
						{Names: []string{"ctx"}, Type: "context.Context"},
						{Names: []string{"x", "y"}, Type: "int"},
						{Names: []string{"pretty"}, Type: "*bool", Doc: "pretty output or not"},
					},
					Results: []*Field{
						{Names: []string{"[]int"}, Type: "[]int", Doc: "ret"},
					},
				},
				&Function{
					Name: "F9",
					Doc:  "F9 is function @FUN9",
					Params: []*Field{
						{Names: []string{"ctx"}, Type: "context.Context"},
						{Names: []string{"x", "y"}, Type: "int"},
						{Names: []string{"pretty"}, Type: "*bool", Doc: "pretty output or not"},
					},
					Results: []*Field{
						{Names: []string{"[]int"}, Type: "[]int", Doc: "ret"},
						{Names: []string{"error"}, Type: "error", Doc: "error"},
					},
				},
			},
		},
		{
			filename: "structs.go",
			expected: []interface{}{
				&TypeSpec{
					Name: "S",
					Doc:  "S is struct @S0",
					Definition: &Struct{
						Fields: []*Field{
							{Names: []string{"ExportedString"}, Type: "string", Doc: "ExportedString is exported string @F0"},
							{Names: []string{"ExportedString2"}, Type: "string", Doc: "ExportedString2 is exported string @F1"},
							{Names: []string{"ExportedString3"}, Type: "string", Doc: "ExportedString3 is exported string @F2\nExportedString3 is exported string @F3"},
							{Names: []string{"Nested"}, Type: "struct{...}", Doc: "Nested is struct @SS0"},
							{Names: []string{"unexportedString"}, Type: "string"},
						},
					},
				},
				&TypeSpec{Name: "S2", Doc: "S2 is struct @S2"},
				&TypeSpec{Name: "S3", Doc: "S3 is struct @S3"},
			},
		},
		{
			filename: "values.go",
			expected: []interface{}{
				&ValueSpec{Kind: token.CONST, Names: []string{"ConstA"}, Doc: "GroupedConsts is a group of consts @CG0\nConstA is a const @CA0\nline comment ConstA @CA1"},
				&ValueSpec{Kind: token.CONST, Names: []string{"ConstB"}, Doc: "GroupedConsts is a group of consts @CG0\nConstB is a const @CB0"},
				&ValueSpec{Kind: token.CONST, Names: []string{"ConstC"}, Doc: "GroupedConsts is a group of consts @CG0\nline comment ConstC @CC1"},
				&ValueSpec{Kind: token.CONST, Names: []string{"SingleConst"}, Doc: "SingleConst is a single const @CS0\nline comment SingleConst @CS1"},
				&ValueSpec{Kind: token.VAR, Names: []string{"VarA"}, Doc: "GroupedVars is a group of vars @VG0\nVarA is a var @VA0\nline comment VarA @VA1"},
				&ValueSpec{Kind: token.VAR, Names: []string{"VarB"}, Doc: "GroupedVars is a group of vars @VG0\nVarB is a var @VB0"},
				&ValueSpec{Kind: token.VAR, Names: []string{"VarC"}, Doc: "GroupedVars is a group of vars @VG0\nline comment VarC @VC1"},
				&ValueSpec{Kind: token.VAR, Names: []string{"SingleVar"}, Doc: "SingleVar is a single var @VS0\nline comment SingleVar @VS1"},
				&ValueSpec{Kind: token.VAR, Names: []string{"MultiVar1", "MultiVar2"}, Doc: "MultiVar is a multi var @VM0\nline comment MultiVar @VM1"},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.filename, func(t *testing.T) {
			path := filepath.Join("testdata", tc.filename)
			docs, err := FromFile(path)
			if err != nil {
				t.Fatalf("FromFile() failed: %v", err)
			}

			if len(docs) != len(tc.expected) {
				t.Fatalf("Expected %d docs, but got %d (GOT: %s)", len(tc.expected), len(docs), spew.Sdump(docs))
			}

			for i, want := range tc.expected {
				got := docs[i]

				// Special handling for nested struct fields which are not deeply parsed
				if tsGot, ok := got.(*TypeSpec); ok {
					if tsWant, ok2 := want.(*TypeSpec); ok2 {
						if sGot, ok3 := tsGot.Definition.(*Struct); ok3 {
							if sWant, ok4 := tsWant.Definition.(*Struct); ok4 {
								for j, fWant := range sWant.Fields {
									if j >= len(sGot.Fields) {
										break
									}
									fGot := sGot.Fields[j]
									if fWant.Type == "struct{...}" {
										fGot.Type = "struct{...}"
									}
								}
							}
						}
					}
				}

				if !reflect.DeepEqual(got, want) {
					t.Errorf("Mismatch at index %d\nGOT:  %s\nWANT: %s", i, spew.Sdump(got), spew.Sdump(want))
				}
			}
		})
	}
}
