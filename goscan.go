package goscan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go/token"

	"github.com/podhmo/go-scan/cache"
	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
)

// Re-export scanner kinds for convenience.
const (
	StructKind    = scanner.StructKind
	AliasKind     = scanner.AliasKind
	FuncKind      = scanner.FuncKind
	InterfaceKind = scanner.InterfaceKind // Ensure InterfaceKind is available
)

// Implements checks if a struct type implements an interface type within the context of a package.
// It requires the PackageInfo to look up methods of the structCandidate.
func Implements(structCandidate *scanner.TypeInfo, interfaceDef *scanner.TypeInfo, pkgInfo *scanner.PackageInfo) bool {
	if structCandidate == nil || structCandidate.Kind != StructKind {
		return false // Candidate must be a struct
	}
	if interfaceDef == nil || interfaceDef.Kind != InterfaceKind || interfaceDef.Interface == nil {
		return false // Interface definition must be a valid interface
	}
	if pkgInfo == nil {
		return false // Package context is needed to find struct methods
	}

	// Collect methods of the structCandidate from pkgInfo.Functions
	// This is a simplified way; a more robust way might involve caching methods on TypeInfo.
	structMethods := make(map[string]*scanner.FunctionInfo)
	for _, fn := range pkgInfo.Functions {
		if fn.Receiver != nil && fn.Receiver.Type != nil {
			receiverTypeName := fn.Receiver.Type.Name
			// Handle pointer receivers, e.g. "*MyStruct" vs "MyStruct"
			if fn.Receiver.Type.IsPointer && len(receiverTypeName) > 0 && receiverTypeName[0] == '*' {
				// This comparison is simplistic. True type resolution is complex.
				// For now, assume Type.Name for pointer receiver is like "*StructName".
				// This might need adjustment based on how FieldType.Name for pointer types is structured.
				// Let's assume FieldType.Name for `*Foo` is `*Foo`, and for `Foo` is `Foo`.
				// The receiver type name might need stripping of '*' for comparison if structCandidate.Name doesn't have it.
				// Or, ensure structCandidate.Name is used consistently.
				// For now, let's assume fn.Receiver.Type.Name is the base name for pointer receivers after parsing.
				// This is a common point of failure if not handled carefully by the parser.
				// Let's assume fn.Receiver.Type.Name is "MyStruct" even for *MyStruct for simplicity here, needs verification.
				// Based on scanner.go, parseFuncDecl gets receiver type via parseTypeExpr.
				// FieldType.Name for *ast.StarExpr prepends "*" if not handled.
				// Let's assume for now fn.Receiver.Type.Name could be "*StructName" or "StructName"
				// And structCandidate.Name is "StructName".

				actualReceiverName := receiverTypeName
				if fn.Receiver.Type.IsPointer && strings.HasPrefix(receiverTypeName, "*") {
					actualReceiverName = strings.TrimPrefix(receiverTypeName, "*")
				}

				if actualReceiverName == structCandidate.Name {
					structMethods[fn.Name] = fn
				}
			} else if receiverTypeName == structCandidate.Name { // Value receiver
				structMethods[fn.Name] = fn
			}
		}
	}

	for _, interfaceMethod := range interfaceDef.Interface.Methods {
		structMethod, found := structMethods[interfaceMethod.Name]
		if !found {
			// fmt.Printf("Method %s not found on struct %s\n", interfaceMethod.Name, structCandidate.Name)
			return false // Method not found
		}

		// Compare signatures (parameters and results)
		if !compareSignatures(interfaceMethod, structMethod) {
			// fmt.Printf("Signature mismatch for method %s on struct %s\n", interfaceMethod.Name, structCandidate.Name)
			return false
		}
	}

	return true
}

// compareSignatures compares the parameters and results of two methods.
// This is a simplified comparison focusing on type names and counts.
// It does not handle complex type equivalences (e.g., type aliases across packages without full resolution).
func compareSignatures(interfaceMethod *scanner.MethodInfo, structMethod *scanner.FunctionInfo) bool {
	// Compare parameters
	if len(interfaceMethod.Parameters) != len(structMethod.Parameters) {
		// fmt.Printf("Param count mismatch: %d vs %d\n", len(interfaceMethod.Parameters), len(structMethod.Parameters))
		return false
	}
	for i, intParam := range interfaceMethod.Parameters {
		strParam := structMethod.Parameters[i]
		if !compareFieldTypes(intParam.Type, strParam.Type) {
			// fmt.Printf("Param type mismatch at index %d: %s vs %s\n", i, intParam.Type.Name, strParam.Type.Name)
			return false
		}
	}

	// Compare results
	if len(interfaceMethod.Results) != len(structMethod.Results) {
		// fmt.Printf("Result count mismatch: %d vs %d\n", len(interfaceMethod.Results), len(structMethod.Results))
		return false
	}
	for i, intResult := range interfaceMethod.Results {
		strResult := structMethod.Results[i]
		if !compareFieldTypes(intResult.Type, strResult.Type) {
			// fmt.Printf("Result type mismatch at index %d: %s vs %s\n", i, intResult.Type.Name, strResult.Type.Name)
			return false
		}
	}

	return true
}

