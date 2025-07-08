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
		Name:     "MyStringPtr",
		TypeInfo: dstTypeInfo,
	}
	// Ensure DstFieldName in tag matches the destination field name if they are different from srcField.Name
	srcField.Tag.DstFieldName = "MyStringPtr"


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

	// DEBUG: Number of source fields: 1 for struct Src
	// DEBUG: Processing source field: MyString
	// DEBUG: dstFieldName = MyStringPtr, dstField is nil = false
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

func TestGenerateHelperFunction_Underlying_MyInt_To_YourInt(t *testing.T) {
	// Define base types
	intType := &model.TypeInfo{Name: "int", FullName: "int", IsBasic: true, Kind: model.KindBasic}

	// Define named source type: type MyInt int
	srcNamedType := &model.TypeInfo{
		Name:        "MyInt",
		FullName:    "example.com/mypkg.MyInt",
		PackagePath: "example.com/mypkg",
		Kind:        model.KindNamed,
		Underlying:  intType,
	}
	// Define named destination type: type YourInt int
	dstNamedType := &model.TypeInfo{
		Name:        "YourInt",
		FullName:    "example.com/mypkg.YourInt",
		PackagePath: "example.com/mypkg",
		Kind:        model.KindNamed,
		Underlying:  intType,
	}

	srcField := model.FieldInfo{Name: "MyAge", TypeInfo: srcNamedType}
	dstField := model.FieldInfo{Name: "YourAge", TypeInfo: dstNamedType}
	srcField.Tag.DstFieldName = "YourAge" // Ensure mapping

	srcStructInfo := &model.StructInfo{Name: "Src", Fields: []model.FieldInfo{srcField}, Type: &model.TypeInfo{Name: "Src", FullName: "example.com/mypkg.Src"}}
	dstStructInfo := &model.StructInfo{Name: "Dst", Fields: []model.FieldInfo{dstField}, Type: &model.TypeInfo{Name: "Dst", FullName: "example.com/mypkg.Dst"}}
	srcField.ParentStruct = srcStructInfo
	dstField.ParentStruct = dstStructInfo

	parsedInfo := model.NewParsedInfo("mypkg", "example.com/mypkg")
	parsedInfo.Structs["Src"] = srcStructInfo
	parsedInfo.Structs["Dst"] = dstStructInfo
	// Add named types to ParsedInfo so resolveTypeFromString can find them if needed by some internal logic, though not strictly necessary for this test's TypeInfo setup
	parsedInfo.NamedTypes["MyInt"] = srcNamedType
	parsedInfo.NamedTypes["YourInt"] = dstNamedType


	srcStructType := &model.TypeInfo{Name: "Src", FullName: "example.com/mypkg.Src", Kind: model.KindStruct, StructInfo: srcStructInfo, PackagePath: "example.com/mypkg"}
	dstStructType := &model.TypeInfo{Name: "Dst", FullName: "example.com/mypkg.Dst", Kind: model.KindStruct, StructInfo: dstStructInfo, PackagePath: "example.com/mypkg"}


	var buf bytes.Buffer
	imports := make(map[string]string)
	worklist := new([]model.ConversionPair)
	processedPairs := make(map[string]bool)

	err := generateHelperFunction(&buf, "srcToDst", srcStructType, dstStructType, parsedInfo, imports, worklist, processedPairs)
	if err != nil {
		t.Fatalf("generateHelperFunction failed: %v", err)
	}

	generatedCode := buf.String()
	expectedBody := `
	// Mapping field Src.MyAge (example.com/mypkg.MyInt) to Dst.YourAge (example.com/mypkg.YourInt)
	// Src: Ptr=false, ElemFull=example.com/mypkg.MyInt | Dst: Ptr=false, ElemFull=example.com/mypkg.YourInt
	ec.Enter("YourAge")
	dst.YourAge = YourInt(src.MyAge)
	ec.Leave()
	// The "if ec.MaxErrorsReached() { return dst }" line is part of the general function structure
	// and not specific to this field's logic block in the same way ec.Leave() is.
	// extractRelevantBody might or might not include it based on its heuristics.
	// For now, we assume extractRelevantBody primarily captures up to ec.Leave().
	// The normalizeCode function will handle trimming, so an extra "if ec.MaxErrorsReached..."
	// in the generated part but not in expected should be caught if extractRelevantBody includes it.
	// Let's keep expectedBody minimal up to the core logic and ec.Leave().
`
	// expectedFullFunc := fmt.Sprintf(`func srcToDst(ec *errorCollector, src mypkg.Src) mypkg.Dst {
	// dst := mypkg.Dst{}
	// if ec.MaxErrorsReached() { return dst }
	// %s
	// return dst
	// }
	// `, expectedBody) // Note: types in signature are simplified here for brevity of test setup

	formattedGenerated, _ := formatCode(generatedCode)
	// formattedExpected, _ := formatCode(expectedFullFunc) // Removed as it's unused

	// For better error reporting, compare normalized versions of the relevant parts
	normalizedGeneratedBody := normalizeCode(extractRelevantBody(generatedCode, "MyAge", "YourAge"))
	normalizedExpectedBody := normalizeCode(expectedBody)


	if normalizedGeneratedBody != normalizedExpectedBody {
		t.Errorf("TestGenerateHelperFunction_Underlying_MyInt_To_YourInt mismatch:\n---EXPECTED BODY---\n%s\n---GENERATED BODY---\n%s\n\n---FULL GENERATED---\n%s", normalizedExpectedBody, normalizedGeneratedBody, formattedGenerated)
	}
}


