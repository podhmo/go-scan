package generator

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
	"testing"

	"example.com/convert2/internal/model"
)

// formatCode formats the given Go code string using go/format.Source.
// If formatting fails, it returns the original string and the error.
func formatCode(code string) (string, error) {
	formatted, err := format.Source([]byte(code))
	if err != nil {
		return code, err
	}
	return string(formatted), nil
}

// normalizeCode for comparison by removing empty lines and trimming space from each line.
func normalizeCode(code string) string {
	var b strings.Builder
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			b.WriteString(trimmed)
			b.WriteString("\n") // Add back newline for non-empty lines
		}
	}
	return strings.TrimSpace(b.String()) // Trim trailing newline
}

func TestGenerateHelperFunction_Pointer_T_to_StarT(t *testing.T) {
	srcTypeInfo := &model.TypeInfo{
		Name:      "string",
		FullName:  "string",
		IsPointer: false,
		Kind:      model.KindBasic,
		IsBasic:   true,
	}
	dstElemTypeInfo := &model.TypeInfo{
		Name:      "string",
		FullName:  "string",
		IsPointer: false,
		Kind:      model.KindBasic,
		IsBasic:   true,
	}
	dstTypeInfo := &model.TypeInfo{
		Name:      "*string",
		FullName:  "*string",
		IsPointer: true,
		Elem:      dstElemTypeInfo,
		Kind:      model.KindPointer,
	}

	srcField := model.FieldInfo{
		Name:     "MyString",
		TypeInfo: srcTypeInfo,
		Tag:      model.ConvertTag{},
	}
	dstField := model.FieldInfo{
		Name:     "MyStringPtr", // Assuming DstFieldName in tag or matched by name
		TypeInfo: dstTypeInfo,
	}

	srcStructInfo := &model.StructInfo{Name: "Src", Fields: []model.FieldInfo{srcField}}
	dstStructInfo := &model.StructInfo{Name: "Dst", Fields: []model.FieldInfo{dstField}}
	srcField.ParentStruct = srcStructInfo
	dstField.ParentStruct = dstStructInfo

	parsedInfo := model.NewParsedInfo("mypkg", "example.com/mypkg")
	parsedInfo.Structs["Src"] = srcStructInfo
	parsedInfo.Structs["Dst"] = dstStructInfo
	srcStructType := &model.TypeInfo{Name: "Src", FullName: "example.com/mypkg.Src", Kind: model.KindStruct, StructInfo: srcStructInfo}
	dstStructType := &model.TypeInfo{Name: "Dst", FullName: "example.com/mypkg.Dst", Kind: model.KindStruct, StructInfo: dstStructInfo}

	var buf bytes.Buffer
	imports := make(map[string]string)

	srcStructInfo.Fields = []model.FieldInfo{srcField}
	dstStructInfo.Fields = []model.FieldInfo{dstField}

	// Initialize worklist and processedPairs for the call
	worklist := new([]model.ConversionPair)
	processedPairs := make(map[string]bool)

	err := generateHelperFunction(&buf, "srcToDst", srcStructType, dstStructType, parsedInfo, imports, worklist, processedPairs)
	if err != nil {
		t.Fatalf("generateHelperFunction failed: %v", err)
	}

	generatedCode := buf.String()
	// In this specific test, MyString maps to MyStringPtr, so no unmapped fields in Dst.
	// The docstring for unmapped fields should NOT be present.
	// However, the generic docstring "srcToDst converts Src to Dst" might be added if we decide to always add a base docstring.
	// Based on current implementation, if unmappedDstFields is empty, no specific docstring is added.
	// Let's assume the general descriptive docstring is now part of the function if there are unmapped fields.
	// If no unmapped fields, the specific "Fields in Dst not populated..." won't appear.
	// The test should verify its absence or presence accordingly.
	// For this test, MyString -> MyStringPtr, Dst has only MyStringPtr, so no unmapped fields.
	// The function signature itself acts as the primary "doc".
	expectedFullFunc := fmt.Sprintf(`func srcToDst(ec *errorCollector, src %s) %s {
	dst := %s{}
	if ec.MaxErrorsReached() { return dst }

	// Mapping field %s.MyString (%s) to %s.MyStringPtr (%s)
	// Src: Ptr=false, ElemFull=string | Dst: Ptr=true, ElemFull=string
	ec.Enter("MyStringPtr")
	{
		srcVal := src.MyString
		dst.MyStringPtr = &srcVal
	}
	ec.Leave()
	if ec.MaxErrorsReached() { return dst }

	return dst
}

`, typeNameInSource(srcStructType, parsedInfo.PackagePath, imports), typeNameInSource(dstStructType, parsedInfo.PackagePath, imports), typeNameInSource(dstStructType, parsedInfo.PackagePath, imports), srcStructType.Name, srcTypeInfo.FullName, dstStructType.Name, dstTypeInfo.FullName)

	formattedGenerated, errGen := formatCode(generatedCode)
	if errGen != nil {
		t.Logf("Warning: could not format generated code: %v\nCode:\n%s", errGen, generatedCode)
	}
	formattedExpected, errExp := formatCode(expectedFullFunc)
	if errExp != nil {
		t.Fatalf("Failed to format expected code: %v\nCode:\n%s", errExp, expectedFullFunc)
	}

	if normalizeCode(formattedGenerated) != normalizeCode(formattedExpected) {
		t.Errorf("generateHelperFunction T_to_StarT mismatch:\n---EXPECTED---\n%s\n---GENERATED---\n%s", formattedExpected, formattedGenerated)
	}
}

