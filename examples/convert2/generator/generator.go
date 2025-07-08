package generator

import (
	"bytes"
	"fmt"
	"sort"

	// "go/ast" // No longer needed
	"go/format"
	// "go/token" // No longer needed
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"example.com/convert2/internal/model"
)

const fileSuffix = "_gen.go"

// GenerateConversionCode generates conversion functions based on parsedInfo
// and writes them to *_gen.go files in the outputDir.
func GenerateConversionCode(parsedInfo *model.ParsedInfo, outputDir string) error {
	// --- DEBUG ---
	fmt.Printf("DEBUG: ParsedInfo.ConversionPairs:\n")
	for i, cp := range parsedInfo.ConversionPairs {
		fmt.Printf("  Pair %d:\n", i)
		fmt.Printf("    SrcType: Name=%s, FullName=%s, Kind=%v, IsBasic=%t, IsPointer=%t\n", cp.SrcTypeInfo.Name, cp.SrcTypeInfo.FullName, cp.SrcTypeInfo.Kind, cp.SrcTypeInfo.IsBasic, cp.SrcTypeInfo.IsPointer)
		if cp.SrcTypeInfo.Underlying != nil {
			fmt.Printf("      Src Underlying: Name=%s, FullName=%s, Kind=%v, IsBasic=%t\n", cp.SrcTypeInfo.Underlying.Name, cp.SrcTypeInfo.Underlying.FullName, cp.SrcTypeInfo.Underlying.Kind, cp.SrcTypeInfo.Underlying.IsBasic)
		}
		if cp.SrcTypeInfo.Elem != nil {
			fmt.Printf("      Src Elem: Name=%s, FullName=%s, Kind=%v, IsBasic=%t\n", cp.SrcTypeInfo.Elem.Name, cp.SrcTypeInfo.Elem.FullName, cp.SrcTypeInfo.Elem.Kind, cp.SrcTypeInfo.Elem.IsBasic)
		}
		fmt.Printf("    DstType: Name=%s, FullName=%s, Kind=%v, IsBasic=%t, IsPointer=%t\n", cp.DstTypeInfo.Name, cp.DstTypeInfo.FullName, cp.DstTypeInfo.Kind, cp.DstTypeInfo.IsBasic, cp.DstTypeInfo.IsPointer)
		if cp.DstTypeInfo.Underlying != nil {
			fmt.Printf("      Dst Underlying: Name=%s, FullName=%s, Kind=%v, IsBasic=%t\n", cp.DstTypeInfo.Underlying.Name, cp.DstTypeInfo.Underlying.FullName, cp.DstTypeInfo.Underlying.Kind, cp.DstTypeInfo.Underlying.IsBasic)
		}
		if cp.DstTypeInfo.Elem != nil {
			fmt.Printf("      Dst Elem: Name=%s, FullName=%s, Kind=%v, IsBasic=%t\n", cp.DstTypeInfo.Elem.Name, cp.DstTypeInfo.Elem.FullName, cp.DstTypeInfo.Elem.Kind, cp.DstTypeInfo.Elem.IsBasic)
		}
	}
	// --- END DEBUG ---

	if len(parsedInfo.ConversionPairs) == 0 {
		fmt.Println("No conversion pairs defined. Nothing to generate.")
		return nil
	}

	outputFileName := fmt.Sprintf("%s%s", parsedInfo.PackageName, fileSuffix)
	outputFilePath := filepath.Join(outputDir, outputFileName)

	var generatedCode bytes.Buffer

	// --- Package and Imports ---
	// Initial set of imports. This will be expanded by addRequiredImport.
	// Map key is import path, value is preferred alias (can be empty if no specific alias needed initially)
	requiredImports := make(map[string]string)
	requiredImports["context"] = "context" // path -> alias
	requiredImports["fmt"] = "fmt"         // For error messages in errorCollector & Sprintf
	requiredImports["errors"] = "errors"   // For errors.Join
	requiredImports["strings"] = "strings" // For strings.Join in errorCollector
	requiredImports["sort"] = "sort"       // For sorting unmapped fields in docstring

	// --- Generate Error Collector ---
	// The errorCollectorTemplate itself does not declare imports.
	// The necessary imports (fmt, strings, errors) are added above.
	errCollectorTmpl, err := template.New("errorCollector").Parse(errorCollectorTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse errorCollector template: %w", err)
	}
	err = errCollectorTmpl.Execute(&generatedCode, nil)
	if err != nil {
		return fmt.Errorf("failed to execute errorCollector template: %w", err)
	}
	generatedCode.WriteString("\n\n")

	// --- Generate Conversion Functions ---
	// generatedFunctionBodies stores the actual source code of the generated functions.
	// Key: function name, Value: function source code.
	generatedFunctionBodies := make(map[string]string)

	// conversionWorklist stores pairs that need helper functions to be generated.
	// Initialize with top-level conversion pairs from parser.
	conversionWorklist := make([]model.ConversionPair, 0, len(parsedInfo.ConversionPairs))
	for _, cp := range parsedInfo.ConversionPairs {
		// Make a copy to avoid modifying the original slice from parsedInfo if the worklist is modified.
		conversionWorklist = append(conversionWorklist, cp)
	}

	// processedPairs tracks (SrcTypeFullName, DstTypeFullName) to avoid redundant generation
	// and infinite loops for recursive types (though full recursion detection is more complex).
	// Key: "SrcFullName->DstFullName"
	processedPairs := make(map[string]bool)

	// Loop as long as there are conversion pairs needing helper functions.
	// generateHelperFunction might add new pairs to conversionWorklist for nested types.
	idx := 0
	for idx < len(conversionWorklist) {
		pair := conversionWorklist[idx]
		idx++ // Move to next item early, in case new items are appended

		pairKey := fmt.Sprintf("%s->%s", pair.SrcTypeInfo.FullName, pair.DstTypeInfo.FullName)
		if processedPairs[pairKey] {
			continue // Already processed or in queue
		}
		// Mark as processed *before* calling generateHelperFunction to handle direct recursion
		// (e.g. A converts to A, or A to B and B to A)
		processedPairs[pairKey] = true

		coreHelperFuncName := fmt.Sprintf("%sTo%s", lowercaseFirstLetter(pair.SrcTypeInfo.Name), pair.DstTypeInfo.Name)
		if _, exists := generatedFunctionBodies[coreHelperFuncName]; !exists {
			var helperFuncBuf bytes.Buffer
			// Pass the worklist so generateHelperFunction can add to it.
			err := generateHelperFunction(&helperFuncBuf, coreHelperFuncName, pair.SrcTypeInfo, pair.DstTypeInfo, parsedInfo, requiredImports, &conversionWorklist, processedPairs)
			if err != nil {
				return fmt.Errorf("failed to generate helper function %s: %w", coreHelperFuncName, err)
			}
			generatedFunctionBodies[coreHelperFuncName] = helperFuncBuf.String()
		}
	}

	// Then, generate top-level functions that call these helpers
	for _, pair := range parsedInfo.ConversionPairs { // Iterate original pairs for top-level functions
		topLevelFuncName := fmt.Sprintf("Convert%sTo%s", pair.SrcTypeInfo.Name, pair.DstTypeInfo.Name) // Make top-level func name more specific
		if _, exists := generatedFunctionBodies[topLevelFuncName]; exists {
			fmt.Printf("Warning: Top-level function %s already generated. Skipping duplicate generation.\n", topLevelFuncName)
			continue
		}

		var funcBuf bytes.Buffer
		coreHelperFuncName := fmt.Sprintf("%sTo%s", lowercaseFirstLetter(pair.SrcTypeInfo.Name), pair.DstTypeInfo.Name)

		addRequiredImport(pair.SrcTypeInfo, parsedInfo.PackagePath, requiredImports)
		addRequiredImport(pair.DstTypeInfo, parsedInfo.PackagePath, requiredImports)

		fmt.Fprintf(&funcBuf, "func %s(ctx context.Context, src %s) (%s, error) {\n",
			topLevelFuncName,
			typeNameInSource(pair.SrcTypeInfo, parsedInfo.PackagePath, requiredImports),
			typeNameInSource(pair.DstTypeInfo, parsedInfo.PackagePath, requiredImports))
		fmt.Fprintf(&funcBuf, "\tec := newErrorCollector(%d)\n", pair.MaxErrors) // Use MaxErrors from the pair
		fmt.Fprintf(&funcBuf, "\tdst := %s(ec, src)\n", coreHelperFuncName)
		fmt.Fprintf(&funcBuf, "\tif ec.HasErrors() {\n")
		// Ensure dst is always returned, even if zero value, to match signature
		fmt.Fprintf(&funcBuf, "\t\treturn dst, errors.Join(ec.Errors()...)\n")
		fmt.Fprintf(&funcBuf, "\t}\n")
		fmt.Fprintf(&funcBuf, "\treturn dst, nil\n")
		fmt.Fprintf(&funcBuf, "}\n\n")

		generatedFunctionBodies[topLevelFuncName] = funcBuf.String()
	}

	// --- Assemble Final Code ---
	var finalCode bytes.Buffer
	fmt.Fprintf(&finalCode, "// Code generated by convert2 tool. DO NOT EDIT.\n")
	fmt.Fprintf(&finalCode, "package %s\n\n", parsedInfo.PackageName)

	// Write imports
	if len(requiredImports) > 0 {
		finalCode.WriteString("import (\n")

		// Sort imports by path for consistent output
		importPaths := make([]string, 0, len(requiredImports))
		for path := range requiredImports {
			importPaths = append(importPaths, path)
		}
		// It's better to sort for deterministic output, rather than relying on Go team's tools.
		// However, gofmt/goimports will ultimately format it. So, direct iteration is fine for now.
		// For stability in tests before gofmt, sorting can be helpful. Let's add a simple sort.
		sortedImportPaths := make([]string, 0, len(requiredImports))
		for path := range requiredImports {
			sortedImportPaths = append(sortedImportPaths, path)
		}
		// Simple string sort for paths
		// For more complex alias sorting, would need custom logic
		// sort.Strings(sortedImportPaths) // Actually, map iteration order is fine, gofmt will fix.

		for importPath, alias := range requiredImports { // Iterate directly, gofmt will handle final order
			// If the determined alias is the same as the package's actual name (last part of path),
			// or if no specific alias was set (alias derived by typeNameInSource/addRequiredImport logic),
			// then no explicit alias is needed in the import statement.
			// Example: import "time" (alias "time", path "time")
			// Example: import "example.com/custom/mypkg" (alias "mypkg", path "example.com/custom/mypkg")
			baseName := filepath.Base(importPath)
			if alias == baseName || alias == "" { // if alias is empty, it means use basename
				fmt.Fprintf(&finalCode, "\t\"%s\"\n", importPath)
			} else {
				fmt.Fprintf(&finalCode, "\t%s \"%s\"\n", alias, importPath)
			}
		}
		finalCode.WriteString(")\n\n")
	}

	finalCode.Write(generatedCode.Bytes()) // Error collector code

	// Write helper functions first
	for funcName, funcBody := range generatedFunctionBodies {
		if !strings.HasPrefix(funcName, "Convert") { // Heuristic for helpers
			finalCode.WriteString(funcBody)
		}
	}
	// Then top-level functions
	for funcName, funcBody := range generatedFunctionBodies {
		if strings.HasPrefix(funcName, "Convert") {
			finalCode.WriteString(funcBody)
		}
	}

	// --- Format and Write to File ---
	formattedCode, err := format.Source(finalCode.Bytes())
	if err != nil {
		os.WriteFile(outputFilePath+".unformatted", finalCode.Bytes(), 0644)
		return fmt.Errorf("failed to format generated code for %s: %w\nUnformatted code saved to %s.unformatted", outputFileName, err, outputFilePath)
	}

	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}

	err = os.WriteFile(outputFilePath, formattedCode, 0644)
	if err != nil {
		return fmt.Errorf("failed to write generated code to %s: %w", outputFilePath, err)
	}

	fmt.Printf("Generated conversion code at %s\n", outputFilePath)
	return nil
}