func TestGenerateHelperFunction_Underlying_MyFloatPtr_To_StarFloat64(t *testing.T) {
	// Base type: float64
	float64Type := &model.TypeInfo{Name: "float64", FullName: "float64", IsBasic: true, Kind: model.KindBasic}

	// Named type: type MyFloat float64
	myFloatType := &model.TypeInfo{
		Name:        "MyFloat",
		FullName:    "example.com/mypkg.MyFloat",
		PackagePath: "example.com/mypkg",
		Kind:        model.KindNamed,
		Underlying:  float64Type,
	}
	// Pointer to named type: type MyFloatPtr *MyFloat
	srcPtrToNamedType := &model.TypeInfo{
		Name:        "MyFloatPtr", // This is the TypeInfo for the field type MyFloatPtr
		FullName:    "example.com/mypkg.MyFloatPtr", // This is the TypeInfo for the field type MyFloatPtr
		PackagePath: "example.com/mypkg",
		Kind:        model.KindNamed, // MyFloatPtr is a named type, its underlying is a pointer
		Underlying: &model.TypeInfo{ // Underlying of MyFloatPtr is *MyFloat
			Name:        "*MyFloat", // Name for *MyFloat might be just MyFloat if PkgName is used as prefix by typeNameInSource
			FullName:    "*example.com/mypkg.MyFloat",
			PackagePath: "example.com/mypkg",
			IsPointer:   true,
			Kind:        model.KindPointer,
			Elem:        myFloatType,
		},
	}
	// Destination type: *float64
	dstPtrToBasicType := &model.TypeInfo{
		Name:        "*float64", // Name for *float64 might be just float64
		FullName:    "*float64",
		PackagePath: "", // Basic types and pointers to them don't have a package path in this context
		IsPointer:   true,
		Kind:        model.KindPointer,
		Elem:        float64Type,
	}

	srcField := model.FieldInfo{Name: "MaybeValue", TypeInfo: srcPtrToNamedType} // Field type is MyFloatPtr
	dstField := model.FieldInfo{Name: "MaybeValue", TypeInfo: dstPtrToBasicType} // Field type is *float64

	srcStructInfo := &model.StructInfo{Name: "Src", Fields: []model.FieldInfo{srcField}, Type: &model.TypeInfo{Name: "Src", FullName: "example.com/mypkg.Src"}}
	dstStructInfo := &model.StructInfo{Name: "Dst", Fields: []model.FieldInfo{dstField}, Type: &model.TypeInfo{Name: "Dst", FullName: "example.com/mypkg.Dst"}}
	srcField.ParentStruct = srcStructInfo
	dstField.ParentStruct = dstStructInfo

	parsedInfo := model.NewParsedInfo("mypkg", "example.com/mypkg")
	parsedInfo.Structs["Src"] = srcStructInfo
	parsedInfo.Structs["Dst"] = dstStructInfo
	parsedInfo.NamedTypes["MyFloat"] = myFloatType
	parsedInfo.NamedTypes["MyFloatPtr"] = srcPtrToNamedType


	srcStructType := &model.TypeInfo{Name: "Src", FullName: "example.com/mypkg.Src", Kind: model.KindStruct, StructInfo: srcStructInfo, PackagePath: "example.com/mypkg"}
	dstStructType := &model.TypeInfo{Name: "Dst", FullName: "example.com/mypkg.Dst", Kind: model.KindStruct, StructInfo: dstStructInfo, PackagePath: "example.com/mypkg"}

	var buf bytes.Buffer
	imports := make(map[string]string)
	worklist := new([]model.ConversionPair)
	processedPairs := make(map[string]bool)

	err := generateHelperFunction(&buf, "srcToDst", srcStructType, dstStructType, parsedInfo, imports, worklist, processedPairs)
	if err != nil {
		t.Fatalf("generateHelperFunction failed: %v", err)
	}
	generatedCode := buf.String()

	// Note: The TypeInfo for MyFloatPtr's field is complex.
	// srcField.TypeInfo.FullName is "example.com/mypkg.MyFloatPtr"
	// dstField.TypeInfo.FullName is "*float64"

	// DEBUG_SRC_FIELD for MyFloatPtr field: Name should be MyFloatPtr, FullName example.com/mypkg.MyFloatPtr, IsBasic=false, Kind=named
	// DEBUG_SRC_FIELD_UNDERLYING for MyFloatPtr: Name=*MyFloat, FullName=*example.com/mypkg.MyFloat, IsBasic=false, Kind=pointer
	// DEBUG_DST_FIELD for *float64 field: Name=*float64, FullName=*float64, IsBasic=false, Kind=pointer
	// DEBUG_SRC_ACTUAL_UNDERLYING (from getUnderlyingTypeInfo(MyFloatPtr)): should be float64 TypeInfo
	// DEBUG_DST_ACTUAL_UNDERLYING (from getUnderlyingTypeInfo(*float64)): should be float64 TypeInfo

	expectedBody := `
	// Mapping field Src.MaybeValue (example.com/mypkg.MyFloatPtr) to Dst.MaybeValue (*float64)
	// Src: Ptr=false, ElemFull=example.com/mypkg.MyFloatPtr | Dst: Ptr=true, ElemFull=float64
	// Note: The above "Src: Ptr=false" is because srcField.TypeInfo for MyFloatPtr is KindNamed, not KindPointer directly.
	// This might be a slight inaccuracy in the initial "// Src: Ptr=..." comment line generation if it doesn't look at the underlying of MyFloatPtr.
	// The important part is the DEBUG comments and the generated logic.
	ec.Enter("MaybeValue")
	if src.MaybeValue != nil {
		convertedVal := float64(*src.MaybeValue)
		dst.MaybeValue = &convertedVal
	} else {
		dst.MaybeValue = nil
	}
	ec.Leave()
	// Similar to the MyInt_To_YourInt test, we expect extractRelevantBody to capture
	// the core logic up to ec.Leave().
`
	// The `*src.MaybeValue` implies MyFloatPtr is dereferenceable like a pointer to MyFloat.
	// This needs MyFloatPtr to be treated as *MyFloat in the srcAccessPath logic.
	// If MyFloatPtr is just a named type `type MyFloatPtr *MyFloat`, then `src.MaybeValue` is of type `*MyFloat`.
	// So `*src.MaybeValue` is `MyFloat`. Then `float64(MyFloat)` is correct.

	// expectedFullFunc := fmt.Sprintf(`func srcToDst(ec *errorCollector, src mypkg.Src) mypkg.Dst {
	// dst := mypkg.Dst{}
	// if ec.MaxErrorsReached() { return dst }
	// %s
	// return dst
	// }
	// `, expectedBody)

	formattedGenerated, _ := formatCode(generatedCode)
	// formattedExpected, _ := formatCode(expectedFullFunc) // Removed as it's unused

	normalizedGeneratedBody := normalizeCode(extractRelevantBody(generatedCode, "MaybeValue", "MaybeValue"))
	normalizedExpectedBody := normalizeCode(expectedBody)

	if normalizedGeneratedBody != normalizedExpectedBody {
		t.Errorf("TestGenerateHelperFunction_Underlying_MyFloatPtr_To_StarFloat64 mismatch:\n---EXPECTED BODY---\n%s\n---GENERATED BODY---\n%s\n\n---FULL GENERATED---\n%s", normalizedExpectedBody, normalizedGeneratedBody, formattedGenerated)
	}
}