func TestGenerateHelperFunction_Pointer_StarT_to_T_Default(t *testing.T) {
	srcElemTypeInfo := &model.TypeInfo{
		Name:      "string",
		FullName:  "string",
		IsPointer: false,
		Kind:      model.KindBasic,
		IsBasic:   true,
	}
	srcTypeInfo := &model.TypeInfo{
		Name:      "*string",
		FullName:  "*string",
		IsPointer: true,
		Elem:      srcElemTypeInfo,
		Kind:      model.KindPointer,
	}
	dstTypeInfo := &model.TypeInfo{
		Name:      "string",
		FullName:  "string",
		IsPointer: false,
		Kind:      model.KindBasic,
		IsBasic:   true,
	}

	srcField := model.FieldInfo{
		Name:     "MyStringPtr",
		TypeInfo: srcTypeInfo,
		Tag:      model.ConvertTag{Required: false}, // Default behavior
	}
	dstField := model.FieldInfo{
		Name:     "MyString",
		TypeInfo: dstTypeInfo,
	}

	srcStructInfo := &model.StructInfo{Name: "Src", Fields: []model.FieldInfo{srcField}}
	dstStructInfo := &model.StructInfo{Name: "Dst", Fields: []model.FieldInfo{dstField}}
	srcField.ParentStruct = srcStructInfo
	dstField.ParentStruct = dstStructInfo

	parsedInfo := model.NewParsedInfo("mypkg", "example.com/mypkg")
	parsedInfo.Structs["Src"] = srcStructInfo
	parsedInfo.Structs["Dst"] = dstStructInfo
	srcStructType := &model.TypeInfo{Name: "Src", FullName: "example.com/mypkg.Src", Kind: model.KindStruct, StructInfo: srcStructInfo}
	dstStructType := &model.TypeInfo{Name: "Dst", FullName: "example.com/mypkg.Dst", Kind: model.KindStruct, StructInfo: dstStructInfo}

	var buf bytes.Buffer
	imports := make(map[string]string)

	srcStructInfo.Fields = []model.FieldInfo{srcField}
	dstStructInfo.Fields = []model.FieldInfo{dstField}

	worklist := new([]model.ConversionPair)
	processedPairs := make(map[string]bool)
	err := generateHelperFunction(&buf, "srcToDst", srcStructType, dstStructType, parsedInfo, imports, worklist, processedPairs)
	if err != nil {
		t.Fatalf("generateHelperFunction failed: %v", err)
	}

	generatedCode := buf.String()
	expectedFullFunc := fmt.Sprintf(`func srcToDst(ec *errorCollector, src %s) %s {
	dst := %s{}
	if ec.MaxErrorsReached() { return dst }

	// Mapping field %s.MyStringPtr (%s) to %s.MyString (%s)
	// Src: Ptr=true, ElemFull=string | Dst: Ptr=false, ElemFull=string
	ec.Enter("MyString")
	if src.MyStringPtr != nil {
		dst.MyString = *src.MyStringPtr
	}
	ec.Leave()
	if ec.MaxErrorsReached() { return dst }

	return dst
}

`, typeNameInSource(srcStructType, parsedInfo.PackagePath, imports), typeNameInSource(dstStructType, parsedInfo.PackagePath, imports), typeNameInSource(dstStructType, parsedInfo.PackagePath, imports), srcStructType.Name, srcTypeInfo.FullName, dstStructType.Name, dstTypeInfo.FullName)

	formattedGenerated, errGen := formatCode(generatedCode)
	if errGen != nil {
		t.Logf("Warning: could not format generated code: %v\nCode:\n%s", errGen, generatedCode)
	}
	formattedExpected, errExp := formatCode(expectedFullFunc)
	if errExp != nil {
		t.Fatalf("Failed to format expected code: %v\nCode:\n%s", errExp, expectedFullFunc)
	}

	if normalizeCode(formattedGenerated) != normalizeCode(formattedExpected) {
		t.Errorf("generateHelperFunction StarT_to_T_Default mismatch:\n---EXPECTED---\n%s\n---GENERATED---\n%s", formattedExpected, formattedGenerated)
	}
}