// compareFieldTypes compares two FieldType instances.
// This is a simplified comparison. A robust solution needs full type resolution.
func compareFieldTypes(type1 *scanner.FieldType, type2 *scanner.FieldType) bool {
	if type1 == nil && type2 == nil {
		return true
	}
	if type1 == nil || type2 == nil {
		return false
	}

	// TODO: This needs to be much more robust.
	// It should handle qualified names, resolve types if necessary, etc.
	// For now, simple name and pointer check.
	// Also, consider IsSlice, IsMap, Elem, MapKey for more complex types.

	// Normalize names: if PkgName is present and type1/2 are from different packages,
	// we need to compare fully qualified names or ensure types are resolved to canonical forms.
	// For types within the same package or primitives, direct name comparison might work.
	// ft.Resolve() could be used here, but adds complexity of error handling and async operations.

	name1 := type1.Name
	name2 := type2.Name

	// Thus, we can directly compare IsPointer and then Name.

	if type1.IsPointer != type2.IsPointer {
		return false
	}

	// Handle slices
	if type1.IsSlice != type2.IsSlice {
		return false
	}
	if type1.IsSlice { // Both are slices
		return compareFieldTypes(type1.Elem, type2.Elem) // Compare element types
	}

	// Handle maps
	if type1.IsMap != type2.IsMap {
		return false
	}
	if type1.IsMap { // Both are maps
		// Compare key types AND value types
		if !compareFieldTypes(type1.MapKey, type2.MapKey) {
			return false
		}
		return compareFieldTypes(type1.Elem, type2.Elem)
	}

	// If not slices or maps, compare base names (IsPointer is already checked and equal)
	// This is where PkgName/ImportPath should be checked for non-primitive, non-builtin types.
	// For now, just comparing names.
	if name1 != name2 {
		// Consider logging here for debugging type mismatches:
		// fmt.Printf("Base name mismatch: T1: %s (pkg:%s) vs T2: %s (pkg:%s)\n", name1, type1.PkgName, name2, type2.PkgName)
		return false
	}

	// TODO: Enhance PkgName and fullImportPath comparison for robust cross-package type identity.
	// For example:
	// if type1.PkgName != type2.PkgName {
	//    // If PkgName is different, names must be fully qualified or resolved via import paths
	//    // This requires type1.FullImportPath and type2.FullImportPath to be populated and compared.
	//    // For now, if PkgName differs and names were identical (e.g. "MyType"), it's a mismatch unless they are built-in.
	//    isBuiltinOrPredeclared := func(name string) bool {
	//        // Add checks for "string", "int", "bool", "error", etc.
	//        // Or rely on PkgName being empty or a special value for builtins.
	//        // scanner.FieldType might need a field like IsBuiltin.
	// 	   return name == "string" || name == "int" // ... and so on
	//    }
	//    if !(isBuiltinOrPredeclared(name1) && type1.PkgName == "" && type2.PkgName == "") && /* more conditions */ {
	//        return false
	//    }
	// }

	return true
}

// Scanner is the main entry point for the type scanning library.
// It combines a locator for finding packages, a scanner for parsing them,
// and caches for improving performance over multiple calls.
// Scanner instances are stateful regarding which files have been visited (parsed).
type Scanner struct {
	locator      *locator.Locator
	scanner      *scanner.Scanner
	packageCache map[string]*scanner.PackageInfo // Cache for PackageInfo from ScanPackage/ScanPackageByImport, key is import path
	visitedFiles map[string]struct{}             // Set of visited (parsed) file absolute paths for this Scanner instance.
	mu           sync.RWMutex
	fset         *token.FileSet

	CachePath             string
	symbolCache           *cache.SymbolCache // Symbol cache (persisted across Scanner instances if path is reused)
	ExternalTypeOverrides scanner.ExternalTypeOverride
}

// New creates a new Scanner. It finds the module root starting from the given path.
// It also initializes an empty set of visited files for this scanner instance.
func New(startPath string) (*Scanner, error) {
	loc, err := locator.New(startPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize locator: %w", err)
	}

	fset := token.NewFileSet()
	initialScanner, err := scanner.New(fset, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create internal scanner: %w", err)
	}

	return &Scanner{
		locator:               loc,
		scanner:               initialScanner,
		packageCache:          make(map[string]*scanner.PackageInfo),
		visitedFiles:          make(map[string]struct{}), // Initialize visitedFiles
		fset:                  fset,
		ExternalTypeOverrides: make(scanner.ExternalTypeOverride),
	}, nil
}

// SetExternalTypeOverrides sets the external type override map for the scanner.
func (s *Scanner) SetExternalTypeOverrides(overrides scanner.ExternalTypeOverride) {
	if overrides == nil {
		overrides = make(scanner.ExternalTypeOverride)
	}
	s.ExternalTypeOverrides = overrides
	newInternalScanner, err := scanner.New(s.fset, s.ExternalTypeOverrides)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to re-initialize internal scanner with new overrides: %v. Continuing with previous scanner settings.\n", err)
		return
	}
	s.scanner = newInternalScanner
}

// listGoFiles lists all .go files (excluding _test.go) in a directory.
// It returns a list of absolute file paths.
func listGoFiles(dirPath string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("listGoFiles: failed to read dir %s: %w", dirPath, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			absPath, err := filepath.Abs(filepath.Join(dirPath, entry.Name()))
			if err != nil {
				return nil, fmt.Errorf("listGoFiles: could not get absolute path for %s: %w", entry.Name(), err)
			}
			files = append(files, absPath)
		}
	}
	return files, nil
}