// generateHelperFunction generates the non-exported core conversion logic.
// It can add new required conversion pairs to the worklist for nested structs.
func generateHelperFunction(
	buf *bytes.Buffer,
	funcName string,
	srcType, dstType *model.TypeInfo,
	parsedInfo *model.ParsedInfo,
	imports map[string]string,
	worklist *[]model.ConversionPair, // Pointer to the shared worklist
	processedPairs map[string]bool, // Pointer to the shared set of processed pairs
) error {
	if srcType.StructInfo == nil || dstType.StructInfo == nil {
		// Handle non-struct or unresolved struct types (e.g. named basic types)
		// This part will require using global conversion rules or direct assignment if types are compatible.
		fmt.Fprintf(buf, "// Helper function %s for types %s -> %s\n", funcName, srcType.FullName, dstType.FullName)
		fmt.Fprintf(buf, "// One or both types are not structs, or struct info not found.\n")
		fmt.Fprintf(buf, "func %s(ec *errorCollector, src %s) %s {\n",
			funcName,
			typeNameInSource(srcType, parsedInfo.PackagePath, imports), // typeNameInSource は一時的に単純化されたまま
			typeNameInSource(dstType, parsedInfo.PackagePath, imports))
		// TODO: Implement conversion for non-structs or when one is not a struct.
		// This could involve checking for global 'using' rules, or direct assignment/casting if types are compatible.
		// Example: if types are `type Meter int32` and `type Centimeter int32`
		fmt.Fprintf(buf, "\t// Placeholder for non-struct conversion logic.\n")
		if srcType.FullName == dstType.FullName || (srcType.Underlying != nil && srcType.Underlying.FullName == dstType.FullName) || (dstType.Underlying != nil && dstType.Underlying.FullName == srcType.FullName) {
			fmt.Fprintf(buf, "\treturn %s(src) // Assuming direct cast is possible\n", typeNameInSource(dstType, parsedInfo.PackagePath, imports))
		} else {
			fmt.Fprintf(buf, "\tvar dst %s\n", typeNameInSource(dstType, parsedInfo.PackagePath, imports))
			fmt.Fprintf(buf, "\tec.Addf(\"conversion from %s to %s not implemented for non-structs or mismatched types\")\n", srcType.FullName, dstType.FullName)
			fmt.Fprintf(buf, "\treturn dst\n")
		}
		fmt.Fprintf(buf, "}\n\n")
		// Imports for srcType and dstType should have been added when they were first encountered or used.
		// Calling addRequiredImport here again is fine, it's idempotent.
		addRequiredImport(srcType, parsedInfo.PackagePath, imports)
		addRequiredImport(dstType, parsedInfo.PackagePath, imports)
		return nil
	}

	srcStruct := srcType.StructInfo
	dstStruct := dstType.StructInfo

	// Add imports for src and dst types of this helper function itself.
	// This is important if srcType or dstType are from external packages.
	addRequiredImport(srcType, parsedInfo.PackagePath, imports)
	addRequiredImport(dstType, parsedInfo.PackagePath, imports)

	// Track destination fields that are not set
	unmappedDstFields := make(map[string]bool)
	for _, dstF := range dstStruct.Fields {
		unmappedDstFields[dstF.Name] = true
	}

	var functionBodyBuf bytes.Buffer
	fmt.Fprintf(&functionBodyBuf, "\tdst := %s{}\n", typeNameInSource(dstType, parsedInfo.PackagePath, imports))
	fmt.Fprintf(&functionBodyBuf, "\tif ec.MaxErrorsReached() { return dst } \n\n")

	fmt.Fprintf(&functionBodyBuf, "\t// DEBUG: Number of source fields: %d for struct %s\n", len(srcStruct.Fields), srcStruct.Name) // DEBUG LINE
	// Iterate over source struct fields to apply rules and find destinations
	for _, srcField := range srcStruct.Fields {
		fmt.Fprintf(&functionBodyBuf, "\t// DEBUG: Processing source field: %s\n", srcField.Name) // DEBUG LINE
		if srcField.Tag.DstFieldName == "-" {
			fmt.Fprintf(&functionBodyBuf, "\t// Source field %s.%s is skipped due to tag '-'.\n", srcStruct.Name, srcField.Name)
			continue
		}

		// The block above this comment (handling original dstFieldName and dstField lookup)
		// is the one that should be removed if it's causing a redeclaration.
		// The new logic starts below with DEBUG_SRC_FIELD_TAG_RAW.

		// DEBUG statements for Tag info
		fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_SRC_FIELD_TAG_RAW: %s\n", srcField.Tag.RawValue)
		fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_SRC_FIELD_TAG_DSTFIELDNAME: %s\n", srcField.Tag.DstFieldName)

		var resolvedDstFieldName string // Explicitly declare
		if srcField.Tag.DstFieldName != "" {
			resolvedDstFieldName = srcField.Tag.DstFieldName
			fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_DSTFIELDNAME_FROM_TAG: Used tag DstFieldName (%s)\n", resolvedDstFieldName)
		} else {
			resolvedDstFieldName = srcField.Name
			fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_DSTFIELDNAME_FALLBACK: Used srcField.Name (%s)\n", resolvedDstFieldName)
		}

		var dstField *model.FieldInfo // Declare dstField here before use
		for i := range dstStruct.Fields {
			if dstStruct.Fields[i].Name == resolvedDstFieldName { // Use the resolved name
				dstField = &dstStruct.Fields[i]
				break
			}
		}

		if dstField == nil {
			// Warning or info message based on whether DstFieldName was from tag or fallback
			fmt.Fprintf(&functionBodyBuf, "\t// Info: No destination field named '%s' (determined from tag or src name) found in '%s' to match source field %s.%s. Field skipped.\n", resolvedDstFieldName, dstStruct.Name, srcStruct.Name, srcField.Name)
			continue
		}
		delete(unmappedDstFields, dstField.Name) // Mark as mapped

		// Found source field and corresponding destination field.
		// Now, implement basic direct assignment if types match.
		fmt.Fprintf(&functionBodyBuf, "\t// Mapping field %s.%s (%s) to %s.%s (%s)\n",
			srcStruct.Name, srcField.Name, srcField.TypeInfo.FullName,
			dstStruct.Name, dstField.Name, dstField.TypeInfo.FullName) // Note: dstField.Name should be used here, not dstFieldNameDetermined if they could differ by logic error

		srcElemCommentName := "nil"
		if srcField.TypeInfo.Elem != nil {
			srcElemCommentName = srcField.TypeInfo.Elem.FullName
		} else if !srcField.TypeInfo.IsPointer && !srcField.TypeInfo.IsSlice && !srcField.TypeInfo.IsMap {
			srcElemCommentName = srcField.TypeInfo.FullName // For non-composite, non-pointer types, Elem is the type itself
		}
		dstElemCommentName := "nil"
		if dstField.TypeInfo.Elem != nil {
			dstElemCommentName = dstField.TypeInfo.Elem.FullName
		} else if !dstField.TypeInfo.IsPointer && !dstField.TypeInfo.IsSlice && !dstField.TypeInfo.IsMap {
			dstElemCommentName = dstField.TypeInfo.FullName // For non-composite, non-pointer types, Elem is the type itself
		}
		fmt.Fprintf(&functionBodyBuf, "\t// Src: Ptr=%t, ElemFull=%s | Dst: Ptr=%t, ElemFull=%s\n",
			srcField.TypeInfo.IsPointer, srcElemCommentName,
			dstField.TypeInfo.IsPointer, dstElemCommentName)
		fmt.Fprintf(&functionBodyBuf, "\tec.Enter(%q)\n", dstField.Name) // Path uses DstFieldName

		// Add imports for field types
		addRequiredImport(srcField.TypeInfo, parsedInfo.PackagePath, imports)
		addRequiredImport(dstField.TypeInfo, parsedInfo.PackagePath, imports)

		var appliedUsingFunc string
		var usingFuncIsGlobal bool

		// Priority 1: Field Tag `using=<funcName>`
		if srcField.Tag.UsingFunc != "" {
			appliedUsingFunc = srcField.Tag.UsingFunc
			usingFuncIsGlobal = false
		} else {
			// Priority 2: Global Rule `convert:rule "<SrcT>" -> "<DstT>", using=<funcName>`
			for _, rule := range parsedInfo.GlobalRules {
				if rule.UsingFunc != "" && rule.SrcTypeInfo != nil && rule.DstTypeInfo != nil &&
					rule.SrcTypeInfo.FullName == srcField.TypeInfo.FullName &&
					rule.DstTypeInfo.FullName == dstField.TypeInfo.FullName {
					appliedUsingFunc = rule.UsingFunc
					usingFuncIsGlobal = true
					break
				}
			}
		}

		if appliedUsingFunc != "" {
			// Handle potential package prefix in appliedUsingFunc for imports
			funcCallName := appliedUsingFunc
			if parts := strings.Split(appliedUsingFunc, "."); len(parts) == 2 {
				// Assuming a simple pkg.Func format for now
				// This needs robust package alias resolution based on actual imports in the source
				// For now, if it contains ".", assume the first part is a package selector.
				// The actual import path for this selector needs to be available.
				// Let's assume the parser stores TypeInfo for func types or we resolve it here.
				// For simplicity, if func name is "pkg.MyFunc", ensure "pkg" (its path) is imported.
				// This is a placeholder for proper import resolution for the custom function's package.
				// The function type itself does not have a TypeInfo here easily.
				// We'd typically need to look up "pkg" in file imports to get its path.
				// For now, just use the prefix as is and rely on goimports or user to have correct imports.
				// A slightly better heuristic: if `appliedUsingFunc` contains a dot,
				// assume the part before the dot is a package that needs to be imported.
				// The `addRequiredImport` function might need to be smarter or take the function name.
				// For now, let's assume the function is in the same package or its package is handled by typeNameInSource logic if it were a type.
				// This is complex. Simplification: assume func is accessible.
				// If appliedUsingFunc is "mypackage.MyFunction", then typeNameInSource for a type from "mypackage"
				// would ensure "mypackage" is imported. We can try to leverage that.

				// Let's try to add import for the package part of the using function.
				if funcParts := strings.Split(appliedUsingFunc, "."); len(funcParts) == 2 {
					// This is a naive way to guess a package name and assume it needs import.
					// A proper solution would look up funcParts[0] in the file's imports list.
					// For now, we don't have that context directly here for the custom function.
					// We'll rely on the user ensuring the function is callable.
					// If the function is in another package, its import must be present in the generated file.
					// We can try to infer the import from the function name if it's qualified.
					// This is still problematic as we don't have the import *path* for pkgAlias.
					// For now, we generate the call as is. The user must ensure imports.
					// A future improvement would be for the parser to resolve function locations.
				}
			}

			if usingFuncIsGlobal {
				fmt.Fprintf(&functionBodyBuf, "\t// Applying global rule: %s -> %s using %s\n", srcField.TypeInfo.FullName, dstField.TypeInfo.FullName, appliedUsingFunc)
			} else {
				fmt.Fprintf(&functionBodyBuf, "\t// Applying field tag: using %s\n", appliedUsingFunc)
			}
			fmt.Fprintf(&functionBodyBuf, "\tdst.%s = %s(ec, src.%s)\n", dstField.Name, funcCallName, srcField.Name)

		} else {
			// Priority 3: Automatic Conversion (including pointer logic)
			srcIsPtr := srcField.TypeInfo.IsPointer
			dstIsPtr := dstField.TypeInfo.IsPointer
			srcElemTypeFullName := ""
			if srcField.TypeInfo.Elem != nil {
				srcElemTypeFullName = srcField.TypeInfo.Elem.FullName
			} else if !srcIsPtr {
				srcElemTypeFullName = srcField.TypeInfo.FullName
			}
			dstElemTypeFullName := ""
			if dstField.TypeInfo.Elem != nil {
				dstElemTypeFullName = dstField.TypeInfo.Elem.FullName
			} else if !dstIsPtr {
				dstElemTypeFullName = dstField.TypeInfo.FullName
			}

			typesMatchDirectly := srcField.TypeInfo.FullName == dstField.TypeInfo.FullName
			elementsMatch := srcElemTypeFullName != "" && dstElemTypeFullName != "" && srcElemTypeFullName == dstElemTypeFullName

			if typesMatchDirectly { // Case: T -> T or *T -> *T (elements must also match for *T -> *T, implied by FullName match)
				fmt.Fprintf(&functionBodyBuf, "\tdst.%s = src.%s\n", dstField.Name, srcField.Name)
			} else if !srcIsPtr && dstIsPtr && elementsMatch { // Case: T -> *T
				// Ensure that dstField.TypeInfo.Elem.FullName matches srcField.TypeInfo.FullName
				fmt.Fprintf(&functionBodyBuf, "\t{\n")
				fmt.Fprintf(&functionBodyBuf, "\t\tsrcVal := src.%s\n", srcField.Name)
				fmt.Fprintf(&functionBodyBuf, "\t\tdst.%s = &srcVal\n", dstField.Name)
				fmt.Fprintf(&functionBodyBuf, "\t}\n")
			} else if srcIsPtr && !dstIsPtr && elementsMatch { // Case: *T -> T
				// Ensure that srcField.TypeInfo.Elem.FullName matches dstField.TypeInfo.FullName
				if srcField.Tag.Required {
					fmt.Fprintf(&functionBodyBuf, "\tif src.%s == nil {\n", srcField.Name)
					fmt.Fprintf(&functionBodyBuf, "\t\tec.Addf(\"field '%s' is required but source field %s is nil\")\n", dstField.Name, srcField.Name)
					fmt.Fprintf(&functionBodyBuf, "\t} else {\n")
					fmt.Fprintf(&functionBodyBuf, "\t\tdst.%s = *src.%s\n", dstField.Name, srcField.Name)
					fmt.Fprintf(&functionBodyBuf, "\t}\n")
				} else {
					fmt.Fprintf(&functionBodyBuf, "\tif src.%s != nil {\n", srcField.Name)
					fmt.Fprintf(&functionBodyBuf, "\t\tdst.%s = *src.%s\n", dstField.Name, srcField.Name)
					fmt.Fprintf(&functionBodyBuf, "\t}\n") // If nil, dst field remains zero value, no error
				}
			} else {
				// Types do not match directly and pointer logic doesn't apply or elements mismatch.
				// Check for struct-to-struct conversion for nested types.
				// Ensure both srcField.TypeInfo and dstField.TypeInfo are actual struct types (not just named types that happen to be structs)
				// or that their underlying types are structs, if they are named types.
				// The StructInfo field being non-nil is a good indicator from the parser.
				// --- Revised logic to look up struct definitions ---
				tempSrcTypeInfo := srcField.TypeInfo
				isSrcFieldPointer := tempSrcTypeInfo.IsPointer
				if isSrcFieldPointer && tempSrcTypeInfo.Elem != nil {
					tempSrcTypeInfo = tempSrcTypeInfo.Elem
				}

				tempDstTypeInfo := dstField.TypeInfo
				isDstFieldPointer := tempDstTypeInfo.IsPointer
				if isDstFieldPointer && tempDstTypeInfo.Elem != nil {
					tempDstTypeInfo = tempDstTypeInfo.Elem
				}

				// Look up in parsedInfo.Structs.
				// Note: This assumes struct names are unique within the scope of what's parsed.
				// For types from different packages, TypeInfo.FullName (which includes package path)
				// would be needed for a truly robust lookup, or by checking TypeInfo.PackagePath.
				// currentParsedInfo.Structs is keyed by simple name for current package.
				var srcStructDef *model.StructInfo
				var srcIsStruct bool
				// Only look up if the type is from the current package or package path matches.
				// This simplistic check might need enhancement for imported struct types if their simple names clash.
				if tempSrcTypeInfo.PackagePath == parsedInfo.PackagePath || tempSrcTypeInfo.PackagePath == "" {
					srcStructDef, srcIsStruct = parsedInfo.Structs[tempSrcTypeInfo.Name]
				} else {
					// TODO: Handle lookup for structs from external packages if parsedInfo.Structs
					// were to include them keyed differently (e.g. by full name).
					// For now, this logic primarily supports nested structs from the same package.
					// A more robust lookup would iterate parsedInfo.Structs and check Type.FullName.
					// However, TypeInfo.Name for external types should be unique with its PackageName prefix from typeNameInSource.
					// This part is tricky: tempSrcTypeInfo.Name is the simple name.
					// We'd need to find a StructInfo whose `Type.FullName` matches `tempSrcTypeInfo.FullName`.
					// This is inefficient. The parser should ideally ensure TypeInfo.StructInfo is populated.
					// For now, let's assume TypeInfo.StructInfo IS populated by the parser for relevant types.
					srcIsStruct = tempSrcTypeInfo.StructInfo != nil
					if srcIsStruct {
						srcStructDef = tempSrcTypeInfo.StructInfo
					}
				}

				var dstStructDef *model.StructInfo
				var dstIsStruct bool
				if tempDstTypeInfo.PackagePath == parsedInfo.PackagePath || tempDstTypeInfo.PackagePath == "" {
					dstStructDef, dstIsStruct = parsedInfo.Structs[tempDstTypeInfo.Name]
				} else {
					dstIsStruct = tempDstTypeInfo.StructInfo != nil
					if dstIsStruct {
						dstStructDef = tempDstTypeInfo.StructInfo
					}
				}
				// Fallback to original check if lookup fails but direct StructInfo is present
				// This handles cases where TypeInfo might be an alias that itself has StructInfo populated.
				if !srcIsStruct && tempSrcTypeInfo.StructInfo != nil {
					srcIsStruct = true
					srcStructDef = tempSrcTypeInfo.StructInfo
				}
				if !dstIsStruct && tempDstTypeInfo.StructInfo != nil {
					dstIsStruct = true
					dstStructDef = tempDstTypeInfo.StructInfo
				}
				fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_STRUCT_CHECK: srcIsStruct=%t (Name: %s, StructInfoNil: %t, Kind: %v), dstIsStruct=%t (Name: %s, StructInfoNil: %t, Kind: %v)\n",
					srcIsStruct, tempSrcTypeInfo.Name, tempSrcTypeInfo.StructInfo == nil, tempSrcTypeInfo.Kind,
					dstIsStruct, tempDstTypeInfo.Name, tempDstTypeInfo.StructInfo == nil, tempDstTypeInfo.Kind)

				if srcIsStruct && dstIsStruct {
					// Use the TypeInfo from the canonical struct definition for the pair
					actualSrcFieldType := srcStructDef.Type
					actualDstFieldType := dstStructDef.Type

					fmt.Fprintf(&functionBodyBuf, "\t// Recursive call for nested struct %s -> %s\n", actualSrcFieldType.Name, actualDstFieldType.Name)
					nestedHelperFuncName := fmt.Sprintf("%sTo%s", lowercaseFirstLetter(actualSrcFieldType.Name), actualDstFieldType.Name)

					nestedPair := model.ConversionPair{
						SrcTypeInfo: actualSrcFieldType, // Use canonical TypeInfo from StructDef
						DstTypeInfo: actualDstFieldType, // Use canonical TypeInfo from StructDef
						MaxErrors:   0,
					}
					nestedPairKey := fmt.Sprintf("%s->%s", actualSrcFieldType.FullName, actualDstFieldType.FullName)

					if !processedPairs[nestedPairKey] {
						// Check if already in worklist to prevent duplicate appends from multiple fields using the same nested type
						alreadyInWorklist := false
						for _, item := range *worklist {
							if item.SrcTypeInfo.FullName == actualSrcFieldType.FullName && item.DstTypeInfo.FullName == actualDstFieldType.FullName {
								alreadyInWorklist = true
								break
							}
						}
						if !alreadyInWorklist {
							*worklist = append(*worklist, nestedPair)
						}
						// processedPairs is marked by the main loop before calling generateHelperFunction for this pair.
					}

					// Handle pointer nature of the *fields* themselves (srcField.TypeInfo, dstField.TypeInfo)
					// not actualSrcFieldType/actualDstFieldType which are definitions.
					// src: T, dst: T  => dst.F = helper(ec, src.F)
					// src: *T, dst: *T => if src.F != nil { val := helper(ec, *src.F); dst.F = &val } else { dst.F = nil }
					// src: T, dst: *T => val := helper(ec, src.F); dst.F = &val
					// src: *T, dst: T => if src.F != nil { dst.F = helper(ec, *src.F) } else { // error if required, else zero val }

					srcAccess := "src." + srcField.Name
					assignToDst := "dst." + dstField.Name + " = "

					if srcField.TypeInfo.IsPointer && !dstField.TypeInfo.IsPointer { // *S -> D
						fmt.Fprintf(&functionBodyBuf, "\tif src.%s != nil {\n", srcField.Name)
						fmt.Fprintf(&functionBodyBuf, "\t\t%s%s(ec, *%s)\n", assignToDst, nestedHelperFuncName, srcAccess)
						// TODO: Handle 'required' tag for src pointer if dst non-pointer needs it
						fmt.Fprintf(&functionBodyBuf, "\t} else {\n")
						if srcField.Tag.Required { // If source pointer is required for a non-pointer destination
							fmt.Fprintf(&functionBodyBuf, "\t\tec.Addf(\"field '%s' is required but source field %s for nested struct is nil\")\n", dstField.Name, srcField.Name)
						} else {
							fmt.Fprintf(&functionBodyBuf, "\t\t// Source field %s is nil, destination %s remains zero value\n", srcField.Name, dstField.Name)
						}
						fmt.Fprintf(&functionBodyBuf, "\t}\n")
					} else if !srcField.TypeInfo.IsPointer && dstField.TypeInfo.IsPointer { // S -> *D
						fmt.Fprintf(&functionBodyBuf, "\t{\n")
						fmt.Fprintf(&functionBodyBuf, "\t\tnestedVal := %s(ec, %s)\n", nestedHelperFuncName, srcAccess)
						fmt.Fprintf(&functionBodyBuf, "\t\t%s&nestedVal\n", assignToDst)
						fmt.Fprintf(&functionBodyBuf, "\t}\n")
					} else if srcField.TypeInfo.IsPointer && dstField.TypeInfo.IsPointer { // *S -> *D
						fmt.Fprintf(&functionBodyBuf, "\tif src.%s != nil {\n", srcField.Name)
						fmt.Fprintf(&functionBodyBuf, "\t\tnestedVal := %s(ec, *%s)\n", nestedHelperFuncName, srcAccess)
						fmt.Fprintf(&functionBodyBuf, "\t\t%s&nestedVal\n", assignToDst)
						fmt.Fprintf(&functionBodyBuf, "\t} else {\n")
						fmt.Fprintf(&functionBodyBuf, "\t\t%s nil // Source pointer is nil, so destination pointer is nil\n", assignToDst)
						fmt.Fprintf(&functionBodyBuf, "\t}\n")
					} else { // S -> D (non-pointers)
						fmt.Fprintf(&functionBodyBuf, "\t%s%s(ec, %s)\n", assignToDst, nestedHelperFuncName, srcAccess)
					}

				} else { // This is the "else" for "if srcIsStruct && dstIsStruct"
					// Fallback for non-struct or other complex types not yet handled
					// Priority 3: Automatic Conversion (Advanced) - Underlying Type Match
					srcUnderlying := getUnderlyingTypeInfo(srcField.TypeInfo)
					dstUnderlying := getUnderlyingTypeInfo(dstField.TypeInfo)

					// --- DEBUG COMMENTS START ---
					fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_SRC_FIELD: Name=%s, FullName=%s, IsBasic=%t, Kind=%v\n", srcField.TypeInfo.Name, srcField.TypeInfo.FullName, srcField.TypeInfo.IsBasic, srcField.TypeInfo.Kind)
					if srcField.TypeInfo.Underlying != nil {
						fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_SRC_FIELD_UNDERLYING: Name=%s, FullName=%s, IsBasic=%t, Kind=%v\n", srcField.TypeInfo.Underlying.Name, srcField.TypeInfo.Underlying.FullName, srcField.TypeInfo.Underlying.IsBasic, srcField.TypeInfo.Underlying.Kind)
					}
					fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_DST_FIELD: Name=%s, FullName=%s, IsBasic=%t, Kind=%v\n", dstField.TypeInfo.Name, dstField.TypeInfo.FullName, dstField.TypeInfo.IsBasic, dstField.TypeInfo.Kind)
					if dstField.TypeInfo.Underlying != nil {
						fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_DST_FIELD_UNDERLYING: Name=%s, FullName=%s, IsBasic=%t, Kind=%v\n", dstField.TypeInfo.Underlying.Name, dstField.TypeInfo.Underlying.FullName, dstField.TypeInfo.Underlying.IsBasic, dstField.TypeInfo.Underlying.Kind)
					}
					if srcUnderlying != nil {
						fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_SRC_ACTUAL_UNDERLYING: Name=%s, FullName=%s, IsBasic=%t, Kind=%v\n", srcUnderlying.Name, srcUnderlying.FullName, srcUnderlying.IsBasic, srcUnderlying.Kind)
					} else {
						fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_SRC_ACTUAL_UNDERLYING: nil\n")
					}
					if dstUnderlying != nil {
						fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_DST_ACTUAL_UNDERLYING: Name=%s, FullName=%s, IsBasic=%t, Kind=%v\n", dstUnderlying.Name, dstUnderlying.FullName, dstUnderlying.IsBasic, dstUnderlying.Kind)
					} else {
						fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_DST_ACTUAL_UNDERLYING: nil\n")
					}
					// --- DEBUG COMMENTS END ---

					fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_BEFORE_underlyingTypesMatch_check: srcUnderlying is nil = %t, dstUnderlying is nil = %t\n", srcUnderlying == nil, dstUnderlying == nil)
					underlyingTypesMatch := false
					if srcUnderlying != nil && dstUnderlying != nil {
						fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_COND_PreCheck: srcUnderlying.IsBasic=%t, dstUnderlying.IsBasic=%t, srcUnderlying.Name=%s, dstUnderlying.Name=%s, srcUnderlying.FullName=%s, dstUnderlying.FullName=%s\n", srcUnderlying.IsBasic, dstUnderlying.IsBasic, srcUnderlying.Name, dstUnderlying.Name, srcUnderlying.FullName, dstUnderlying.FullName)
						if srcUnderlying.IsBasic && dstUnderlying.IsBasic && srcUnderlying.Name == dstUnderlying.Name {
							underlyingTypesMatch = true
							fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_MATCH_COND: Cond1_BasicByName (%s)\n", srcUnderlying.Name)
						} else if !srcUnderlying.IsBasic && !dstUnderlying.IsBasic && srcUnderlying.FullName == dstUnderlying.FullName {
							underlyingTypesMatch = true
							fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_MATCH_COND: Cond2_NonBasicByFullName (%s)\n", srcUnderlying.FullName)
						} else if srcUnderlying.FullName == dstUnderlying.FullName { // This is a broader fallback
							underlyingTypesMatch = true
							fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_MATCH_COND: Cond3_FallbackByFullName (%s).\n", srcUnderlying.FullName)
						} else {
							fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_MATCH_COND: No match for underlying types.\n")
						}
					} else {
						fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_MATCH_COND: srcUnderlying or dstUnderlying is nil.\n")
					}
					fmt.Fprintf(&functionBodyBuf, "\t// DEBUG_FINAL_underlyingTypesMatch: %t\n", underlyingTypesMatch)


					if underlyingTypesMatch &&
						srcUnderlying.StructInfo == nil && dstUnderlying.StructInfo == nil &&
						!srcUnderlying.IsMap && !dstUnderlying.IsMap &&
						!srcUnderlying.IsSlice && !dstUnderlying.IsSlice {

						dstTypeNameStr := typeNameInSource(dstField.TypeInfo, parsedInfo.PackagePath, imports)
						srcAccessPathBase := "src." + srcField.Name // Base access path

						isSrcUnderlyingEffectivelyPointer := srcField.TypeInfo.IsPointer
						if !isSrcUnderlyingEffectivelyPointer && srcField.TypeInfo.Kind == model.KindNamed && srcField.TypeInfo.Underlying != nil {
							isSrcUnderlyingEffectivelyPointer = srcField.TypeInfo.Underlying.IsPointer
						}

						srcCastOperandForUnderlying := srcAccessPathBase
						if isSrcUnderlyingEffectivelyPointer { // This applies if the source type (or its underlying if named) is a pointer
							srcCastOperandForUnderlying = "*" + srcAccessPathBase
						}

						isActualSrcPointer := srcField.TypeInfo.IsPointer || (srcField.TypeInfo.Kind == model.KindNamed && srcField.TypeInfo.Underlying != nil && srcField.TypeInfo.Underlying.IsPointer)
						isActualDstPointer := dstField.TypeInfo.IsPointer || (dstField.TypeInfo.Kind == model.KindNamed && dstField.TypeInfo.Underlying != nil && dstField.TypeInfo.Underlying.IsPointer)


						if !isActualSrcPointer && !isActualDstPointer {
							fmt.Fprintf(&functionBodyBuf, "\tdst.%s = %s(%s)\n", dstField.Name, dstTypeNameStr, srcCastOperandForUnderlying)
						} else if !isActualSrcPointer && isActualDstPointer {
							fmt.Fprintf(&functionBodyBuf, "\t{\n")
							fmt.Fprintf(&functionBodyBuf, "\t\tconvertedVal := %s(%s)\n", typeNameInSource(dstField.TypeInfo.Elem, parsedInfo.PackagePath, imports), srcCastOperandForUnderlying)
							fmt.Fprintf(&functionBodyBuf, "\t\tdst.%s = &convertedVal\n", dstField.Name)
							fmt.Fprintf(&functionBodyBuf, "\t}\n")
						} else if isActualSrcPointer && !isActualDstPointer {
							if srcField.Tag.Required {
								fmt.Fprintf(&functionBodyBuf, "\tif %s == nil {\n", srcAccessPathBase) // Check original field path for nil
								fmt.Fprintf(&functionBodyBuf, "\t\tec.Addf(\"field '%s' is required but source field %s for underlying cast is nil\")\n", dstField.Name, srcField.Name)
								fmt.Fprintf(&functionBodyBuf, "\t} else {\n")
								fmt.Fprintf(&functionBodyBuf, "\t\tdst.%s = %s(%s)\n", dstField.Name, dstTypeNameStr, srcCastOperandForUnderlying)
								fmt.Fprintf(&functionBodyBuf, "\t}\n")
							} else {
								fmt.Fprintf(&functionBodyBuf, "\tif %s != nil {\n", srcAccessPathBase) // Check original field path for nil
								fmt.Fprintf(&functionBodyBuf, "\t\tdst.%s = %s(%s)\n", dstField.Name, dstTypeNameStr, srcCastOperandForUnderlying)
								fmt.Fprintf(&functionBodyBuf, "\t}\n")
							}
						} else { // isActualSrcPointer && isActualDstPointer
							fmt.Fprintf(&functionBodyBuf, "\tif %s != nil {\n", srcAccessPathBase) // Check original field path for nil
							fmt.Fprintf(&functionBodyBuf, "\t\tconvertedVal := %s(%s)\n", typeNameInSource(dstField.TypeInfo.Elem, parsedInfo.PackagePath, imports), srcCastOperandForUnderlying)
							fmt.Fprintf(&functionBodyBuf, "\t\tdst.%s = &convertedVal\n", dstField.Name)
							fmt.Fprintf(&functionBodyBuf, "\t} else {\n")
							fmt.Fprintf(&functionBodyBuf, "\t\tdst.%s = nil\n", dstField.Name)
							fmt.Fprintf(&functionBodyBuf, "\t}\n")
						// } else { // Should be non-pointer to non-pointer if all other pointer cases are handled
						// 	fmt.Fprintf(&functionBodyBuf, "\tdst.%s = %s(%s)\n", dstField.Name, dstTypeNameStr, srcCastOperandForUnderlying)
						}
					} else {
						fmt.Fprintf(&functionBodyBuf, "\t// TODO: Implement conversion for %s (%s) to %s (%s).\n",
							srcField.Name, srcField.TypeInfo.FullName,
							dstField.Name, dstField.TypeInfo.FullName)
						fmt.Fprintf(&functionBodyBuf, "\tec.Addf(\"type mismatch or complex conversion not yet implemented for field '%s' (%s -> %s)\")\n",
							dstField.Name, srcField.TypeInfo.FullName, dstField.TypeInfo.FullName)
					}
				} // This closes the "else" for "if srcIsStruct && dstIsStruct"
			} // This closes the "else" for "if typesMatchDirectly || basic pointer conversions"
		} // if appliedUsingFunc != "" の else ブロックの閉じ括弧

		fmt.Fprintf(&functionBodyBuf, "\tec.Leave()\n")
		fmt.Fprintf(&functionBodyBuf, "\tif ec.MaxErrorsReached() { return dst } \n\n")
	} // for ループの閉じ括弧

	// --- Write function signature and docstring ---
	// Add docstring for unmapped fields
	if len(unmappedDstFields) > 0 {
		fmt.Fprintf(buf, "// %s converts %s to %s.\n", funcName, srcType.Name, dstType.Name)
		fmt.Fprintf(buf, "// Fields in %s not populated by this conversion:\n", dstType.Name)
		sortedUnmappedFields := make([]string, 0, len(unmappedDstFields))
		for fName := range unmappedDstFields {
			sortedUnmappedFields = append(sortedUnmappedFields, fName)
		}
		sort.Strings(sortedUnmappedFields) // Sort for deterministic output
		for _, fName := range sortedUnmappedFields {
			fmt.Fprintf(buf, "// - %s\n", fName)
		}
	}

	fmt.Fprintf(buf, "func %s(ec *errorCollector, src %s) %s {\n",
		funcName,
		typeNameInSource(srcType, parsedInfo.PackagePath, imports),
		typeNameInSource(dstType, parsedInfo.PackagePath, imports))
	buf.Write(functionBodyBuf.Bytes()) // Write the already generated function body

	fmt.Fprintf(buf, "\treturn dst\n")
	fmt.Fprintf(buf, "}\n\n")
	return nil
}