func TestGenerateHelperFunction_Pointer_StarT_to_T_Required_Nil(t *testing.T) {
	srcElemTypeInfo := &model.TypeInfo{
		Name:      "string",
		FullName:  "string",
		IsPointer: false,
		Kind:      model.KindBasic,
		IsBasic:   true,
	}
	srcTypeInfo := &model.TypeInfo{
		Name:      "*string",
		FullName:  "*string",
		IsPointer: true,
		Elem:      srcElemTypeInfo,
		Kind:      model.KindPointer,
	}
	dstTypeInfo := &model.TypeInfo{
		Name:      "string",
		FullName:  "string",
		IsPointer: false,
		Kind:      model.KindBasic,
		IsBasic:   true,
	}

	srcField := model.FieldInfo{
		Name:     "MyRequiredStringPtr", // Source field name
		TypeInfo: srcTypeInfo,
		Tag:      model.ConvertTag{Required: true},
	}
	dstField := model.FieldInfo{
		Name:     "MyRequiredString", // Destination field name
		TypeInfo: dstTypeInfo,
	}

	srcStructInfo := &model.StructInfo{Name: "Src", Fields: []model.FieldInfo{srcField}}
	dstStructInfo := &model.StructInfo{Name: "Dst", Fields: []model.FieldInfo{dstField}}
	srcField.ParentStruct = srcStructInfo
	dstField.ParentStruct = dstStructInfo

	parsedInfo := model.NewParsedInfo("mypkg", "example.com/mypkg")
	parsedInfo.Structs["Src"] = srcStructInfo
	parsedInfo.Structs["Dst"] = dstStructInfo
	srcStructType := &model.TypeInfo{Name: "Src", FullName: "example.com/mypkg.Src", Kind: model.KindStruct, StructInfo: srcStructInfo}
	dstStructType := &model.TypeInfo{Name: "Dst", FullName: "example.com/mypkg.Dst", Kind: model.KindStruct, StructInfo: dstStructInfo}

	var buf bytes.Buffer
	imports := make(map[string]string)

	srcStructInfo.Fields = []model.FieldInfo{srcField}
	dstStructInfo.Fields = []model.FieldInfo{dstField}

	worklist := new([]model.ConversionPair)
	processedPairs := make(map[string]bool)
	err := generateHelperFunction(&buf, "srcToDst", srcStructType, dstStructType, parsedInfo, imports, worklist, processedPairs)
	if err != nil {
		t.Fatalf("generateHelperFunction failed: %v", err)
	}

	generatedCode := buf.String()
	// Note: The error message in ec.Addf uses dstField.Name and srcField.Name
	expectedFullFunc := fmt.Sprintf(`func srcToDst(ec *errorCollector, src %s) %s {
	dst := %s{}
	if ec.MaxErrorsReached() { return dst }

	// Mapping field %s.MyRequiredStringPtr (%s) to %s.MyRequiredString (%s)
	// Src: Ptr=true, ElemFull=string | Dst: Ptr=false, ElemFull=string
	ec.Enter("MyRequiredString")
	if src.MyRequiredStringPtr == nil {
		ec.Addf("field '%s' is required but source field %s is nil")
	} else {
		dst.MyRequiredString = *src.MyRequiredStringPtr
	}
	ec.Leave()
	if ec.MaxErrorsReached() { return dst }

	return dst
}

`, typeNameInSource(srcStructType, parsedInfo.PackagePath, imports),
		typeNameInSource(dstStructType, parsedInfo.PackagePath, imports),
		typeNameInSource(dstStructType, parsedInfo.PackagePath, imports),
		srcStructType.Name, srcTypeInfo.FullName, dstStructType.Name, dstTypeInfo.FullName,
		dstField.Name, srcField.Name) // For the ec.Addf parameters

	formattedGenerated, errGen := formatCode(generatedCode)
	if errGen != nil {
		t.Logf("Warning: could not format generated code: %v\nCode:\n%s", errGen, generatedCode)
	}
	formattedExpected, errExp := formatCode(expectedFullFunc)
	if errExp != nil {
		t.Fatalf("Failed to format expected code: %v\nCode:\n%s", errExp, expectedFullFunc)
	}

	if normalizeCode(formattedGenerated) != normalizeCode(formattedExpected) {
		t.Errorf("generateHelperFunction StarT_to_T_Required_Nil mismatch:\n---EXPECTED---\n%s\n---GENERATED---\n%s", formattedExpected, formattedGenerated)
	}
}

