package generator

import (
	"bytes"
	"fmt"
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
	if len(parsedInfo.ConversionPairs) == 0 {
		fmt.Println("No conversion pairs defined. Nothing to generate.")
		return nil
	}

	// Determine package name and import path for the generated code.
	// The generated code will be in a 'gen' sub-package of the input package.
	genPackageName := "gen" // Or derive from outputDir if it's not always "gen"
	genPackageImportPath := parsedInfo.PackagePath + "/" + genPackageName
	// Ensure outputDir corresponds to this, or adjust. For now, assume outputDir is where 'gen' will be created.
	// The actual output file name will be based on the original package name, but inside 'gen/'.
	// E.g. if original is 'complex', output is 'gen/complex_gen.go', package inside is 'gen'.

	outputFileName := fmt.Sprintf("%s%s", parsedInfo.PackageName, fileSuffix) // e.g. complex_gen.go
	// outputFilePath should be outputDir/genPackageName/outputFileName if outputDir is models' parent
	// But current setup: outputDir is 'testdata/complex/gen', so file is just outputDir/outputFileName
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
	generatedFunctionBodies := make(map[string]string) // funcName -> funcBody
	pairsToProcess := make([]model.ConversionPair, 0, len(parsedInfo.ConversionPairs))
	pairsToProcess = append(pairsToProcess, parsedInfo.ConversionPairs...)

	// Keep track of generated helper functions to avoid duplicates and simple recursion
	// Key: srcType.FullName + "->" + dstType.FullName
	processedHelpers := make(map[string]bool)

	for len(pairsToProcess) > 0 {
		pair := pairsToProcess[0]
		pairsToProcess = pairsToProcess[1:]

		helperKey := pair.SrcTypeInfo.FullName + "->" + pair.DstTypeInfo.FullName
		if processedHelpers[helperKey] {
			continue
		}

		// Determine helper function name
		// Ensure SrcTypeInfo and DstTypeInfo are non-nil before accessing Name
		srcName := "unknownSrc"
		if pair.SrcTypeInfo != nil {
			srcName = pair.SrcTypeInfo.Name
		}
		dstName := "unknownDst"
		if pair.DstTypeInfo != nil {
			dstName = pair.DstTypeInfo.Name
		}
		coreHelperFuncName := fmt.Sprintf("%sTo%s", lowercaseFirstLetter(srcName), Capitalize(dstName))


		// Check if this exact helper func name is already generated (e.g. from a different pair that resolved to same types)
		if _, exists := generatedFunctionBodies[coreHelperFuncName]; exists {
			processedHelpers[helperKey] = true // Mark as processed even if body generation is skipped
			continue
		}

		var helperFuncBuf bytes.Buffer
		// Pass ctx to generateHelperFunction if it needs to be available for using functions
		// For now, top-level function has ctx, and generateHelperFunction will decide if using funcs need it.
		newPairs, err := generateHelperFunction(&helperFuncBuf, coreHelperFuncName, pair.SrcTypeInfo, pair.DstTypeInfo, parsedInfo, requiredImports, processedHelpers, "ctx") // Pass "ctx" as contextParamName
		if err != nil {
			return fmt.Errorf("failed to generate helper function %s for %s to %s: %w", coreHelperFuncName, pair.SrcTypeInfo.FullName, pair.DstTypeInfo.FullName, err)
		}
		generatedFunctionBodies[coreHelperFuncName] = helperFuncBuf.String()
		processedHelpers[helperKey] = true

		// Add new pairs to the queue
		for _, newPair := range newPairs {
			newHelperKey := newPair.SrcTypeInfo.FullName + "->" + newPair.DstTypeInfo.FullName
			if !processedHelpers[newHelperKey] {
				alreadyInQueue := false
				for _, pInQueue := range pairsToProcess {
					if pInQueue.SrcTypeInfo.FullName == newPair.SrcTypeInfo.FullName && pInQueue.DstTypeInfo.FullName == newPair.DstTypeInfo.FullName {
						alreadyInQueue = true
						break
					}
				}
				if !alreadyInQueue {
					pairsToProcess = append(pairsToProcess, newPair)
				}
			}
		}
	}

	// Then, generate top-level functions for the initial pairs
	for _, pair := range parsedInfo.ConversionPairs {
		// Ensure SrcTypeInfo and DstTypeInfo are non-nil before accessing Name
		srcName := "unknownSrc"
		if pair.SrcTypeInfo != nil {
			srcName = pair.SrcTypeInfo.Name
		}
		dstName := "unknownDst"
		if pair.DstTypeInfo != nil {
			dstName = pair.DstTypeInfo.Name
		}

		topLevelFuncName := fmt.Sprintf("Convert%sTo%s", Capitalize(srcName), Capitalize(dstName))
		// Check if a top-level function with this name (or for this pair) was already made.
		// This check might be redundant if top-level funcs are uniquely named by initial pairs.
		if _, exists := generatedFunctionBodies[topLevelFuncName]; exists {
			continue
		}

		var funcBuf bytes.Buffer
		// The coreHelperFuncName must match the one generated above for this pair.
		coreHelperFuncName := fmt.Sprintf("%sTo%s", lowercaseFirstLetter(srcName), Capitalize(dstName))

		// For top-level functions, currentPackagePath for typeNameInSource should be genPackageImportPath
		addRequiredImport(pair.SrcTypeInfo, genPackageImportPath, requiredImports)
		addRequiredImport(pair.DstTypeInfo, genPackageImportPath, requiredImports)

		fmt.Fprintf(&funcBuf, "func %s(ctx context.Context, src %s) (%s, error) {\n",
			topLevelFuncName,
			typeNameInSource(pair.SrcTypeInfo, genPackageImportPath, requiredImports),
			typeNameInSource(pair.DstTypeInfo, genPackageImportPath, requiredImports))
		// The call to coreHelperFuncName needs to use the context from the top-level func's signature
		fmt.Fprintf(&funcBuf, "\tec := newErrorCollector(%d)\n", pair.MaxErrors)
		fmt.Fprintf(&funcBuf, "\tdst := %s(ctx, ec, src)\n", coreHelperFuncName) // Pass ctx
		fmt.Fprintf(&funcBuf, "\tif ec.HasErrors() {\n")
		if pair.DstTypeInfo != nil && pair.DstTypeInfo.IsPointer { // If dest is a pointer, return nil on error.
		    fmt.Fprintf(&funcBuf, "\t\treturn nil, errors.Join(ec.Errors()...)\n")
		} else {
		    fmt.Fprintf(&funcBuf, "\t\treturn dst, errors.Join(ec.Errors()...)\n")
		}
		fmt.Fprintf(&funcBuf, "\t}\n")
		fmt.Fprintf(&funcBuf, "\treturn dst, nil\n")
		fmt.Fprintf(&funcBuf, "}\n\n")

		generatedFunctionBodies[topLevelFuncName] = funcBuf.String()
	}

	// --- Assemble Final Code ---
	var finalCode bytes.Buffer
	fmt.Fprintf(&finalCode, "// Code generated by convert2 tool. DO NOT EDIT.\n")
	// TODO: Consider using golang.org/x/tools/imports.Process for final import list cleanup.
	fmt.Fprintf(&finalCode, "package %s\n\n", genPackageName) // Use genPackageName

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
// It returns a list of new ConversionPairs discovered (e.g., for nested structs/slices) that need their own helper functions.
func generateHelperFunction(
	buf *bytes.Buffer,
	funcName string,
	srcType, dstType *model.TypeInfo,
	parsedInfo *model.ParsedInfo,
	imports map[string]string,
	processedHelpers map[string]bool, // Used to check if a sub-helper is already being handled
	contextParamName string, // Name of the context.Context variable available in the caller's scope
) ([]model.ConversionPair, error) {
	var newPairs []model.ConversionPair
	// --- Entire body of generateHelperFunction commented out for debug ---
	/*
	// Handle cases where one or both types are not structs first (e.g. named basic types, aliases to non-structs)
	// This includes direct assignment, underlying type casts, or global 'using' rules for non-structs.
	if srcType.StructInfo == nil || dstType.StructInfo == nil {
		currentGenPkgPath := parsedInfo.PackagePath + "/gen" // Path of the package being generated

		addRequiredImport(srcType, currentGenPkgPath, imports)
		addRequiredImport(dstType, currentGenPkgPath, imports)

		// Determine if the helper for non-structs needs context (if a global 'using' func expects it)
		var appliedGlobalUsingFunc string
		var globalUsingNeedsCtx bool
		for _, rule := range parsedInfo.GlobalRules {
			if rule.UsingFunc != "" && rule.SrcTypeInfo != nil && rule.DstTypeInfo != nil &&
				rule.SrcTypeInfo.FullName == srcType.FullName &&
				rule.DstTypeInfo.FullName == dstType.FullName {
				appliedGlobalUsingFunc = rule.UsingFunc
				if strings.HasSuffix(appliedGlobalUsingFunc, "Ctx") { // Convention for context
					globalUsingNeedsCtx = true
				}
				break
			}
		}

		funcSigParams := ""
		if globalUsingNeedsCtx {
			funcSigParams = fmt.Sprintf("%s context.Context, ec *errorCollector, src %s", contextParamName, typeNameInSource(srcType, currentGenPkgPath, imports))
			addRequiredImport(&model.TypeInfo{PackagePath: "context"}, currentGenPkgPath, imports) // ensure context is imported
		} else {
			funcSigParams = fmt.Sprintf("ec *errorCollector, src %s", typeNameInSource(srcType, currentGenPkgPath, imports))
		}


		fmt.Fprintf(buf, "// Helper function %s for types %s -> %s (non-struct or one side is non-struct)\n", funcName, srcType.FullName, dstType.FullName)
		fmt.Fprintf(buf, "func %s(%s) %s {\n", // Pass contextParamName if needed
			funcName,
			funcSigParams,
			typeNameInSource(dstType, currentGenPkgPath, imports))

		// The variables appliedGlobalUsingFunc and globalUsingNeedsCtx were already determined above
		// when deciding the function signature. We just need to use them here.

		if appliedGlobalUsingFunc != "" {
			fmt.Fprintf(buf, "\t// Applying global rule for non-struct types: using %s\n", appliedGlobalUsingFunc)
			// TODO: Handle import for appliedGlobalUsingFunc if it's from another package.
			// This requires resolving the function's package, similar to field-level 'using'.
			if globalUsingNeedsCtx {
				fmt.Fprintf(buf, "\treturn %s(%s, ec, src)\n", appliedGlobalUsingFunc, contextParamName)
			} else {
				fmt.Fprintf(buf, "\treturn %s(ec, src)\n", appliedGlobalUsingFunc)
			}
		} else if srcType.FullName == dstType.FullName { // Identical types
			fmt.Fprintf(buf, "\treturn src // Identical non-struct types\n")
		} else if dstType.Underlying != nil && srcType.FullName == dstType.Underlying.FullName { // T -> MyT (where MyT is type T string)
			fmt.Fprintf(buf, "\treturn %s(src) // Cast to named type from its underlying type\n", typeNameInSource(dstType, currentGenPkgPath, imports))
		} else if srcType.Underlying != nil && srcType.Underlying.FullName == dstType.FullName { // MyT -> T
			fmt.Fprintf(buf, "\treturn %s(src) // Cast from named type to its underlying type\n", typeNameInSource(dstType, currentGenPkgPath, imports))
		} else if srcType.Underlying != nil && dstType.Underlying != nil && srcType.Underlying.FullName == dstType.Underlying.FullName { // MyT1 -> MyT2 (both are type T string)
			fmt.Fprintf(buf, "\treturn %s(src) // Cast between named types with same underlying type\n", typeNameInSource(dstType, currentGenPkgPath, imports))
		} else {
			// Default for non-structs if no rule and no direct/underlying match: error or zero value
			fmt.Fprintf(buf, "\tvar dst %s\n", typeNameInSource(dstType, currentGenPkgPath, imports))
			fmt.Fprintf(buf, "\tec.Addf(\"conversion from non-struct %s to non-struct %s not implemented and no global rule applies\")\n", srcType.FullName, dstType.FullName)
			fmt.Fprintf(buf, "\treturn dst\n")
		}
		fmt.Fprintf(buf, "}\n\n")
		return newPairs, nil
	} else {
		// Both srcType and dstType have StructInfo.
		currentGenPkgPath := parsedInfo.PackagePath + "/gen" // Path of the package being generated
		srcStruct := srcType.StructInfo
		dstStruct := dstType.StructInfo

	addRequiredImport(srcType, currentGenPkgPath, imports)
	addRequiredImport(dstType, currentGenPkgPath, imports)

	// Determine if this struct helper function itself needs context in its signature
	helperAcceptsContext := true
	funcSigParams := ""
	if helperAcceptsContext {
		funcSigParams = fmt.Sprintf("%s context.Context, ec *errorCollector, src %s", contextParamName, typeNameInSource(srcType, currentGenPkgPath, imports))
		addRequiredImport(&model.TypeInfo{PackagePath: "context"}, currentGenPkgPath, imports)
	} else {
		funcSigParams = fmt.Sprintf("ec *errorCollector, src %s", typeNameInSource(srcType, currentGenPkgPath, imports))
	}


	fmt.Fprintf(buf, "func %s(%s) %s {\n",
		funcName,
		funcSigParams,
		typeNameInSource(dstType, currentGenPkgPath, imports))
	fmt.Fprintf(buf, "\tdst := %s{}\n", typeNameInSource(dstType, currentGenPkgPath, imports))
	fmt.Fprintf(buf, "\tif ec.MaxErrorsReached() { return dst } \n\n")

	for _, srcField := range srcStruct.Fields {
		if srcField.Tag.DstFieldName == "-" {
			fmt.Fprintf(buf, "\t// Source field %s.%s is skipped due to tag '-'.\n", srcStruct.Name, srcField.Name)
			continue
		}

		dstFieldName := srcField.Tag.DstFieldName
		if dstFieldName == "" {
			dstFieldName = srcField.Name
		}

		var dstField *model.FieldInfo
		for i := range dstStruct.Fields {
			if dstStruct.Fields[i].Name == dstFieldName {
				dstField = &dstStruct.Fields[i]
				break
			}
		}

		if dstField == nil {
			if srcField.Tag.DstFieldName != "" {
				fmt.Fprintf(buf, "\t// Warning: Destination field '%s' specified for source field '%s.%s' not found in struct '%s'.\n", srcField.Tag.DstFieldName, srcStruct.Name, srcField.Name, dstStruct.Name)
			}
			continue
		}

		fmt.Fprintf(buf, "\t// Mapping field %s.%s (%s) to %s.%s (%s)\n",
			srcStruct.Name, srcField.Name, srcField.TypeInfo.FullName,
			dstStruct.Name, dstField.Name, dstField.TypeInfo.FullName)

		srcElemFullName := "nil"
		if srcField.TypeInfo.Elem != nil {
			srcElemFullName = srcField.TypeInfo.Elem.FullName
		}
		dstElemFullName := "nil"
		if dstField.TypeInfo.Elem != nil {
			dstElemFullName = dstField.TypeInfo.Elem.FullName
		}
		fmt.Fprintf(buf, "\t// Src: Ptr=%t, ElemFull=%s | Dst: Ptr=%t, ElemFull=%s\n",
			srcField.TypeInfo.IsPointer, srcElemFullName,
			dstField.TypeInfo.IsPointer, dstElemFullName)
		fmt.Fprintf(buf, "\tec.Enter(%q)\n", dstField.Name)

		addRequiredImport(srcField.TypeInfo, currentGenPkgPath, imports)
		addRequiredImport(dstField.TypeInfo, currentGenPkgPath, imports)

		var appliedUsingFunc string
		var usingFuncIsGlobal bool
		var usingFuncPackagePath string // Path of the package where the using/validator func is defined

		// Priority 1: Field Tag `using=<funcName>`
		if srcField.Tag.UsingFunc != "" {
			appliedUsingFunc = srcField.Tag.UsingFunc
			usingFuncIsGlobal = false
			// If func is "pkg.Func", its package path needs to be resolved.
			// For now, assume if it has a selector, its package is already imported via type usage.
			// If it's just "Func", assume it's in the models' package.
			if !strings.Contains(appliedUsingFunc, ".") {
				usingFuncPackagePath = parsedInfo.PackagePath // Models' package path
			} else {
				// TODO: Resolve package path for "pkg.Func" from field tag if needed for explicit import.
				// This is complex; typeNameInSource handles type imports, not arbitrary functions.
			}
		} else {
			// Priority 2: Global Rule `convert:rule "<SrcT>" -> "<DstT>", using=<funcName>`
			for _, rule := range parsedInfo.GlobalRules {
				if rule.UsingFunc != "" && rule.SrcTypeInfo != nil && rule.DstTypeInfo != nil &&
					rule.SrcTypeInfo.FullName == srcField.TypeInfo.FullName &&
					rule.DstTypeInfo.FullName == dstField.TypeInfo.FullName {
					appliedUsingFunc = rule.UsingFunc
					usingFuncIsGlobal = true
					// Global rule functions are assumed to be in the models' package unless specified with a prefix.
					if !strings.Contains(appliedUsingFunc, ".") {
						usingFuncPackagePath = parsedInfo.PackagePath // Models' package path
					} else {
						// TODO: Resolve for "pkg.Func" in global rule.
					}
					break
				}
			}
		}

		if appliedUsingFunc != "" {
			funcCallName := appliedUsingFunc
			// If usingFuncPackagePath is set (meaning function is from the models' package and had no prefix)
			// and the generated code is in a different package (gen), then prefix with models' package alias.
			modelsPackageAlias := parsedInfo.PackageName // This is the alias for the models package.
			if usingFuncPackagePath == parsedInfo.PackagePath && // Func is in models package
				currentGenPkgPath != parsedInfo.PackagePath && // And we are generating into a different package (e.g., "gen")
				!strings.Contains(funcCallName, ".") { // And the func name is not already qualified
				funcCallName = modelsPackageAlias + "." + funcCallName
				// Ensure the models package itself is imported with this alias.
				// This should happen naturally if any type from models package is used.
				// Explicitly add here if it might not be (e.g. using func converts basic types).
				imports[parsedInfo.PackagePath] = modelsPackageAlias
			} else if strings.Contains(funcCallName, ".") {
				// If funcCallName is already "pkg.Func", ensure "pkg" is imported.
				// addRequiredImport needs to handle this. For now, assume typeNameInSource covers it if types from "pkg" are used.
				// This is a simplification. A robust solution would parse "pkg" and find its import path.
				// parts := strings.Split(funcCallName, ".") // pkgAlias derived from this was unused for now
				// pkgAlias := parts[0]
				// We need to find the actual import path for pkgAlias.
				// This information is in parsedInfo.FileImports, but that's per-file.
				// For now, we assume that if a type from that pkg was used, its import is there.
				// This is a known limitation for functions from packages not otherwise imported by types.
			}


			if parts := strings.Split(funcCallName, "."); len(parts) == 2 {
				// This block is for when appliedUsingFunc is already "pkg.Func"
				// It was intended for resolving imports, but actual prefixing for global rules was missing.
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
				fmt.Fprintf(buf, "\t// Applying global rule: %s -> %s using %s\n", srcField.TypeInfo.FullName, dstField.TypeInfo.FullName, appliedUsingFunc)
			} else {
				fmt.Fprintf(buf, "\t// Applying field tag: using %s\n", appliedUsingFunc)
			}

			if strings.HasSuffix(appliedUsingFunc, "Ctx") {
				// TODO: Ensure 'context' package is imported if any Ctx func is used.
				// addRequiredImport for "context" happens at the start of GenerateConversionCode.
				// Also, helper function signature now includes contextParamName.
				fmt.Fprintf(buf, "\tdst.%s = %s(%s, ec, src.%s)\n", dstField.Name, funcCallName, contextParamName, srcField.Name)
			} else {
				fmt.Fprintf(buf, "\tdst.%s = %s(ec, src.%s)\n", dstField.Name, funcCallName, srcField.Name)
			}
			// If a 'using' function was applied, skip other conversion logic for this field.
			fmt.Fprintf(buf, "\tec.Leave()\n")
			fmt.Fprintf(buf, "\tif ec.MaxErrorsReached() { return dst } \n\n")
			continue // Move to the next field
		}

		// No 'using' function, proceed with automatic conversion logic
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
				fmt.Fprintf(buf, "\tdst.%s = src.%s\n", dstField.Name, srcField.Name)
			} else if !srcIsPtr && dstIsPtr && elementsMatch { // Case: T -> *T
				// Ensure that dstField.TypeInfo.Elem.FullName matches srcField.TypeInfo.FullName
				fmt.Fprintf(buf, "\t{\n")
				fmt.Fprintf(buf, "\t\tsrcVal := src.%s\n", srcField.Name)
				fmt.Fprintf(buf, "\t\tdst.%s = &srcVal\n", dstField.Name)
				fmt.Fprintf(buf, "\t}\n")
			} else if srcIsPtr && !dstIsPtr && elementsMatch { // Case: *T -> T
				if srcField.Tag.Required {
					fmt.Fprintf(buf, "\tif src.%s == nil {\n", srcField.Name)
					fmt.Fprintf(buf, "\t\tec.Addf(\"field %q is required but source field %s is nil\")\n", dstField.Name, srcField.Name)
					fmt.Fprintf(buf, "\t} else {\n")
					fmt.Fprintf(buf, "\t\tdst.%s = *src.%s\n", dstField.Name, srcField.Name)
					fmt.Fprintf(buf, "\t}\n")
				} else {
					fmt.Fprintf(buf, "\tif src.%s != nil {\n", srcField.Name)
					fmt.Fprintf(buf, "\t\tdst.%s = *src.%s\n", dstField.Name, srcField.Name)
					fmt.Fprintf(buf, "\t}\n")
				}
			} else if srcField.TypeInfo.IsSlice && dstField.TypeInfo.IsSlice { // Case: []T1 -> []T2
				newPairsForSlice, err := generateSliceConversion(buf, "src."+srcField.Name, "dst."+dstField.Name, srcField.TypeInfo, dstField.TypeInfo, parsedInfo, imports /*, processedHelpers, srcField.Tag */)
				if err != nil {
					return newPairs, fmt.Errorf("generating slice conversion for field %s: %w", srcField.Name, err)
				}
				newPairs = append(newPairs, newPairsForSlice...)
			} else if srcField.TypeInfo.Kind == model.KindStruct && dstField.TypeInfo.Kind == model.KindStruct && (srcField.TypeInfo.StructInfo != nil && dstField.TypeInfo.StructInfo != nil) { // Case: StructS -> StructD (recursive)
				// This handles direct struct fields. Pointer to struct fields will be handled by pointer logic + this.
				nestedSrcType := srcField.TypeInfo
				nestedDstType := dstField.TypeInfo

				// Dereference if they are pointers to structs for the recursive call helper name
				if nestedSrcType.IsPointer && nestedSrcType.Elem != nil && nestedSrcType.Elem.Kind == model.KindStruct {
					nestedSrcType = nestedSrcType.Elem
				}
				if nestedDstType.IsPointer && nestedDstType.Elem != nil && nestedDstType.Elem.Kind == model.KindStruct {
					nestedDstType = nestedDstType.Elem
				}

				// Ensure that the recursive call is for actual struct types, not just named types that happen to be structs.
				// The StructInfo check above should cover this.

				helperName := fmt.Sprintf("%sTo%s", lowercaseFirstLetter(nestedSrcType.Name), Capitalize(nestedDstType.Name))

				// Add this nested pair to newPairs IF it's not already processed or being processed.
				helperKey := nestedSrcType.FullName + "->" + nestedDstType.FullName
				if !processedHelpers[helperKey] {
					// Check if it's already in the main processing queue (pairsToProcess in GenerateConversionCode)
					// This check is imperfect here as we don't have direct access to pairsToProcess.
					// The processedHelpers map is the primary guard.
					newPairs = append(newPairs, model.ConversionPair{SrcTypeInfo: nestedSrcType, DstTypeInfo: nestedDstType, MaxErrors: parsedInfo.ConversionPairs[0].MaxErrors /* Inherit max errors? */})
					// Mark as processed here to prevent adding multiple times from different fields
					// The main loop in GenerateConversionCode will also mark it.
					// processedHelpers[helperKey] = true // This might be too early, let the main loop handle it.
				}

				// Generate code for the call
				srcFieldAccessor := "src." + srcField.Name
				assignToDst := "dst." + dstField.Name + " = "

				// Handle pointer combinations for the call itself
				// S -> D : dst.F = helper(ec, src.F)
				// S -> *D : val := helper(ec, src.F); dst.F = &val
				// *S -> D : if src.F != nil { dst.F = helper(ec, *src.F) } (else if required error)
				// *S -> *D : if src.F != nil { val := helper(ec, *src.F); dst.F = &val }

				if !srcField.TypeInfo.IsPointer && !dstField.TypeInfo.IsPointer { // S -> D
					fmt.Fprintf(buf, "\t%s%s(ec, %s)\n", assignToDst, helperName, srcFieldAccessor)
				} else if !srcField.TypeInfo.IsPointer && dstField.TypeInfo.IsPointer { // S -> *D
					fmt.Fprintf(buf, "\t{\n")
					fmt.Fprintf(buf, "\t\tconverted := %s(ec, %s)\n", helperName, srcFieldAccessor)
					fmt.Fprintf(buf, "\t\t%s&converted\n", assignToDst)
					fmt.Fprintf(buf, "\t}\n")
				} else if srcField.TypeInfo.IsPointer && !dstField.TypeInfo.IsPointer { // *S -> D
					fmt.Fprintf(buf, "\tif %s != nil {\n", srcFieldAccessor)
					fmt.Fprintf(buf, "\t\t%s%s(ec, *%s)\n", assignToDst, helperName, srcFieldAccessor)
					fmt.Fprintf(buf, "\t} else {\n")
					if srcField.Tag.Required {
						fmt.Fprintf(buf, "\t\tec.Addf(\"field %q is required but source field %s is nil\")\n", dstField.Name, srcField.Name)
					}
					// If not required, dst field remains zero value for D.
					fmt.Fprintf(buf, "\t}\n")
				} else { // *S -> *D
					fmt.Fprintf(buf, "\tif %s != nil {\n", srcFieldAccessor)
					fmt.Fprintf(buf, "\t\tconverted := %s(ec, *%s)\n", helperName, srcFieldAccessor)
					fmt.Fprintf(buf, "\t\t%s&converted\n", assignToDst)
					fmt.Fprintf(buf, "\t} else {\n")
					// dst.F remains nil if src.F is nil, unless required.
					if srcField.Tag.Required {
						fmt.Fprintf(buf, "\t\tec.Addf(\"field %q is required but source field %s is nil\")\n", dstField.Name, srcField.Name)
					}
					fmt.Fprintf(buf, "\t}\n")
				}
			// Priority 3.5: Underlying Type Matching (for fields that are not pointers, slices, or structs handled above)
			// This handles cases like: type MyInt int; f1 int -> f2 MyInt  OR  f1 MyInt -> f2 int
			} else if srcField.TypeInfo.Underlying != nil && dstField.TypeInfo.Underlying != nil &&
				srcField.TypeInfo.Underlying.FullName == dstField.TypeInfo.Underlying.FullName &&
				srcField.TypeInfo.Underlying.IsBasic && dstField.TypeInfo.Underlying.IsBasic { // Both are named types of the same underlying basic type
				fmt.Fprintf(buf, "\tdst.%s = %s(src.%s)\n", dstField.Name, typeNameInSource(dstField.TypeInfo, currentGenPkgPath, imports), srcField.Name)
			} else if dstField.TypeInfo.Underlying != nil && dstField.TypeInfo.Underlying.FullName == srcField.TypeInfo.FullName { // T -> MyT (where MyT's underlying is T)
				fmt.Fprintf(buf, "\tdst.%s = %s(src.%s)\n", dstField.Name, typeNameInSource(dstField.TypeInfo, currentGenPkgPath, imports), srcField.Name)
			} else if srcField.TypeInfo.Underlying != nil && srcField.TypeInfo.Underlying.FullName == dstField.TypeInfo.FullName { // MyT -> T (where MyT's underlying is T)
				fmt.Fprintf(buf, "\tdst.%s = src.%s\n", dstField.Name, srcField.Name) // Direct assignment if target is the base type
			} else {
				// Fallback for unhandled types
				fmt.Fprintf(buf, "\t// No specific conversion rule or type match found for field %s (%s) to %s (%s).\n",
					srcField.Name, srcField.TypeInfo.FullName,
					dstField.Name, dstField.TypeInfo.FullName)
				fmt.Fprintf(buf, "\tec.Addf(\"type mismatch or complex conversion not implemented for field '%s' (%s -> %s)\")\n",
					dstField.Name, srcField.TypeInfo.FullName, dstField.TypeInfo.FullName)
			}
		}

		fmt.Fprintf(buf, "\tec.Leave()\n")
		fmt.Fprintf(buf, "\tif ec.MaxErrorsReached() { return dst } \n\n")
	} // End of for _, srcField := range srcStruct.Fields

	// After all fields are populated, check for and apply validator functions for the DstType
	for _, rule := range parsedInfo.GlobalRules {
		if rule.ValidatorFunc != "" && rule.DstTypeInfo != nil && rule.DstTypeInfo.FullName == dstType.FullName {
			validatorCallName := rule.ValidatorFunc
			modelsPackageAlias := parsedInfo.PackageName
			if !strings.Contains(validatorCallName, ".") && currentGenPkgPath != parsedInfo.PackagePath {
				validatorCallName = modelsPackageAlias + "." + validatorCallName
				imports[parsedInfo.PackagePath] = modelsPackageAlias
			}
			// TODO: Handle qualified validator funcs like "otherpkg.MyValidator" for imports.

			fmt.Fprintf(buf, "\t// Applying global validator rule: %s for type %s\n", rule.ValidatorFunc, dstType.FullName)
			fmt.Fprintf(buf, "\t%s(ec, &dst)\n", validatorCallName) // Pass address of dst
			fmt.Fprintf(buf, "\tif ec.MaxErrorsReached() { return dst } \n")
			break
		}
	}

	fmt.Fprintf(buf, "\treturn dst\n") // This is the return for the generated helper function
	fmt.Fprintf(buf, "}\n\n") // This closes the generated helper function body
	} // End of else block for struct-to-struct conversion
	return newPairs, nil // This is the return for Go function generateHelperFunction
} // End of generateHelperFunction