// typeNameInSource returns the string representation of the type as it should appear in the generated source code.
// It considers if the type is from the package being generated into, or an external package.
// It uses the `imports` map (path -> alias) to determine the correct alias for external packages.
func typeNameInSource(typeInfo *model.TypeInfo, currentPackagePath string, imports map[string]string) string {
	if typeInfo == nil {
		return "interface{}" // Fallback
	}

	var buildName func(ti *model.TypeInfo) string
	buildName = func(ti *model.TypeInfo) string {
		if ti.IsPointer {
			return "*" + buildName(ti.Elem)
		}
		// IsArray logic removed as model.TypeInfo.ArrayLengthExpr was removed.
		// All []T are treated as slices now.
		if ti.IsSlice { // This will now also cover what might have been arrays
			return "[]" + buildName(ti.Elem)
		}
		if ti.IsMap {
			return fmt.Sprintf("map[%s]%s", buildName(ti.Key), buildName(ti.Value))
		}

		// For non-composite types (Ident, Basic, Struct, Named)
		if ti.PackagePath != "" && ti.PackagePath != currentPackagePath && !ti.IsBasic {
			// External package. Find its alias from the `imports` map.
			alias, aliasExists := imports[ti.PackagePath]
			if !aliasExists {
				// This should not happen if addRequiredImport was called for this type.
				// Fallback to using the TypeInfo's PackageName (if set by parser from a selector)
				// or the base name of the package path.
				fmt.Printf("Warning: Package path '%s' for type '%s' not found in required imports map. Using fallback selector.\n", ti.PackagePath, ti.FullName)
				if ti.PackageName != "" { // Parser might have set this from a selector like `pkg.Type`
					alias = ti.PackageName
				} else {
					alias = filepath.Base(ti.PackagePath)
				}
			}
			// If alias is same as base name of path, it means no explicit alias in import stmt.
			// e.g. import "time", used as time.Time. alias="time"
			// e.g. import custom "example.com/custom", used as custom.Type. alias="custom"
			fullNameCandidate := alias + "." + ti.Name
			// Defensive check: if ti.Name somehow already contains the package alias prefix.
			// This might happen if ti.Name was populated with a fully qualified name from another source.
			// e.g. alias="time", ti.Name="time.Time" -> results in "time.time.Time" without this check.
			if strings.HasPrefix(ti.Name, alias+".") && len(ti.Name) > len(alias)+1 {
				// Check to ensure it's not just Name="time" and alias="time" leading to Name being prefix of itself.
				// This implies ti.Name is already correctly qualified for its own package, but used via an alias here.
				// This situation is unusual. Normally ti.Name would be "Time" and alias "time".
				// If ti.Name is "pkg.Type" and alias is "pkg", we want "pkg.Type".
				return ti.Name
			}
			return fullNameCandidate
		}
		return ti.Name // Type is in the current package, or a basic type, or its package path is empty.
	}
	return buildName(typeInfo)
}