func TestGenerateHelperFunction_Pointer_StarT_to_T_Required_NonNil(t *testing.T) {
	srcElemTypeInfo := &model.TypeInfo{
		Name:      "string",
		FullName:  "string",
		IsPointer: false,
		Kind:      model.KindBasic,
		IsBasic:   true,
	}
	srcTypeInfo := &model.TypeInfo{
		Name:      "*string",
		FullName:  "*string",
		IsPointer: true,
		Elem:      srcElemTypeInfo,
		Kind:      model.KindPointer,
	}
	dstTypeInfo := &model.TypeInfo{
		Name:      "string",
		FullName:  "string",
		IsPointer: false,
		Kind:      model.KindBasic,
		IsBasic:   true,
	}

	srcField := model.FieldInfo{
		Name:     "MyRequiredStringPtrNN", // Source field name
		TypeInfo: srcTypeInfo,
		Tag:      model.ConvertTag{Required: true},
	}
	dstField := model.FieldInfo{
		Name:     "MyRequiredStringNN", // Destination field name
		TypeInfo: dstTypeInfo,
	}

	srcStructInfo := &model.StructInfo{Name: "Src", Fields: []model.FieldInfo{srcField}}
	dstStructInfo := &model.StructInfo{Name: "Dst", Fields: []model.FieldInfo{dstField}}
	srcField.ParentStruct = srcStructInfo
	dstField.ParentStruct = dstStructInfo

	parsedInfo := model.NewParsedInfo("mypkg", "example.com/mypkg")
	parsedInfo.Structs["Src"] = srcStructInfo
	parsedInfo.Structs["Dst"] = dstStructInfo
	srcStructType := &model.TypeInfo{Name: "Src", FullName: "example.com/mypkg.Src", Kind: model.KindStruct, StructInfo: srcStructInfo}
	dstStructType := &model.TypeInfo{Name: "Dst", FullName: "example.com/mypkg.Dst", Kind: model.KindStruct, StructInfo: dstStructInfo}

	var buf bytes.Buffer
	imports := make(map[string]string)

	srcStructInfo.Fields = []model.FieldInfo{srcField}
	dstStructInfo.Fields = []model.FieldInfo{dstField}

	worklist := new([]model.ConversionPair)
	processedPairs := make(map[string]bool)
	err := generateHelperFunction(&buf, "srcToDst", srcStructType, dstStructType, parsedInfo, imports, worklist, processedPairs)
	if err != nil {
		t.Fatalf("generateHelperFunction failed: %v", err)
	}

	generatedCode := buf.String()
	// This case should generate the 'else' part of the required check.
	expectedFullFunc := fmt.Sprintf(`func srcToDst(ec *errorCollector, src %s) %s {
	dst := %s{}
	if ec.MaxErrorsReached() { return dst }

	// Mapping field %s.MyRequiredStringPtrNN (%s) to %s.MyRequiredStringNN (%s)
	// Src: Ptr=true, ElemFull=string | Dst: Ptr=false, ElemFull=string
	ec.Enter("MyRequiredStringNN")
	if src.MyRequiredStringPtrNN == nil {
		ec.Addf("field '%s' is required but source field %s is nil")
	} else {
		dst.MyRequiredStringNN = *src.MyRequiredStringPtrNN
	}
	ec.Leave()
	if ec.MaxErrorsReached() { return dst }

	return dst
}

`, typeNameInSource(srcStructType, parsedInfo.PackagePath, imports),
		typeNameInSource(dstStructType, parsedInfo.PackagePath, imports),
		typeNameInSource(dstStructType, parsedInfo.PackagePath, imports),
		srcStructType.Name, srcTypeInfo.FullName, dstStructType.Name, dstTypeInfo.FullName,
		dstField.Name, srcField.Name) // For the ec.Addf parameters

	formattedGenerated, errGen := formatCode(generatedCode)
	if errGen != nil {
		t.Logf("Warning: could not format generated code: %v\nCode:\n%s", errGen, generatedCode)
	}
	formattedExpected, errExp := formatCode(expectedFullFunc)
	if errExp != nil {
		t.Fatalf("Failed to format expected code: %v\nCode:\n%s", errExp, expectedFullFunc)
	}

	if normalizeCode(formattedGenerated) != normalizeCode(formattedExpected) {
		t.Errorf("generateHelperFunction StarT_to_T_Required_NonNil mismatch:\n---EXPECTED---\n%s\n---GENERATED---\n%s", formattedExpected, formattedGenerated)
	}
}

