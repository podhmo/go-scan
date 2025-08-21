package resolver

import (
	"context"
	"sync"

	goscan "github.com/podhmo/go-scan"
)

// scannerService defines the interface for the underlying scanning mechanism.
// This allows for easier testing by swapping out the real scanner with a mock.
type scannerService interface {
	ScanPackageByImport(ctx context.Context, path string) (*goscan.Package, error)
}

// Resolver provides a cached, on-demand mechanism for loading and scanning Go packages.
// It is safe for concurrent use.
type Resolver struct {
	scanner scannerService
	mu      sync.RWMutex
	cache   map[string]*goscan.Package
}

// New creates a new Resolver instance.
// It requires a scanner service, which is typically a *goscan.Scanner.
func New(scanner scannerService) *Resolver {
	return &Resolver{
		scanner: scanner,
		cache:   make(map[string]*goscan.Package),
	}
}

// Resolve scans a package by its import path and returns its information.
// It uses an internal cache to avoid re-scanning packages.
func (r *Resolver) Resolve(ctx context.Context, path string) (*goscan.Package, error) {
	// Check cache with a read lock first for performance.
	r.mu.RLock()
	pkgInfo, found := r.cache[path]
	r.mu.RUnlock()

	if found {
		return pkgInfo, nil
	}

	// If not found, acquire a write lock to perform the scan.
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check the cache after acquiring the write lock, in case another
	// goroutine populated it while we were waiting.
	if pkgInfo, found := r.cache[path]; found {
		return pkgInfo, nil
	}

	// Scan the package.
	pkgInfo, err := r.scanner.ScanPackageByImport(ctx, path)
	if err != nil {
		return nil, err
	}

	// Store in cache.
	r.cache[path] = pkgInfo

	return pkgInfo, nil
}
