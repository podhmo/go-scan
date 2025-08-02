package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	goscan "github.com/podhmo/go-scan"
)

func main() {
	var (
		startPkg string
		hops     int
		ignore   string
		output   string
		full     bool
		short    bool
	)

	flag.StringVar(&startPkg, "start-pkg", "", "The root package to start the dependency walk from (required)")
	flag.IntVar(&hops, "hops", 1, "Maximum number of hops to walk from the start package")
	flag.StringVar(&ignore, "ignore", "", "A comma-separated list of package patterns to ignore")
	flag.StringVar(&output, "output", "", "Output file path for the DOT graph (defaults to stdout)")
	flag.BoolVar(&full, "full", false, "Include dependencies outside the current module")
	flag.BoolVar(&short, "short", false, "Omit module prefix from package paths in the output")
	flag.Parse()

	if startPkg == "" {
		log.Fatal("-start-pkg is required")
	}

	if err := run(context.Background(), startPkg, hops, ignore, output, full, short); err != nil {
		log.Fatalf("Error: %+v", err)
	}
}

func run(ctx context.Context, startPkg string, hops int, ignore string, output string, full bool, short bool) error {
	s, err := goscan.New()
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	ignorePatterns := []string{}
	if ignore != "" {
		ignorePatterns = strings.Split(ignore, ",")
	}

	visitor := &graphVisitor{
		s:              s,
		hops:           hops,
		full:           full,
		short:          short,
		ignorePatterns: ignorePatterns,
		dependencies:   make(map[string][]string),
		packageHops:    make(map[string]int),
	}

	// Set the starting hop level for the root package
	visitor.packageHops[startPkg] = 0

	if err := s.Walk(ctx, startPkg, visitor); err != nil {
		return fmt.Errorf("walk failed: %w", err)
	}

	var buf bytes.Buffer
	if err := visitor.WriteDOT(&buf); err != nil {
		return fmt.Errorf("failed to generate DOT graph: %w", err)
	}

	if output == "" {
		_, err = os.Stdout.Write(buf.Bytes())
		if err != nil {
			return fmt.Errorf("writing to stdout: %w", err)
		}
		return nil
	}
	return os.WriteFile(output, buf.Bytes(), 0644)
}

type graphVisitor struct {
	s              *goscan.Scanner
	hops           int
	full           bool
	short          bool
	ignorePatterns []string
	dependencies   map[string][]string // from -> to[]
	packageHops    map[string]int      // package -> hop level
}

func (v *graphVisitor) Visit(pkg *goscan.PackageImports) ([]string, error) {
	currentHop, exists := v.packageHops[pkg.ImportPath]
	if !exists {
		// This can happen for packages that are imported but not yet visited via the queue.
		// It's a dependency of a package we are visiting.
		// We should not proceed if it's an externally discovered package beyond hop 0.
		// However, the Walk function's design means this will be a package from the queue.
		// If it's not in packageHops, it means it's a new package, and its hop level should have been set when it was added to the queue.
		// The logic below handles setting the hop for newly discovered packages.
		// Let's be safe and assume if it's not in the map, we can't determine its hop count, so we stop.
		return nil, fmt.Errorf("internal error: visiting package %q with no hop level assigned", pkg.ImportPath)
	}

	if currentHop >= v.hops {
		return nil, nil // Stop traversal from this node
	}

	var importsToFollow []string
	modulePath := v.s.ModulePath()

	for _, imp := range pkg.Imports {
		// Check ignore patterns
		isIgnored := false
		for _, pattern := range v.ignorePatterns {
			if matched, _ := filepath.Match(pattern, imp); matched {
				isIgnored = true
				break
			}
		}
		if isIgnored {
			continue
		}

		// Record the dependency, regardless of whether we follow it or not
		v.dependencies[pkg.ImportPath] = append(v.dependencies[pkg.ImportPath], imp)

		// Check if we should follow this import
		isInternal := modulePath != "" && strings.HasPrefix(imp, modulePath)
		if !v.full && !isInternal {
			continue // Skip external dependencies if not in full mode
		}

		if _, visited := v.packageHops[imp]; !visited {
			v.packageHops[imp] = currentHop + 1
			importsToFollow = append(importsToFollow, imp)
		}
	}
	return importsToFollow, nil
}

func (v *graphVisitor) WriteDOT(w io.Writer) error {
	fmt.Fprintln(w, "digraph dependencies {")
	fmt.Fprintln(w, `  rankdir="LR";`)
	fmt.Fprintln(w, `  node [shape=box, style="rounded,filled", fillcolor=lightgrey];`)

	// Collect all unique packages to declare them as nodes
	allPackagesSet := make(map[string]struct{})
	for from, toList := range v.dependencies {
		allPackagesSet[from] = struct{}{}
		for _, to := range toList {
			allPackagesSet[to] = struct{}{}
		}
	}

	// Sort packages for deterministic output
	sortedPackages := make([]string, 0, len(allPackagesSet))
	for pkg := range allPackagesSet {
		sortedPackages = append(sortedPackages, pkg)
	}
	sort.Strings(sortedPackages)

	modulePath := v.s.ModulePath()

	// Declare all nodes with their import paths as labels
	for _, pkg := range sortedPackages {
		label := pkg
		if v.short && modulePath != "" && strings.HasPrefix(pkg, modulePath) {
			label = strings.TrimPrefix(pkg, modulePath)
			label = strings.TrimPrefix(label, "/")
		}
		fmt.Fprintf(w, `  "%s" [label="%s"];`+"\n", pkg, label)
	}

	fmt.Fprintln(w, "") // separator

	// Sort dependencies for deterministic output
	sortedFroms := make([]string, 0, len(v.dependencies))
	for from := range v.dependencies {
		sortedFroms = append(sortedFroms, from)
	}
	sort.Strings(sortedFroms)

	// Define all edges
	for _, from := range sortedFroms {
		toList := v.dependencies[from]
		sort.Strings(toList) // Sort the 'to' packages as well
		for _, to := range toList {
			fmt.Fprintf(w, `  "%s" -> "%s";`+"\n", from, to)
		}
	}

	fmt.Fprintln(w, "}")
	return nil
}