// generateSliceConversion generates code for converting a slice field.
// srcFieldPath is like "src.MySliceField", dstFieldPath is like "dst.ConvertedSliceField".
func generateSliceConversion(
	buf *bytes.Buffer,
	srcFieldPath, dstFieldPath string,
	srcSliceType, dstSliceType *model.TypeInfo,
	parsedInfo *model.ParsedInfo,
	imports map[string]string,
	// processedHelpers map[string]bool, // Temporarily removed
	// fieldTag model.ConvertTag // Temporarily removed
) ([]model.ConversionPair, error) {
	var newPairs []model.ConversionPair
	// --- Entire body of generateSliceConversion commented out for debug ---
	/*
	currentGenPkgPath := parsedInfo.PackagePath + "/gen" // Path of the package being generated
	var fieldTag model.ConvertTag // Dummy to compile, logic using it is effectively disabled

	srcElemType := srcSliceType.Elem
	dstElemType := dstSliceType.Elem

	if srcElemType == nil || dstElemType == nil {
		return newPairs, fmt.Errorf("slice %s or %s has nil Elem type", srcSliceType.FullName, dstSliceType.FullName)
	}

	addRequiredImport(srcElemType, currentGenPkgPath, imports)
	addRequiredImport(dstElemType, currentGenPkgPath, imports)

	fmt.Fprintf(buf, "\tif %s == nil {\n", srcFieldPath)
	// TODO: Restore fieldTag.Required check
	// if fieldTag.Required {
	// 	fmt.Fprintf(buf, "\t\tec.Addf(\"slice field %q is required but source is nil\") \n", strings.Split(dstFieldPath, ".")[1])
	// } else {
	fmt.Fprintf(buf, "\t\t%s = nil\n", dstFieldPath) // Simplified: always allow nil for now
	// }
	}
	fmt.Fprintf(buf, "\t} else {\n")
	fmt.Fprintf(buf, "\t\t%s = make(%s, len(%s))\n", dstFieldPath, typeNameInSource(dstSliceType, currentGenPkgPath, imports), srcFieldPath)
	fmt.Fprintf(buf, "\t\tfor i, srcItem := range %s {\n", srcFieldPath)
	fmt.Fprintf(buf, "\t\t\tec.Enter(fmt.Sprintf(\"[%d]\", i))\n")

	srcItemName := "srcItem"
	// dstItemName := "convertedItem" // Unused, direct assignment is used.

	// How to convert elements:
	// 1. Identical types: direct assignment
	// 2. Structs requiring conversion: call helper
	// 3. Pointer logic for elements

	// Helper function for element conversion (if needed)
	var elemHelperName string
	// needsElemHelper := false // Unused, condition checked directly.

	if srcElemType.FullName == dstElemType.FullName {
		// Direct assignment, no helper needed for element itself
		fmt.Fprintf(buf, "\t\t\t%s[i] = %s\n", dstFieldPath, srcItemName)
	} else if srcElemType.Kind == model.KindStruct && dstElemType.Kind == model.KindStruct && srcElemType.StructInfo != nil && dstElemType.StructInfo != nil {
		// needsElemHelper = true // Condition already met
		elemHelperName = fmt.Sprintf("%sTo%s", lowercaseFirstLetter(srcElemType.Name), Capitalize(dstElemType.Name))
		helperKey := srcElemType.FullName + "->" + dstElemType.FullName
		// TODO: Restore processedHelpers check
		// if !processedHelpers[helperKey] {
		newPairs = append(newPairs, model.ConversionPair{SrcTypeInfo: srcElemType, DstTypeInfo: dstElemType, MaxErrors: parsedInfo.ConversionPairs[0].MaxErrors})
		// }
		// Call logic, considering if srcItem or dstItem are pointers
		// This assumes srcItem is a value from range loop. If srcSliceType.Elem is *T, srcItem is *T.
		// And dstSliceType.Elem determines if convertedItem should be T or *T.

		// Simplified: Assume elemHelper takes value, returns value for now. Pointer logic wrapped here.
		// If srcElemType is *S, srcItem is *S. If dstElemType is *D, dst[i] is *D.
		// helper signature: func sToD(ec, S) D

		callSrcArg := srcItemName
		assignmentTarget := fmt.Sprintf("%s[i]", dstFieldPath)

		if srcElemType.IsPointer && !dstElemType.IsPointer { // *S -> D
			fmt.Fprintf(buf, "\t\t\tif %s != nil {\n", srcItemName)
			fmt.Fprintf(buf, "\t\t\t\t%s = %s(ec, *%s)\n", assignmentTarget, elemHelperName, srcItemName)
			// TODO: else if required (for element)? This 'required' usually applies to parent field.
			fmt.Fprintf(buf, "\t\t\t} else {\n")
			// if element is required, error. For now, assign zero value.
			fmt.Fprintf(buf, "\t\t\t\t// %s is nil, %s will be zero value for its type\n", srcItemName, assignmentTarget)
			fmt.Fprintf(buf, "\t\t\t}\n")
		} else if !srcElemType.IsPointer && dstElemType.IsPointer { // S -> *D
			fmt.Fprintf(buf, "\t\t\t{\n") // Scope for temp var
			fmt.Fprintf(buf, "\t\t\t\tconvertedElem := %s(ec, %s)\n", elemHelperName, callSrcArg)
			fmt.Fprintf(buf, "\t\t\t\t%s = &convertedElem\n", assignmentTarget)
			fmt.Fprintf(buf, "\t\t\t}\n")
		} else if srcElemType.IsPointer && dstElemType.IsPointer { // *S -> *D
			fmt.Fprintf(buf, "\t\t\tif %s != nil {\n", srcItemName)
			fmt.Fprintf(buf, "\t\t\t\tconvertedElem := %s(ec, *%s)\n", elemHelperName, srcItemName)
			fmt.Fprintf(buf, "\t\t\t\t%s = &convertedElem\n", assignmentTarget)
			fmt.Fprintf(buf, "\t\t\t} else {\n")
			// %s[i] remains nil
			fmt.Fprintf(buf, "\t\t\t\t%s = nil\n", assignmentTarget)
			fmt.Fprintf(buf, "\t\t\t}\n")
		} else { // S -> D (no pointers on elements)
			fmt.Fprintf(buf, "\t\t\t%s = %s(ec, %s)\n", assignmentTarget, elemHelperName, callSrcArg)
		}

	} else { // Basic types, or other non-struct non-identical that might need using rule or cast
		// This part would be similar to the main field logic: check using, then direct assign/cast for basic compatible, then error
		// For now, simplify: if not identical and not structs, assume direct assignment or error.
		// TODO: Expand slice element conversion to handle 'using' rules for elements, underlying type casts for elements.
		if srcElemType.FullName == dstElemType.FullName { // Handles T->T, *T->*T for elements
			fmt.Fprintf(buf, "\t\t\t%s[i] = %s\n", dstFieldPath, srcItemName)
		} else if !srcElemType.IsPointer && dstElemType.IsPointer && srcElemType.FullName == dstElemType.Elem.FullName { // T -> *T
			fmt.Fprintf(buf, "\t\t\tval := %s; %s[i] = &val\n", srcItemName, dstFieldPath)
		} else if srcElemType.IsPointer && !dstElemType.IsPointer && srcElemType.Elem.FullName == dstElemType.FullName { // *T -> T
			fmt.Fprintf(buf, "\t\t\tif %s != nil { %s[i] = *%s }\n", srcItemName, dstFieldPath, srcItemName)
			// TODO: else if required (for element)?
		} else {
			fmt.Fprintf(buf, "\t\t\t// TODO: Conversion for slice element %s to %s not fully implemented.\n", srcElemType.FullName, dstElemType.FullName)
			fmt.Fprintf(buf, "\t\t\tec.Addf(\"slice element conversion from %s to %s not implemented for item at index %%d\", i)\n", srcElemType.FullName, dstElemType.FullName)
		}
	}

	fmt.Fprintf(buf, "\t\t\tec.Leave()\n")
	fmt.Fprintf(buf, "\t\t\tif ec.MaxErrorsReached() { break } // Exit loop early if max errors reached\n")
	fmt.Fprintf(buf, "\t\t}\n")
	fmt.Fprintf(buf, "\t}\n") // else for srcFieldPath != nil
	*/
	// --- End of commented out body for generateSliceConversion ---
	return newPairs, nil
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
			return alias + "." + ti.Name
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
		path := typeInfo.PackagePath

		// If this path is already imported, use its existing alias.
		if existingAlias, ok := imports[path]; ok {
			typeInfo.PackageName = existingAlias // Ensure TypeInfo has the potentially unique alias
			return
		}

		// Determine preferred alias:
		// 1. From TypeInfo.PackageName if set by parser (e.g. from `alias.Type`)
		// 2. Else, base name of the package path.
		preferredAlias := typeInfo.PackageName // This might be the original alias from source
		if preferredAlias == "" || preferredAlias == "." { // if no alias or dot import, use basename
			preferredAlias = filepath.Base(path)
		}

		// Ensure alias is valid Go identifier (simple check, can be improved)
		if !isValidGoIdentifier(preferredAlias) {
		    preferredAlias = strings.ReplaceAll(preferredAlias, "-", "_") // common case
		    preferredAlias = strings.ReplaceAll(preferredAlias, ".", "_")
		    if !isValidGoIdentifier(preferredAlias) { // if still not valid
		        preferredAlias = "pkg" // generic fallback
		    }
		}


		finalAlias := preferredAlias
		counter := 1
		// Check for collisions: if this finalAlias is already used for a *different* path.
		// Keep trying new aliases (e.g., pkg1, pkg2) until a unique one is found.
		for {
			collision := false
			for existingPath, aliasInMap := range imports {
				if aliasInMap == finalAlias && existingPath != path {
					collision = true
					break
				}
			}
			if !collision {
				break
			}
			finalAlias = fmt.Sprintf("%s%d", preferredAlias, counter)
			counter++
		}

		imports[path] = finalAlias
		typeInfo.PackageName = finalAlias // Update TypeInfo to use the resolved (potentially unique) alias
	}
}

// isValidGoIdentifier checks if a string is a valid Go identifier (simplified).
func isValidGoIdentifier(name string) bool {
	if name == "" {
		return false
	}
	// For simplicity, check if first char is a letter and rest are letters or numbers.
	// This doesn't cover all Unicode rules but is a basic check.
	// A proper check would use unicode.IsLetter and unicode.IsDigit.
	if !((name[0] >= 'a' && name[0] <= 'z') || (name[0] >= 'A' && name[0] <= 'Z')) {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !((name[i] >= 'a' && name[i] <= 'z') || (name[i] >= 'A' && name[i] <= 'Z') || (name[i] >= '0' && name[i] <= '9')) {
			return false
		}
	}
	return true
}


func lowercaseFirstLetter(s string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.ToLower(s[0:1]) + s[1:]
}

// Capitalize makes the first letter of a string uppercase.
func Capitalize(s string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.ToUpper(s[0:1]) + s[1:]
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