// ScanPackage scans a single package at a given directory path (absolute or relative to CWD).
// It parses all .go files (excluding _test.go) in that directory that have not yet been
// visited (parsed) by this Scanner instance.
// The returned PackageInfo contains information derived ONLY from the files parsed in THIS specific call.
// If no unvisited files are found in the package, the returned PackageInfo will be minimal
// (e.g., Path and ImportPath set, but no types/functions unless a previous cached version for the entire package is returned).
// The result of this call (representing the newly parsed files, or a prior cached full result if no new files were parsed and cache existed)
// is stored in an in-memory package cache (s.packageCache) for subsequent calls to ScanPackage or ScanPackageByImport
// for the same import path.
// The global symbol cache (s.symbolCache), if enabled, is updated with symbols from the newly parsed files.
func (s *Scanner) ScanPackage(pkgPath string) (*scanner.PackageInfo, error) {
	absPkgPath, err := filepath.Abs(pkgPath)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute path for package path %s: %w", pkgPath, err)
	}
	info, err := os.Stat(absPkgPath)
	if err != nil {
		return nil, fmt.Errorf("could not stat path %s: %w", absPkgPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path %s is not a directory", absPkgPath)
	}

	moduleRoot := s.locator.RootDir()
	modulePath := s.locator.ModulePath()
	var importPath string

	if modulePath != "" && moduleRoot != "" && strings.HasPrefix(absPkgPath, moduleRoot) {
		relPath, rErr := filepath.Rel(moduleRoot, absPkgPath)
		if rErr != nil {
			return nil, fmt.Errorf("could not determine relative path for %s from module root %s: %w", absPkgPath, moduleRoot, rErr)
		}
		if relPath == "." || relPath == "" {
			importPath = modulePath
		} else {
			importPath = filepath.ToSlash(filepath.Join(modulePath, relPath))
		}
	} else {
		// Try to determine import path for standard library packages or other non-module paths
		// This part might be complex and require go list or similar logic for full accuracy.
		// For now, if not in module, we might not be able to form a canonical import path.
		// However, ScanPackage is often called with a direct path, so importPath might be less critical
		// than for ScanPackageByImport. Let's use the directory name as a fallback package name.
		// If a robust import path is needed for out-of-module packages, this needs enhancement.
		if modulePath == "" && moduleRoot == "" { // Likely not in a module context
			fmt.Fprintf(os.Stderr, "warning: ScanPackage called for %s which is likely outside a Go module, import path may be inaccurate.\n", absPkgPath)
			importPath = filepath.Base(absPkgPath) // Fallback
		} else if modulePath == "" { // Locator initialized but no go.mod?
			return nil, fmt.Errorf("module path is empty, but ScanPackage called for %s. Locator issue or not in module?", absPkgPath)
		} else { // Inside a module context, but pkgPath is outside moduleRoot
			return nil, fmt.Errorf("package directory %s is outside the module root %s, cannot determine canonical import path", absPkgPath, moduleRoot)
		}
	}

	allFilesInDir, err := listGoFiles(absPkgPath)
	if err != nil {
		return nil, fmt.Errorf("ScanPackage: could not list go files in %s: %w", absPkgPath, err)
	}

	var filesToParseNow []string
	for _, fp := range allFilesInDir {
		if _, visited := s.visitedFiles[fp]; !visited {
			filesToParseNow = append(filesToParseNow, fp)
		}
	}

	var currentCallPkgInfo *scanner.PackageInfo
	if len(filesToParseNow) > 0 {
		currentCallPkgInfo, err = s.scanner.ScanFiles(filesToParseNow, absPkgPath, s)
		if err != nil {
			return nil, fmt.Errorf("ScanPackage: internal scan of files for package %s failed: %w", absPkgPath, err)
		}
		if currentCallPkgInfo != nil {
			for _, fp := range currentCallPkgInfo.Files { // Files actually parsed in this call
				s.visitedFiles[fp] = struct{}{}
			}
			currentCallPkgInfo.ImportPath = importPath // Set import path for this call's result
			currentCallPkgInfo.Path = absPkgPath       // Ensure path is set
			s.updateSymbolCacheWithPackageInfo(importPath, currentCallPkgInfo)
		}
	}

	// Update the main package cache with the cumulative information for this importPath.
	// This requires merging if a previous entry existed. For now, replace.
	// A more robust strategy might involve storing all PackageInfo from each scan call and merging on demand.
	// For now, the cache will store the result of the latest ScanPackage or ScanPackageByImport call.
	// If no new files were parsed, currentCallPkgInfo will be nil.
	// We should ensure a PackageInfo object is always cached if the package itself is valid (even if empty of new symbols).
	if currentCallPkgInfo == nil { // No new files parsed
		s.mu.RLock()
		existingCachedInfo, found := s.packageCache[importPath]
		s.mu.RUnlock()
		if found {
			return existingCachedInfo, nil // Return existing full cache if nothing new parsed
		}
		// If no cache and no new files, create a minimal PackageInfo
		currentCallPkgInfo = &scanner.PackageInfo{
			Path:       absPkgPath,
			ImportPath: importPath,
			Name:       filepath.Base(absPkgPath), // Best guess for name
			Fset:       s.fset,
			Files:      []string{}, // No files parsed in *this call*
		}
	}

	// Ensure the PackageInfo reflects all known files in the directory for its Files list if it's a full ScanPackage result
	// This is tricky without merging. The current `currentCallPkgInfo.Files` only has *newly* parsed files.
	// For ScanPackage, the expectation is often a view of the whole package.
	// Let's adjust: if currentCallPkgInfo was non-nil (new files parsed), its .Files is correct for *this scan*.
	// If we are to cache a "full" view, we'd need to merge or reconstruct.
	// Given "no merge" for ScanFiles, let's keep ScanPackage simple: its return and cache reflect *this call's parsed files*.
	// This means s.packageCache might hold partial info if ScanPackage is called after ScanFiles visited some.
	// This seems to align with the "no merge" philosophy more consistently.
	// The `Files` field of PackageInfo will list files parsed in *this specific call*.

	s.mu.Lock()
	s.packageCache[importPath] = currentCallPkgInfo // Cache the result of this specific call
	s.mu.Unlock()

	return currentCallPkgInfo, nil
}