func TestGenerateHelperFunction_Pointer_StarT_to_StarT(t *testing.T) {
	srcElemTypeInfo := &model.TypeInfo{
		Name:      "string",
		FullName:  "string",
		IsPointer: false,
		Kind:      model.KindBasic,
		IsBasic:   true,
	}
	srcTypeInfo := &model.TypeInfo{
		Name:      "*string",
		FullName:  "*string",
		IsPointer: true,
		Elem:      srcElemTypeInfo,
		Kind:      model.KindPointer,
	}
	dstTypeInfo := &model.TypeInfo{ // Destination is also *string
		Name:      "*string",
		FullName:  "*string",
		IsPointer: true,
		Elem:      srcElemTypeInfo, // Same element type
		Kind:      model.KindPointer,
	}

	srcField := model.FieldInfo{
		Name:     "MyPtrSrc",
		TypeInfo: srcTypeInfo,
		Tag:      model.ConvertTag{},
	}
	dstField := model.FieldInfo{
		Name:     "MyPtrDst",
		TypeInfo: dstTypeInfo,
	}

	srcStructInfo := &model.StructInfo{Name: "Src", Fields: []model.FieldInfo{srcField}}
	dstStructInfo := &model.StructInfo{Name: "Dst", Fields: []model.FieldInfo{dstField}}
	srcField.ParentStruct = srcStructInfo
	dstField.ParentStruct = dstStructInfo

	parsedInfo := model.NewParsedInfo("mypkg", "example.com/mypkg")
	parsedInfo.Structs["Src"] = srcStructInfo
	parsedInfo.Structs["Dst"] = dstStructInfo
	srcStructType := &model.TypeInfo{Name: "Src", FullName: "example.com/mypkg.Src", Kind: model.KindStruct, StructInfo: srcStructInfo}
	dstStructType := &model.TypeInfo{Name: "Dst", FullName: "example.com/mypkg.Dst", Kind: model.KindStruct, StructInfo: dstStructInfo}

	var buf bytes.Buffer
	imports := make(map[string]string)

	srcStructInfo.Fields = []model.FieldInfo{srcField}
	dstStructInfo.Fields = []model.FieldInfo{dstField}

	worklist := new([]model.ConversionPair)
	processedPairs := make(map[string]bool)
	err := generateHelperFunction(&buf, "srcToDst", srcStructType, dstStructType, parsedInfo, imports, worklist, processedPairs)
	if err != nil {
		t.Fatalf("generateHelperFunction failed: %v", err)
	}

	generatedCode := buf.String()
	expectedFullFunc := fmt.Sprintf(`func srcToDst(ec *errorCollector, src %s) %s {
	dst := %s{}
	if ec.MaxErrorsReached() { return dst }

	// Mapping field %s.MyPtrSrc (%s) to %s.MyPtrDst (%s)
	// Src: Ptr=true, ElemFull=string | Dst: Ptr=true, ElemFull=string
	ec.Enter("MyPtrDst")
	dst.MyPtrDst = src.MyPtrSrc
	ec.Leave()
	if ec.MaxErrorsReached() { return dst }

	return dst
}

`, typeNameInSource(srcStructType, parsedInfo.PackagePath, imports),
		typeNameInSource(dstStructType, parsedInfo.PackagePath, imports),
		typeNameInSource(dstStructType, parsedInfo.PackagePath, imports),
		srcStructType.Name, srcTypeInfo.FullName, dstStructType.Name, dstTypeInfo.FullName)

	formattedGenerated, errGen := formatCode(generatedCode)
	if errGen != nil {
		t.Logf("Warning: could not format generated code: %v\nCode:\n%s", errGen, generatedCode)
	}
	formattedExpected, errExp := formatCode(expectedFullFunc)
	if errExp != nil {
		t.Fatalf("Failed to format expected code: %v\nCode:\n%s", errExp, expectedFullFunc)
	}

	if normalizeCode(formattedGenerated) != normalizeCode(formattedExpected) {
		t.Errorf("generateHelperFunction StarT_to_StarT mismatch:\n---EXPECTED---\n%s\n---GENERATED---\n%s", formattedExpected, formattedGenerated)
	}
}