// extractRelevantBody is a helper to get only the part of the generated function
// related to a specific field mapping for easier comparison in tests.
// It's a bit simplistic and might need adjustment.
// It now attempts to exclude DEBUG comments.
func extractRelevantBody(fullCode, srcFieldName, dstFieldName string) string {
	var relevantLines []string
	inRelevantBlock := false
	lines := strings.Split(fullCode, "\n")

	// More generic mapping comment search to be less dependent on exact struct name "Src"
	// This regex will look for "// Mapping field ANY_STRUCT.srcFieldName (ANY_TYPE) to ANY_STRUCT.dstFieldName (ANY_TYPE)"
	// For simplicity in this step, we'll stick to a simpler string search, assuming the tests
	// use consistent source/destination struct names or the current logic is sufficient.
	// A more robust solution might involve regex if struct names vary significantly across tests using this helper.
	mappingCommentStart := fmt.Sprintf("// Mapping field ") // General start
	// Construct a more specific search if possible, but allow fallback
	specificMappingComment := fmt.Sprintf("// Mapping field %s.%s", "Src", srcFieldName) // Defaulting to "Src" as before.
	if dstFieldName != srcFieldName {
		specificMappingComment = fmt.Sprintf("// Mapping field %s.%s (%s) to %s.%s", "Src", srcFieldName, ".*", "Dst", dstFieldName)
		// This specific comment is getting too complex for simple string matching if types are involved.
		// Let's simplify the trigger for starting the block.
		specificMappingComment = fmt.Sprintf("// Mapping field %s.%s", "Src", srcFieldName)
	}


	foundMappingComment := false
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		if !inRelevantBlock {
			// Try to find the start of the block
			// Prefer specific mapping comment if Src name is known and consistent
			if strings.HasPrefix(trimmedLine, specificMappingComment) || (strings.HasPrefix(trimmedLine, mappingCommentStart) && strings.Contains(trimmedLine, srcFieldName) && strings.Contains(trimmedLine, dstFieldName)) {
				inRelevantBlock = true
				foundMappingComment = true // Mark that we found the entry point
				// The mapping comment itself can be included if not a DEBUG line, or excluded if desired
				if !strings.HasPrefix(trimmedLine, "// DEBUG_") {
					relevantLines = append(relevantLines, line)
				}
				continue
			}
		}

		if inRelevantBlock {
			// Exclude DEBUG lines
			if !strings.HasPrefix(trimmedLine, "// DEBUG_") {
				relevantLines = append(relevantLines, line)
			}

			if strings.HasPrefix(trimmedLine, "ec.Leave()") {
				// We've reached the end of the logical block for this field.
				// The next line is typically "if ec.MaxErrorsReached()...", which we also want to include.
				// So, find the next line, add it if it's the error check, then break.
				// This is still a bit heuristic.
				// For now, just include ec.Leave() and break.
				// The "if ec.MaxErrorsReached()" part might be better handled by normalizing the whole function body
				// if this extraction becomes too fragile.
				break
			}
		}
	}

	if !foundMappingComment {
		// Fallback or error if the mapping comment isn't found.
		// This might indicate an issue with test setup or generated code not matching expectations.
		// For now, return what might have been collected, or empty if nothing.
		// Returning the whole code if block not found can make debugging harder.
		// Let's return empty if no specific block was isolated.
		// This makes test failures clearer if `extractRelevantBody` fails.
		// However, the original code had a fallback to return `fullCode`.
		// Let's try returning a specific error message or a marker.
		// For the purpose of this step, an empty string if not found is a clear signal.
		// If tests fail due to this, we'll know `extractRelevantBody` is the cause.
		if len(relevantLines) == 0 {
			// Try a simpler search if the specific one failed, focusing only on ec.Enter and ec.Leave
			// This is a secondary fallback.
			ecEnterLine := fmt.Sprintf(`ec.Enter("%s")`, dstFieldName)
			ecLeaveLine := `ec.Leave()`
			blockLines := []string{}
			inEcBlock := false
			for _, line := range lines {
				trimmedLine := strings.TrimSpace(line)
				if strings.HasPrefix(trimmedLine, ecEnterLine) {
					inEcBlock = true
				}
				if inEcBlock {
					if !strings.HasPrefix(trimmedLine, "// DEBUG_") {
						blockLines = append(blockLines, line)
					}
					if strings.HasPrefix(trimmedLine, ecLeaveLine) {
						// Also capture the "if ec.MaxErrorsReached()" that follows
						// This needs a lookahead or post-processing.
						// For now, just the block up to ec.Leave().
						return strings.Join(blockLines, "\n")
					}
				}
			}
		}
	}
	return strings.Join(relevantLines, "\n")
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
		Tag:      model.ConvertTag{Required: false, DstFieldName: "MyString"}, // Default behavior
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

	// DEBUG: Number of source fields: 1 for struct Src
	// DEBUG: Processing source field: MyStringPtr
	// DEBUG: dstFieldName = MyString, dstField is nil = false
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
		Tag:      model.ConvertTag{Required: true, DstFieldName: "MyRequiredString"},
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

	// DEBUG: Number of source fields: 1 for struct Src
	// DEBUG: Processing source field: MyRequiredStringPtr
	// DEBUG: dstFieldName = MyRequiredString, dstField is nil = false
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
		Tag:      model.ConvertTag{Required: true, DstFieldName: "MyRequiredStringNN"},
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

	// DEBUG: Number of source fields: 1 for struct Src
	// DEBUG: Processing source field: MyRequiredStringPtrNN
	// DEBUG: dstFieldName = MyRequiredStringNN, dstField is nil = false
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
		Tag:      model.ConvertTag{DstFieldName: "MyPtrDst"},
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

	// DEBUG: Number of source fields: 1 for struct Src
	// DEBUG: Processing source field: MyPtrSrc
	// DEBUG: dstFieldName = MyPtrDst, dstField is nil = false
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
		Tag:      model.ConvertTag{UsingFunc: "IntToStringConverter", DstFieldName: "DstString"},
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

	// DEBUG: Number of source fields: 1 for struct Src
	// DEBUG: Processing source field: SrcInt
	// DEBUG: dstFieldName = DstString, dstField is nil = false
	// Mapping field %s.SrcInt (%s) to %s.DstString (%s)
	// Src: Ptr=false, ElemFull=int | Dst: Ptr=false, ElemFull=string
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
		Tag:      model.ConvertTag{DstFieldName: "DstDecimal"}, // No field tag using
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

	// DEBUG: Number of source fields: 1 for struct Src
	// DEBUG: Processing source field: SrcFloat
	// DEBUG: dstFieldName = DstDecimal, dstField is nil = false
	// Mapping field %s.SrcFloat (%s) to %s.DstDecimal (%s)
	// Src: Ptr=false, ElemFull=float64 | Dst: Ptr=false, ElemFull=custompkg.Decimal
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