// resolveFilePath attempts to resolve a given path string (rawPath) into an absolute file path.
func (s *Scanner) resolveFilePath(rawPath string) (string, error) {
	checkFile := func(p string) (string, bool) {
		absP, err := filepath.Abs(p)
		if err != nil {
			return "", false
		}
		info, err := os.Stat(absP)
		if err == nil && !info.IsDir() && strings.HasSuffix(strings.ToLower(absP), ".go") { // Check .go case-insensitively for robustness
			return absP, true
		}
		return "", false
	}

	// Try as absolute or CWD-relative path first
	if absPath, ok := checkFile(rawPath); ok {
		return absPath, nil
	}

	// Try as module-qualified path
	if s.locator != nil {
		modulePath := s.locator.ModulePath()
		moduleRoot := s.locator.RootDir()
		if modulePath != "" && moduleRoot != "" && strings.HasPrefix(rawPath, modulePath) {
			prefixToTrim := modulePath
			// Ensure we are trimming "modulePath/" not just "modulePath" if there's more path
			if !strings.HasSuffix(modulePath, "/") && len(rawPath) > len(modulePath) && rawPath[len(modulePath)] == '/' {
				prefixToTrim += "/"
			} else if rawPath == modulePath { // rawPath is just the module path, not a file in it
				return "", fmt.Errorf("path %q is a module path, not a file path within the module", rawPath)
			}

			if strings.HasPrefix(rawPath, prefixToTrim) {
				suffixPath := strings.TrimPrefix(rawPath, prefixToTrim)
				candidatePath := filepath.Join(moduleRoot, suffixPath)
				if absPath, ok := checkFile(candidatePath); ok {
					return absPath, nil
				}
			}
		}
	}
	return "", fmt.Errorf("could not resolve path %q to an existing .go file", rawPath)
}