func TestGenerateHelperFunction_Using_FieldTag(t *testing.T) {
	srcTypeInfo := &model.TypeInfo{Name: "int", FullName: "int", Kind: model.KindBasic, IsBasic: true}
	dstTypeInfo := &model.TypeInfo{Name: "string", FullName: "string", Kind: model.KindBasic, IsBasic: true}

	srcField := model.FieldInfo{
		Name:     "SrcInt",
		TypeInfo: srcTypeInfo,
		Tag:      model.ConvertTag{UsingFunc: "IntToStringConverter"},
	}
	dstField := model.FieldInfo{
		Name:     "DstString",
		TypeInfo: dstTypeInfo,
	}

	srcStructInfo := &model.StructInfo{Name: "Src", Fields: []model.FieldInfo{srcField}}
	dstStructInfo := &model.StructInfo{Name: "Dst", Fields: []model.FieldInfo{dstField}}
	srcField.ParentStruct = srcStructInfo
	dstField.ParentStruct = dstStructInfo

	parsedInfo := model.NewParsedInfo("mypkg", "example.com/mypkg")
	parsedInfo.Structs["Src"] = srcStructInfo
	parsedInfo.Structs["Dst"] = dstStructInfo
	srcStructType := &model.TypeInfo{Name: "Src", FullName: "example.com/mypkg.Src", Kind: model.KindStruct, StructInfo: srcStructInfo}
	dstStructType := &model.TypeInfo{Name: "Dst", FullName: "example.com/mypkg.Dst", Kind: model.KindStruct, StructInfo: dstStructInfo}

	var buf bytes.Buffer
	imports := make(map[string]string)
	worklist := new([]model.ConversionPair)
	processedPairs := make(map[string]bool)
	err := generateHelperFunction(&buf, "srcToDst", srcStructType, dstStructType, parsedInfo, imports, worklist, processedPairs)
	if err != nil {
		t.Fatalf("generateHelperFunction failed for field tag using: %v", err)
	}

	generatedCode := buf.String()
	expectedFullFunc := fmt.Sprintf(`func srcToDst(ec *errorCollector, src %s) %s {
	dst := %s{}
	if ec.MaxErrorsReached() { return dst }

	// Mapping field %s.SrcInt (%s) to %s.DstString (%s)
	// Src: Ptr=false, ElemFull=nil | Dst: Ptr=false, ElemFull=nil
	ec.Enter("DstString")
	// Applying field tag: using IntToStringConverter
	dst.DstString = IntToStringConverter(ec, src.SrcInt)
	ec.Leave()
	if ec.MaxErrorsReached() { return dst }

	return dst
}
`, typeNameInSource(srcStructType, parsedInfo.PackagePath, imports), typeNameInSource(dstStructType, parsedInfo.PackagePath, imports), typeNameInSource(dstStructType, parsedInfo.PackagePath, imports), srcStructType.Name, srcTypeInfo.FullName, dstStructType.Name, dstTypeInfo.FullName)

	formattedGenerated, _ := formatCode(generatedCode)
	formattedExpected, _ := formatCode(expectedFullFunc)
	if normalizeCode(formattedGenerated) != normalizeCode(formattedExpected) {
		t.Errorf("generateHelperFunction Using_FieldTag mismatch:\n---EXPECTED---\n%s\n---GENERATED---\n%s", formattedExpected, formattedGenerated)
	}
}