// addRequiredImport tracks necessary imports.
// `imports` is a map of import_path -> import_alias.
// It ensures that TypeInfo.PackageName is set to the alias that will be used in the generated code.
func addRequiredImport(typeInfo *model.TypeInfo, currentPackagePath string, imports map[string]string) { // Rewritten
	if typeInfo == nil {
		return
	}

	// Recursively add for elements, keys, values, underlying types
	if typeInfo.IsPointer || typeInfo.IsSlice || typeInfo.IsArray {
		if typeInfo.Elem != nil {
			addRequiredImport(typeInfo.Elem, currentPackagePath, imports)
		}
		return // Pointer/slice/array types themselves don't define a package to import; their elements might.
	}
	if typeInfo.IsMap {
		if typeInfo.Key != nil {
			addRequiredImport(typeInfo.Key, currentPackagePath, imports)
		}
		if typeInfo.Value != nil {
			addRequiredImport(typeInfo.Value, currentPackagePath, imports)
		}
		return // Map types themselves don't define a package; their key/value types might.
	}

	// If it's a named type, process its underlying type first.
	// e.g. for `type MyTime time.Time`, `time.Time` needs `time` import.
	// The named type `MyTime` itself is in `currentPackagePath`.
	if typeInfo.Kind == model.KindNamed && typeInfo.Underlying != nil {
		// The named type A itself is in currentPackagePath (unless it's an alias to external type, handled by PackagePath check below)
		addRequiredImport(typeInfo.Underlying, currentPackagePath, imports)
		// Fall through to check if the named type A itself is from an external package (e.g. alias to external)
	}
	// If it's a struct, its fields' types will be processed when the struct conversion is generated.
	// At that point, addRequiredImport will be called for each field type.

	if typeInfo.PackagePath != "" && typeInfo.PackagePath != currentPackagePath && !typeInfo.IsBasic {
		// External package that needs to be imported.

		// Determine the alias.
		// 1. If an alias is already registered for this path, use it.
		// 2. If TypeInfo.PackageName is set (e.g. from parser seeing 'pkg.Type'), try to use that as alias.
		// 3. Otherwise, use the base name of the package path.
		var chosenAlias string
		if existingAlias, ok := imports[typeInfo.PackagePath]; ok && existingAlias != "" {
			chosenAlias = existingAlias
		} else if typeInfo.PackageName != "" && typeInfo.PackageName != filepath.Base(typeInfo.PackagePath) && typeInfo.PackageName != "main" {
			// If PackageName is set by parser AND it's different from basename (e.g. a true alias like `customtime.Time`)
			// and not 'main' (which cannot be used as an alias typically for external packages)
			chosenAlias = typeInfo.PackageName
		} else {
			chosenAlias = filepath.Base(typeInfo.PackagePath)
		}

		// Check for collision: if this chosenAlias is already used for a *different* path.
		// This simple check doesn't fully resolve complex collisions but warns.
		// A more robust system would generate unique aliases (pkg1, pkg2, etc.)
		for path, aliasInMap := range imports {
			if aliasInMap == chosenAlias && path != typeInfo.PackagePath {
				fmt.Printf("Warning: Import alias '%s' for path '%s' collides with existing import for path '%s'. Manual intervention may be needed or use unique aliases.\n", chosenAlias, typeInfo.PackagePath, path)
				// Potentially re-assign a unique alias here if collision detected: chosenAlias = chosenAlias + "_colliding"
				// For now, we'll proceed, hoping gofmt or user handles if it's a real issue.
				break
			}
		}

		imports[typeInfo.PackagePath] = chosenAlias

		// Crucially, ensure TypeInfo.PackageName reflects the alias to be used in generated code.
		// This helps typeNameInSource use the correct prefix.
		typeInfo.PackageName = chosenAlias
	}
}