// ScanFiles scans a specified set of Go files.
//
// File paths in the `filePaths` argument can be provided in three forms:
//  1. Absolute path (e.g., "/path/to/your/project/pkg/file.go").
//  2. Path relative to the current working directory (CWD) (e.g., "pkg/file.go").
//  3. Module-qualified path (e.g., "github.com/your/module/pkg/file.go"), which is resolved
//     using the Scanner's associated module information (from go.mod).
//
// All provided file paths, after resolution, must belong to the same directory,
// effectively meaning they must be part of the same Go package.
//
// This function only parses files that have not been previously visited (parsed)
// by this specific Scanner instance (tracked in `s.visitedFiles`).
//
// The returned `scanner.PackageInfo` contains information derived *only* from the
// files that were newly parsed in *this specific call*. If all specified files
// were already visited, the `PackageInfo.Files` list (and consequently Types, Functions, etc.)
// will be empty, though `Path` and `ImportPath` will be set according to the files' package.
//
// Results from `ScanFiles` are *not* stored in the main package cache (`s.packageCache`)
// because they represent partial package information. However, the global symbol
// cache (`s.symbolCache`), if enabled, *is* updated with symbols from the newly parsed files.
// Files parsed by this function are marked as visited in `s.visitedFiles`.
func (s *Scanner) ScanFiles(filePaths []string) (*scanner.PackageInfo, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no file paths provided to ScanFiles")
	}
	if s.locator == nil {
		return nil, fmt.Errorf("scanner locator is not initialized")
	}
	moduleRoot := s.locator.RootDir()
	modulePath := s.locator.ModulePath()
	if modulePath == "" && moduleRoot == "" { // Heuristic: not in a module context at all
		// Allow scanning if files are absolute paths and locator isn't strictly needed for path resolution itself
		// but import path calculation will be severely limited.
		fmt.Fprintf(os.Stderr, "warning: ScanFiles called likely outside a Go module context. Import path resolution will be affected.\n")
	} else if modulePath == "" || moduleRoot == "" { // Inconsistent module info
		return nil, fmt.Errorf("module path or root is empty, ensure a go.mod file exists and is discoverable by the scanner's locator")
	}

	var resolvedAbsFilePaths []string
	var firstFileDir string

	for i, rawFp := range filePaths {
		absFp, err := s.resolveFilePath(rawFp)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve file path %q: %w", rawFp, err)
		}
		resolvedAbsFilePaths = append(resolvedAbsFilePaths, absFp)
		currentFileDir := filepath.Dir(absFp)
		if i == 0 {
			firstFileDir = currentFileDir
		} else if currentFileDir != firstFileDir {
			return nil, fmt.Errorf("all files must belong to the same directory (package); %s is in %s, but expected %s", absFp, currentFileDir, firstFileDir)
		}
	}

	pkgDirAbs := firstFileDir
	var importPath string

	if modulePath != "" && moduleRoot != "" { // Only attempt module-based import path if module context is valid
		if !strings.HasPrefix(pkgDirAbs, moduleRoot) {
			return nil, fmt.Errorf("package directory %s is outside the module root %s, cannot determine module-relative import path", pkgDirAbs, moduleRoot)
		}
		relPath, err := filepath.Rel(moduleRoot, pkgDirAbs)
		if err != nil {
			return nil, fmt.Errorf("could not determine relative path for %s from module root %s: %w", pkgDirAbs, moduleRoot, err)
		}
		if relPath == "." || relPath == "" {
			importPath = modulePath
		} else {
			importPath = filepath.ToSlash(filepath.Join(modulePath, relPath))
		}
	} else { // Fallback if not in a clear module context (e.g. scanning /usr/local/go/src/fmt)
		// This part needs careful consideration for how to represent non-module packages.
		// For now, use the directory path as a pseudo-import path.
		importPath = filepath.ToSlash(pkgDirAbs)
		fmt.Fprintf(os.Stderr, "warning: creating pseudo import path %q for package at %s\n", importPath, pkgDirAbs)
	}

	var filesToParse []string
	for _, absFp := range resolvedAbsFilePaths {
		if _, visited := s.visitedFiles[absFp]; !visited {
			filesToParse = append(filesToParse, absFp)
		}
	}

	if len(filesToParse) == 0 { // All specified files already visited
		// Return an empty PackageInfo but with correct Path/ImportPath
		return &scanner.PackageInfo{
			Path:       pkgDirAbs,
			ImportPath: importPath,
			Name:       "", // Name would require parsing or looking up a cached full PackageInfo
			Fset:       s.fset,
			Files:      []string{}, // No files *newly* parsed
		}, nil
	}

	pkgInfo, err := s.scanner.ScanFiles(filesToParse, pkgDirAbs, s) // Scan only unvisited files
	if err != nil {
		return nil, fmt.Errorf("failed to scan files in %s (import path %s): %w", pkgDirAbs, importPath, err)
	}

	if pkgInfo != nil {
		pkgInfo.ImportPath = importPath    // Set the calculated import path
		pkgInfo.Path = pkgDirAbs           // Ensure directory path is also set
		for _, fp := range pkgInfo.Files { // Mark newly parsed files as visited
			s.visitedFiles[fp] = struct{}{}
		}
		// Results from ScanFiles (which are partial by design based on unvisited files)
		// are NOT cached in s.packageCache. Only symbol cache is updated.
		s.updateSymbolCacheWithPackageInfo(importPath, pkgInfo)
	}
	return pkgInfo, nil
}

