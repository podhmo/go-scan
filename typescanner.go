package typescanner

import (
	"fmt"
	"os"
	"sync"

	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
)

// Re-export scanner kinds for convenience.
const (
	StructKind = scanner.StructKind
	AliasKind  = scanner.AliasKind
	FuncKind   = scanner.FuncKind
)

// Scanner is the main entry point for the type scanning library.
// It combines a locator for finding packages and a scanner for parsing them.
type Scanner struct {
	locator      *locator.Locator
	scanner      *scanner.Scanner
	packageCache map[string]*scanner.PackageInfo
	mu           sync.RWMutex
}

// New creates a new Scanner. It finds the module root starting from the given path.
func New(startPath string) (*Scanner, error) {
	loc, err := locator.New(startPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize locator: %w", err)
	}

	return &Scanner{
		locator:      loc,
		scanner:      scanner.New(),
		packageCache: make(map[string]*scanner.PackageInfo),
	}, nil
}

// ScanPackage scans a single package at a given directory path.
// The path should be relative to the project root or an absolute path.
func (s *Scanner) ScanPackage(pkgPath string) (*scanner.PackageInfo, error) {
	info, err := os.Stat(pkgPath)
	if err != nil {
		return nil, fmt.Errorf("could not stat path %s: %w", pkgPath, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path %s is not a directory", pkgPath)
	}

	return s.scanner.ScanPackage(pkgPath, s)
}

// ScanPackageByImport scans a single package using its Go import path.
// It uses a cache to avoid re-scanning the same package multiple times.
func (s *Scanner) ScanPackageByImport(importPath string) (*scanner.PackageInfo, error) {
	// Check cache first
	s.mu.RLock()
	cachedPkg, found := s.packageCache[importPath]
	s.mu.RUnlock()
	if found {
		return cachedPkg, nil
	}

	// If not in cache, find directory and scan
	dirPath, err := s.locator.FindPackageDir(importPath)
	if err != nil {
		return nil, fmt.Errorf("could not find directory for import path %s: %w", importPath, err)
	}

	pkgInfo, err := s.ScanPackage(dirPath)
	if err != nil {
		return nil, err
	}

	// Store in cache
	s.mu.Lock()
	s.packageCache[importPath] = pkgInfo
	s.mu.Unlock()

	return pkgInfo, nil
}