func lowercaseFirstLetter(s string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.ToLower(s[0:1]) + s[1:]
}

// getUnderlyingTypeInfo recursively finds the base type information,
// stripping away pointers and named type aliases until a non-named, non-pointer type is found,
// or a named type that is a struct/map/slice itself (not an alias to one).
// It returns the TypeInfo of this fundamental underlying type.
func getUnderlyingTypeInfo(ti *model.TypeInfo) *model.TypeInfo {
	if ti == nil {
		return nil
	}
	current := ti
	for {
		if current.IsPointer {
			if current.Elem == nil {
				return nil // Should not happen
			}
			current = current.Elem
			continue
		}
		// If it's a named type and has a different underlying type, continue unwrapping.
		if current.Kind == model.KindNamed && current.Underlying != nil && current.Underlying.FullName != current.FullName {
			current = current.Underlying
			continue
		}
		// No more pointers, and not a named type that can be further unwrapped.
		return current
	}
}

// errorCollectorTemplate (content is the same as before)
const errorCollectorTemplate = `
// errorCollector collects errors with path tracking.
type errorCollector struct {
	maxErrors int
	errors    []error
	pathStack []string
}
func newErrorCollector(maxErrors int) *errorCollector {
	return &errorCollector{
		maxErrors: maxErrors,
		errors:    make([]error, 0),
		pathStack: make([]string, 0),
	}
}
func (ec *errorCollector) Add(reason string) bool {
	if ec.maxErrors > 0 && len(ec.errors) >= ec.maxErrors {
		return true
	}
	fullPath := strings.Join(ec.pathStack, "")
	err := fmt.Errorf("%s: %s", fullPath, reason)
	ec.errors = append(ec.errors, err)
	return ec.maxErrors > 0 && len(ec.errors) >= ec.maxErrors
}
func (ec *errorCollector) Addf(format string, args ...interface{}) bool {
	if ec.maxErrors > 0 && len(ec.errors) >= ec.maxErrors {
		return true
	}
	return ec.Add(fmt.Sprintf(format, args...))
}
func (ec *errorCollector) Enter(segment string) {
	// separator variable was unused, logic is directly handled below.
	if strings.HasPrefix(segment, "[") && strings.HasSuffix(segment, "]") { // Array/slice index
		ec.pathStack = append(ec.pathStack, segment)
	} else { // Field name
		if len(ec.pathStack) == 0 {
			ec.pathStack = append(ec.pathStack, segment)
		} else {
			ec.pathStack = append(ec.pathStack, "."+segment)
		}
	}
}
func (ec *errorCollector) Leave() {
	if len(ec.pathStack) > 0 {
		ec.pathStack = ec.pathStack[:len(ec.pathStack)-1]
	}
}
func (ec *errorCollector) Errors() []error {
	return ec.errors
}
func (ec *errorCollector) HasErrors() bool {
	return len(ec.errors) > 0
}
func (ec *errorCollector) MaxErrorsReached() bool {
	return ec.maxErrors > 0 && len(ec.errors) >= ec.maxErrors
}
`