// UnscannedGoFiles returns a list of absolute paths to .go files (excluding _test.go files)
// within the specified package that have not yet been visited (parsed) by this Scanner instance.
//
// The `packagePathOrImportPath` argument can be:
//  1. An absolute directory path to the package.
//  2. A directory path relative to the current working directory (CWD).
//  3. A Go import path (e.g., "github.com/your/module/pkg"), which will be resolved
//     to a directory using the Scanner's locator.
//
// This method lists all relevant .go files in the identified package directory
// and filters out those already present in the Scanner's `visitedFiles` set.
// It is useful for discovering which files in a package still need to be processed
// if performing iterative scanning.
func (s *Scanner) UnscannedGoFiles(packagePathOrImportPath string) ([]string, error) {
	if s.locator == nil && !(filepath.IsAbs(packagePathOrImportPath) && isDir(packagePathOrImportPath)) {
		// If locator is nil, we can only proceed if packagePathOrImportPath is an absolute directory path.
		return nil, fmt.Errorf("scanner locator is not initialized, and path is not an absolute directory to a package")
	}

	var pkgDirAbs string
	var err error

	// Try as a direct file system path first (absolute or CWD-relative directory)
	pathAsDir, err := filepath.Abs(packagePathOrImportPath)
	if err == nil {
		info, statErr := os.Stat(pathAsDir)
		if statErr == nil && info.IsDir() {
			pkgDirAbs = pathAsDir
		}
	}

	// If not resolved as a direct directory path, try as an import path via locator (if locator exists)
	if pkgDirAbs == "" {
		if s.locator == nil { // Guard again, as locator might be nil
			return nil, fmt.Errorf("cannot resolve %q as import path: locator not available", packagePathOrImportPath)
		}
		pkgDirAbs, err = s.locator.FindPackageDir(packagePathOrImportPath)
		if err != nil {
			return nil, fmt.Errorf("could not find package directory for %q (tried as path and import path): %w", packagePathOrImportPath, err)
		}
	}

	allGoFilesInDir, err := listGoFiles(pkgDirAbs) // listGoFiles returns absolute paths
	if err != nil {
		return nil, fmt.Errorf("UnscannedGoFiles: could not list go files in %s: %w", pkgDirAbs, err)
	}

	var unscannedFiles []string
	for _, absFilePath := range allGoFilesInDir {
		if _, visited := s.visitedFiles[absFilePath]; !visited {
			unscannedFiles = append(unscannedFiles, absFilePath)
		}
	}
	return unscannedFiles, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ScanPackageByImport scans a single Go package identified by its import path.
//
// This function resolves the import path to a directory using the Scanner's locator.
// It then attempts to parse all .go files (excluding _test.go files) in that directory
// that have not yet been visited by this Scanner instance (`s.visitedFiles`).
// The selection of files to parse may also be influenced by the state of the
// symbol cache (`s.symbolCache`), if enabled, to avoid re-parsing unchanged files
// for which symbol information is already cached and deemed valid.
//
// The returned `scanner.PackageInfo` contains information derived from the files
// parsed or processed in *this specific call*.
//
// The result of this call is stored in an in-memory package cache (`s.packageCache`)
// and is intended to represent the Scanner's current understanding of the package,
// which might be based on a full parse of unvisited files or a combination of
// cached data and newly parsed information.
// The global symbol cache (`s.symbolCache`), if enabled, is updated with symbols
// from any newly parsed files. Files parsed by this function are marked as visited
// in `s.visitedFiles`.
func (s *Scanner) ScanPackageByImport(importPath string) (*scanner.PackageInfo, error) {
	s.mu.RLock()
	cachedPkg, found := s.packageCache[importPath]
	s.mu.RUnlock()
	if found {
		fmt.Printf("DEBUG: ScanPackageByImport CACHE HIT for %s. Returning cached PackageInfo with %d types.\n", importPath, len(cachedPkg.Types))
		return cachedPkg, nil
	}
	fmt.Printf("DEBUG: ScanPackageByImport CACHE MISS for %s.\n", importPath)

	pkgDirAbs, err := s.locator.FindPackageDir(importPath)
	if err != nil {
		return nil, fmt.Errorf("could not find directory for import path %s: %w", importPath, err)
	}
	fmt.Printf("DEBUG: ScanPackageByImport resolved import path %s to directory %s.\n", importPath, pkgDirAbs)

	allGoFilesInPkg, err := listGoFiles(pkgDirAbs) // Gets absolute paths
	if err != nil {
		return nil, fmt.Errorf("ScanPackageByImport: failed to list go files in %s: %w", pkgDirAbs, err)
	}
	fmt.Printf("DEBUG: ScanPackageByImport found %d .go files in %s: %v\n", len(allGoFilesInPkg), pkgDirAbs, allGoFilesInPkg)

	if len(allGoFilesInPkg) == 0 {
		// If a directory for an import path exists but has no .go files, cache an empty PackageInfo.
		fmt.Printf("DEBUG: ScanPackageByImport found no .go files in %s. Caching empty PackageInfo.\n", pkgDirAbs)
		pkgInfo := &scanner.PackageInfo{Path: pkgDirAbs, ImportPath: importPath, Name: "", Fset: s.fset, Files: []string{}, Types: []*scanner.TypeInfo{}}
		s.mu.Lock()
		s.packageCache[importPath] = pkgInfo
		s.mu.Unlock()
		return pkgInfo, nil
	}

	var filesToParseThisCall []string
	symCache, _ := s.getOrCreateSymbolCache() // Error getting cache is not fatal here
	fmt.Printf("DEBUG: ScanPackageByImport for %s, symbol cache enabled: %t\n", importPath, symCache != nil && symCache.IsEnabled())

	filesConsideredBySymCache := make(map[string]struct{})

	if symCache != nil && symCache.IsEnabled() {
		newDiskFiles, existingDiskFiles, errSym := symCache.GetFilesToScan(pkgDirAbs)
		if errSym != nil {
			fmt.Fprintf(os.Stderr, "warning: GetFilesToScan for %s (%s) failed: %v. Will scan all unvisited files in the package.\n", importPath, pkgDirAbs, errSym)
			// Fallback: scan all files in the package that this Scanner instance hasn't visited.
			for _, f := range allGoFilesInPkg {
				if _, visited := s.visitedFiles[f]; !visited {
					filesToParseThisCall = append(filesToParseThisCall, f)
				}
			}
		} else {
			// Add files symCache identified as new/changed
			for _, f := range newDiskFiles {
				filesToParseThisCall = append(filesToParseThisCall, f)
				filesConsideredBySymCache[f] = struct{}{}
			}
			// For files symCache says are existing (potentially unchanged),
			// only parse if this Scanner instance hasn't visited them yet.
			for _, f := range existingDiskFiles {
				filesConsideredBySymCache[f] = struct{}{} // Mark as considered
				if _, visited := s.visitedFiles[f]; !visited {
					filesToParseThisCall = append(filesToParseThisCall, f)
				}
			}
		}
	}

	// Add any file in the directory not mentioned by symCache (e.g. untracked) if unvisited by this Scanner instance
	for _, f := range allGoFilesInPkg {
		if _, considered := filesConsideredBySymCache[f]; !considered {
			if _, visited := s.visitedFiles[f]; !visited {
				filesToParseThisCall = append(filesToParseThisCall, f)
			}
		}
	}

	// Deduplicate filesToParseThisCall (abs paths, so simple map is fine)
	uniqueFilesToParse := make(map[string]struct{})
	var dedupedFilesToParse []string
	for _, f := range filesToParseThisCall {
		if _, exists := uniqueFilesToParse[f]; !exists {
			uniqueFilesToParse[f] = struct{}{}
			dedupedFilesToParse = append(dedupedFilesToParse, f)
		}
	}
	filesToParseThisCall = dedupedFilesToParse

	var currentCallPkgInfo *scanner.PackageInfo
	if len(filesToParseThisCall) > 0 {
		currentCallPkgInfo, err = s.scanner.ScanFiles(filesToParseThisCall, pkgDirAbs, s)
		if err != nil {
			return nil, fmt.Errorf("ScanPackageByImport: scanning files for %s failed: %w", importPath, err)
		}
		if currentCallPkgInfo != nil {
			currentCallPkgInfo.ImportPath = importPath    // Set import path
			currentCallPkgInfo.Path = pkgDirAbs           // Ensure path
			for _, fp := range currentCallPkgInfo.Files { // Mark newly parsed files as visited by this instance
				s.visitedFiles[fp] = struct{}{}
			}
			s.updateSymbolCacheWithPackageInfo(importPath, currentCallPkgInfo) // Update global symbol cache
		}
	}

	// If no new files were parsed in this call, but the package is not empty,
	// it means all files were either already visited or symcache deemed them unchanged & visited.
	// We should return a PackageInfo that reflects the package structure.
	if currentCallPkgInfo == nil {
		currentCallPkgInfo = &scanner.PackageInfo{
			Path:       pkgDirAbs,
			ImportPath: importPath,
			Name:       "", // Name might be derivable if any file was ever parsed for this package
			Fset:       s.fset,
			Files:      []string{}, // No files *newly* parsed in this call.
		}
		// Attempt to set a name if possible from a previously (partially) cached PackageInfo
		// This is a bit of a workaround for not merging.
		s.mu.RLock()
		if prevInfo, ok := s.packageCache[importPath]; ok && prevInfo.Name != "" {
			currentCallPkgInfo.Name = prevInfo.Name
		} else if len(allGoFilesInPkg) > 0 { // Try to get from any already visited file if no cache
			// This is complex; for now, leave Name blank if not easily found.
		}
		s.mu.RUnlock()
	}

	// The PackageInfo cached by ScanPackageByImport should represent the state of the package
	// as understood by this call (i.e., including all files parsed up to this point for this package).
	// Since "no merge" is a principle, the cache stores the result of *this specific call*.
	// If this call parsed new files, currentCallPkgInfo has them. If not, it's minimal.
	// This means the packageCache might not always have the "fullest" possible PackageInfo
	// if ScanFiles was used to visit parts of the package before this.
	// This is a known trade-off of the "no merge" + "instance-visited" design.

	s.mu.Lock()
	s.packageCache[importPath] = currentCallPkgInfo
	s.mu.Unlock()

	return currentCallPkgInfo, nil
}

// getOrCreateSymbolCache ensures the symbolCache is initialized.
func (s *Scanner) getOrCreateSymbolCache() (*cache.SymbolCache, error) {
	if s.CachePath == "" {
		if s.symbolCache == nil || s.symbolCache.IsEnabled() {
			rootDir := ""
			if s.locator != nil {
				rootDir = s.locator.RootDir()
			}
			disabledCache, err := cache.NewSymbolCache(rootDir, "")
			if err != nil {
				return nil, fmt.Errorf("failed to initialize a disabled symbol cache: %w", err)
			}
			s.symbolCache = disabledCache
		}
		return s.symbolCache, nil
	}

	if s.symbolCache != nil && s.symbolCache.IsEnabled() && s.symbolCache.FilePath() == s.CachePath {
		return s.symbolCache, nil
	}

	rootDir := ""
	if s.locator != nil {
		rootDir = s.locator.RootDir()
	} else {
		return nil, fmt.Errorf("scanner locator is not initialized, cannot determine root directory for cache")
	}

	sc, err := cache.NewSymbolCache(rootDir, s.CachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize symbol cache with path %s: %w", s.CachePath, err)
	}
	s.symbolCache = sc

	if err := s.symbolCache.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load symbol cache from %s: %v\n", s.symbolCache.FilePath(), err)
	}
	return s.symbolCache, nil
}