func TestGenerateHelperFunction_Using_GlobalRule(t *testing.T) {
	srcTypeInfo := &model.TypeInfo{Name: "float64", FullName: "float64", Kind: model.KindBasic, IsBasic: true}
	dstTypeInfo := &model.TypeInfo{Name: "Decimal", FullName: "custompkg.Decimal", PackagePath: "example.com/custompkg", PackageName: "custompkg", Kind: model.KindIdent}

	srcField := model.FieldInfo{
		Name:     "SrcFloat",
		TypeInfo: srcTypeInfo,
		Tag:      model.ConvertTag{}, // No field tag using
	}
	dstField := model.FieldInfo{
		Name:     "DstDecimal",
		TypeInfo: dstTypeInfo,
	}

	globalRule := model.TypeRule{
		SrcTypeInfo: srcTypeInfo,
		DstTypeInfo: dstTypeInfo,
		UsingFunc:   "custompkg.FloatToDecimalConverter",
	}

	srcStructInfo := &model.StructInfo{Name: "Src", Fields: []model.FieldInfo{srcField}}
	dstStructInfo := &model.StructInfo{Name: "Dst", Fields: []model.FieldInfo{dstField}}
	srcField.ParentStruct = srcStructInfo
	dstField.ParentStruct = dstStructInfo

	parsedInfo := model.NewParsedInfo("mypkg", "example.com/mypkg")
	parsedInfo.Structs["Src"] = srcStructInfo
	parsedInfo.Structs["Dst"] = dstStructInfo
	parsedInfo.GlobalRules = []model.TypeRule{globalRule} // Add global rule

	srcStructType := &model.TypeInfo{Name: "Src", FullName: "example.com/mypkg.Src", Kind: model.KindStruct, StructInfo: srcStructInfo}
	dstStructType := &model.TypeInfo{Name: "Dst", FullName: "example.com/mypkg.Dst", Kind: model.KindStruct, StructInfo: dstStructInfo}

	var buf bytes.Buffer
	imports := make(map[string]string)
	worklist := new([]model.ConversionPair)
	processedPairs := make(map[string]bool)
	err := generateHelperFunction(&buf, "srcToDst", srcStructType, dstStructType, parsedInfo, imports, worklist, processedPairs)
	if err != nil {
		t.Fatalf("generateHelperFunction failed for global rule using: %v", err)
	}

	generatedCode := buf.String()
	// Expecting custompkg.FloatToDecimalConverter to be called.
	// The import for "custompkg" should also be added by addRequiredImport if DstTypeInfo is processed by it.
	// For functions, direct import handling is still a TODO in the generator.
	// We will assume the function name is rendered as is.
	expectedFullFunc := fmt.Sprintf(`func srcToDst(ec *errorCollector, src %s) %s {
	dst := %s{}
	if ec.MaxErrorsReached() { return dst }

	// Mapping field %s.SrcFloat (%s) to %s.DstDecimal (%s)
	// Src: Ptr=false, ElemFull=nil | Dst: Ptr=false, ElemFull=nil
	ec.Enter("DstDecimal")
	// Applying global rule: float64 -> custompkg.Decimal using custompkg.FloatToDecimalConverter
	dst.DstDecimal = custompkg.FloatToDecimalConverter(ec, src.SrcFloat)
	ec.Leave()
	if ec.MaxErrorsReached() { return dst }

	return dst
}
`, typeNameInSource(srcStructType, parsedInfo.PackagePath, imports), typeNameInSource(dstStructType, parsedInfo.PackagePath, imports), typeNameInSource(dstStructType, parsedInfo.PackagePath, imports), srcStructType.Name, srcTypeInfo.FullName, dstStructType.Name, dstTypeInfo.FullName)

	formattedGenerated, _ := formatCode(generatedCode)
	formattedExpected, _ := formatCode(expectedFullFunc)
	if normalizeCode(formattedGenerated) != normalizeCode(formattedExpected) {
		t.Errorf("generateHelperFunction Using_GlobalRule mismatch:\n---EXPECTED---\n%s\n---GENERATED---\n%s", formattedExpected, formattedGenerated)
	}
}