// updateSymbolCacheWithPackageInfo updates the symbol cache with information from a given PackageInfo.
// The pkgInfo provided should typically represent the symbols parsed from a specific set of files
// in the context of the given importPath.
func (s *Scanner) updateSymbolCacheWithPackageInfo(importPath string, pkgInfo *scanner.PackageInfo) {
	if s.CachePath == "" || pkgInfo == nil || len(pkgInfo.Files) == 0 {
		return
	}
	symCache, err := s.getOrCreateSymbolCache()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting symbol cache for update: %v\n", err)
		return
	}
	if !symCache.IsEnabled() {
		return
	}

	symbolsByFile := make(map[string][]string)
	addSymbol := func(symbolName, absFilePath string) {
		if symbolName != "" && absFilePath != "" {
			// Ensure absFilePath is truly absolute for consistency
			absFilePath, _ = filepath.Abs(absFilePath) // error unlikely if path came from system
			key := importPath + "." + symbolName
			if err := symCache.SetSymbol(key, absFilePath); err != nil {
				fmt.Fprintf(os.Stderr, "error setting cache for symbol %s: %v\n", key, err)
			}
			symbolsByFile[absFilePath] = append(symbolsByFile[absFilePath], symbolName)
		}
	}

	for _, typeInfo := range pkgInfo.Types {
		addSymbol(typeInfo.Name, typeInfo.FilePath)
	}
	for _, funcInfo := range pkgInfo.Functions {
		addSymbol(funcInfo.Name, funcInfo.FilePath)
	}
	for _, constInfo := range pkgInfo.Constants {
		addSymbol(constInfo.Name, constInfo.FilePath)
	}

	for _, absFilePath := range pkgInfo.Files { // These are files that were actually parsed for pkgInfo
		absFilePath, _ = filepath.Abs(absFilePath) // Ensure absolute
		if _, err := os.Stat(absFilePath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: file %s from pkgInfo.Files not found, skipping for FileMetadata update\n", absFilePath)
			continue
		}
		fileSymbols := symbolsByFile[absFilePath]
		if fileSymbols == nil {
			fileSymbols = []string{}
		}
		metadata := cache.FileMetadata{Symbols: fileSymbols}
		if err := symCache.SetFileMetadata(absFilePath, metadata); err != nil {
			fmt.Fprintf(os.Stderr, "error setting file metadata for %s: %v\n", absFilePath, err)
		}
	}
}

// SaveSymbolCache saves the symbol cache to disk if CachePath is set.
func (s *Scanner) SaveSymbolCache() error {
	if s.CachePath == "" {
		return nil
	}
	if _, err := s.getOrCreateSymbolCache(); err != nil {
		return fmt.Errorf("cannot save symbol cache, failed to ensure cache initialization for path %s: %w", s.CachePath, err)
	}
	if s.symbolCache != nil && s.symbolCache.IsEnabled() {
		if err := s.symbolCache.Save(); err != nil {
			return fmt.Errorf("failed to save symbol cache to %s: %w", s.symbolCache.FilePath(), err)
		}
	}
	return nil
}

// FindSymbolDefinitionLocation attempts to find the absolute file path where a given symbol is defined.
// The `symbolFullName` should be in the format "package/import/path.SymbolName".
//
// It first checks the persistent symbol cache (if enabled and loaded).
// If not found in the cache, it triggers a scan of the relevant package
// (using `ScanPackageByImport`) to populate caches and then re-checks.
// Finally, it inspects the `PackageInfo` obtained from the scan.
func (s *Scanner) FindSymbolDefinitionLocation(symbolFullName string) (string, error) {
	lastDot := strings.LastIndex(symbolFullName, ".")
	if lastDot == -1 || lastDot == 0 || lastDot == len(symbolFullName)-1 {
		return "", fmt.Errorf("invalid symbol full name format: %q. Expected 'package/import/path.SymbolName'", symbolFullName)
	}
	importPath := symbolFullName[:lastDot]
	symbolName := symbolFullName[lastDot+1:]
	cacheKey := importPath + "." + symbolName

	if s.CachePath != "" {
		symCache, err := s.getOrCreateSymbolCache()
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not get symbol cache for %q: %v. Proceeding with full scan.\n", symbolFullName, err)
		} else if symCache != nil && symCache.IsEnabled() {
			filePath, found := symCache.VerifyAndGet(cacheKey)
			if found {
				return filePath, nil
			}
		}
	}
	// If symbol not found in cache, try to scan the package.
	pkgInfo, err := s.ScanPackageByImport(importPath) // This will parse unvisited files and update caches
	if err != nil {
		return "", fmt.Errorf("scan for package %s (for symbol %s) failed: %w", importPath, symbolName, err)
	}

	// After scan, check cache again (if enabled)
	if s.CachePath != "" {
		if s.symbolCache != nil && s.symbolCache.IsEnabled() {
			filePath, found := s.symbolCache.Get(cacheKey)
			if found {
				if _, statErr := os.Stat(filePath); statErr == nil {
					return filePath, nil
				}
				fmt.Fprintf(os.Stderr, "warning: symbol %s found in cache at %s after scan, but file does not exist.\n", symbolFullName, filePath)
			}
		}
	}

	// If still not found via cache, check the pkgInfo returned by the ScanPackageByImport call.
	// This pkgInfo contains symbols from files *parsed in that specific call*.
	if pkgInfo != nil {
		targetFilePath := ""
		for _, t := range pkgInfo.Types {
			if t.Name == symbolName {
				targetFilePath = t.FilePath
				break
			}
		}
		if targetFilePath == "" {
			for _, f := range pkgInfo.Functions {
				if f.Name == symbolName {
					targetFilePath = f.FilePath
					break
				}
			}
		}
		if targetFilePath == "" {
			for _, c := range pkgInfo.Constants {
				if c.Name == symbolName {
					targetFilePath = c.FilePath
					break
				}
			}
		}

		if targetFilePath != "" {
			if _, statErr := os.Stat(targetFilePath); statErr == nil {
				return targetFilePath, nil
			}
			return "", fmt.Errorf("symbol %s found in package %s at %s by scan, but file does not exist", symbolName, importPath, targetFilePath)
		}
	}

	return "", fmt.Errorf("symbol %s not found in package %s even after scan and cache check", symbolName, importPath)
}