// Placeholder for other tests to be added in next steps of the plan.
// This file will be expanded upon.

func TestGenerateHelperFunction_UnmappedFieldsDocstring(t *testing.T) {
	srcTypeInfo := &model.TypeInfo{Name: "string", FullName: "string", Kind: model.KindBasic, IsBasic: true}
	dstTypeInfo := &model.TypeInfo{Name: "string", FullName: "string", Kind: model.KindBasic, IsBasic: true}
	dstUnmappedTypeInfo := &model.TypeInfo{Name: "int", FullName: "int", Kind: model.KindBasic, IsBasic: true}

	srcField := model.FieldInfo{Name: "MappedField", TypeInfo: srcTypeInfo, Tag: model.ConvertTag{}}
	dstMappedField := model.FieldInfo{Name: "MappedField", TypeInfo: dstTypeInfo}
	dstUnmappedField := model.FieldInfo{Name: "UnmappedExtraField", TypeInfo: dstUnmappedTypeInfo} // This field in Dst has no source

	srcStructInfo := &model.StructInfo{Name: "Src", Fields: []model.FieldInfo{srcField}}
	dstStructInfo := &model.StructInfo{Name: "Dst", Fields: []model.FieldInfo{dstMappedField, dstUnmappedField}}
	srcField.ParentStruct = srcStructInfo
	dstMappedField.ParentStruct = dstStructInfo
	dstUnmappedField.ParentStruct = dstStructInfo

	parsedInfo := model.NewParsedInfo("mypkg", "example.com/mypkg")
	parsedInfo.Structs["Src"] = srcStructInfo
	parsedInfo.Structs["Dst"] = dstStructInfo
	srcStructType := &model.TypeInfo{Name: "Src", FullName: "example.com/mypkg.Src", Kind: model.KindStruct, StructInfo: srcStructInfo}
	dstStructType := &model.TypeInfo{Name: "Dst", FullName: "example.com/mypkg.Dst", Kind: model.KindStruct, StructInfo: dstStructInfo}

	var buf bytes.Buffer
	imports := make(map[string]string)
	err := generateHelperFunction(&buf, "srcToDstWithUnmapped", srcStructType, dstStructType, parsedInfo, imports, new([]model.ConversionPair), make(map[string]bool))
	if err != nil {
		t.Fatalf("generateHelperFunction failed for unmapped fields docstring: %v", err)
	}

	generatedCode := buf.String()
	expectedDocstring := `// srcToDstWithUnmapped converts Src to Dst.
// Fields in Dst not populated by this conversion:
// - UnmappedExtraField
`
	expectedFuncSignature := fmt.Sprintf(`func srcToDstWithUnmapped(ec *errorCollector, src %s) %s {`, typeNameInSource(srcStructType, parsedInfo.PackagePath, imports), typeNameInSource(dstStructType, parsedInfo.PackagePath, imports))

	if !strings.HasPrefix(generatedCode, expectedDocstring) {
		t.Errorf("Generated code for unmapped fields is missing expected docstring.\n---EXPECTED DOCSTRING---\n%s\n---GENERATED CODE---\n%s", expectedDocstring, generatedCode)
	}
	if !strings.Contains(generatedCode, expectedFuncSignature) {
		t.Errorf("Generated code for unmapped fields is missing expected function signature.\n---EXPECTED SIGNATURE---\n%s\n---GENERATED CODE---\n%s", expectedFuncSignature, generatedCode)
	}

	// Also check the body content to ensure mapping happened for the mapped field
	expectedMappedFieldLogic := fmt.Sprintf(`// Mapping field Src.MappedField (string) to Dst.MappedField (string)`)
	if !strings.Contains(generatedCode, expectedMappedFieldLogic) {
		t.Errorf("Generated code for unmapped fields is missing expected mapping logic for MappedField.\n---GENERATED CODE---\n%s", generatedCode)
	}
}
