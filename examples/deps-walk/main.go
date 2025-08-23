package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	goscan "github.com/podhmo/go-scan"
)

// logLevelVar is a custom flag.Value implementation for slog.LevelVar
type logLevelVar struct {
	levelVar *slog.LevelVar
}

func (v *logLevelVar) String() string {
	if v.levelVar == nil {
		return ""
	}
	return v.levelVar.Level().String()
}

func (v *logLevelVar) Set(s string) error {
	var level slog.Level
	switch strings.ToLower(s) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return fmt.Errorf("unknown log level: %s", s)
	}
	v.levelVar.Set(level)
	return nil
}

func main() {
	var (
		hops        int
		ignore      string
		hide        string
		output      string
		format      string
		granularity string
		full        bool
		short       bool
		direction   string
		aggressive  bool
		test        bool
		dryRun      bool
		inspect     bool
		logLevel    = new(slog.LevelVar)
	)

	// No -start-pkg flag, positional arguments are used instead
	flag.IntVar(&hops, "hops", 1, "Maximum number of hops to walk from the start package")
	flag.StringVar(&ignore, "ignore", "", "A comma-separated list of package patterns to ignore")
	flag.StringVar(&hide, "hide", "", "A comma-separated list of package patterns to hide from the output")
	flag.StringVar(&output, "output", "", "Output file path for the graph (defaults to stdout)")
	flag.StringVar(&format, "format", "dot", "Output format (dot, mermaid, or json)")
	flag.StringVar(&granularity, "granularity", "package", "Dependency granularity (package or file)")
	flag.BoolVar(&full, "full", false, "Include dependencies outside the current module")
	flag.BoolVar(&short, "short", false, "Omit module prefix from package paths in the output")
	flag.StringVar(&direction, "direction", "forward", "Direction of dependency walk (forward, reverse, bidi)")
	flag.BoolVar(&aggressive, "aggressive", false, "Use aggressive git-grep based search for reverse mode")
	flag.BoolVar(&test, "test", false, "Include test files in the analysis")
	flag.BoolVar(&dryRun, "dry-run", false, "don't write to output file, just print to stdout")
	flag.BoolVar(&inspect, "inspect", false, "enable inspection logging")
	flag.Var(&logLevelVar{levelVar: logLevel}, "log-level", "set log level (debug, info, warn, error)")
	flag.Parse()

	startPkgs := flag.Args()
	if len(startPkgs) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	opts := slog.HandlerOptions{Level: logLevel}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &opts))
	slog.SetDefault(logger)

	if err := run(context.Background(), startPkgs, hops, ignore, hide, output, format, granularity, full, short, direction, aggressive, test, dryRun, inspect, logger); err != nil {
		slog.ErrorContext(context.Background(), "Error", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(ctx context.Context, startPkgs []string, hops int, ignore string, hide string, output string, format string, granularity string, full bool, short bool, direction string, aggressive bool, test bool, dryRun bool, inspect bool, logger *slog.Logger) error {
	var finalOutput bytes.Buffer

	var scannerOpts []goscan.ScannerOption
	if full {
		scannerOpts = append(scannerOpts, goscan.WithGoModuleResolver())
	}
	scannerOpts = append(scannerOpts, goscan.WithIncludeTests(test))
	scannerOpts = append(scannerOpts, goscan.WithDryRun(dryRun))
	scannerOpts = append(scannerOpts, goscan.WithInspect(inspect))
	scannerOpts = append(scannerOpts, goscan.WithLogger(logger))

	s, err := goscan.New(scannerOpts...)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	for i, startPkg := range startPkgs {
		// Use the facade function from the root goscan package
		resolvedStartPkg, err := goscan.ResolvePath(ctx, startPkg)
		if err != nil {
			return fmt.Errorf("failed to resolve start package path for %q: %w", startPkg, err)
		}
		startPkg = resolvedStartPkg

		ignorePatterns := []string{}
		if ignore != "" {
			ignorePatterns = strings.Split(ignore, ",")
		}
		hidePatterns := []string{}
		if hide != "" {
			hidePatterns = strings.Split(hide, ",")
		}

		visitor := &graphVisitor{
			startPkg:            startPkg,
			s:                   s,
			hops:                hops,
			full:                full,
			short:               short,
			granularity:         granularity,
			ignorePatterns:      ignorePatterns,
			hidePatterns:        hidePatterns,
			dependencies:        make(map[string][]string),
			reverseDependencies: make(map[string][]string),
			packageHops:         make(map[string]int),
		}

		if aggressive && !(direction == "reverse" || direction == "bidi") {
			return fmt.Errorf("--aggressive is only valid with --direction=reverse or --direction=bidi")
		}
		if granularity == "file" && (direction == "reverse" || direction == "bidi") {
			return fmt.Errorf("--granularity=file is not compatible with --direction=reverse or --direction=bidi")
		}

		doForwardSearch := func() error {
			visitor.packageHops[startPkg] = 0
			return s.Walker.Walk(ctx, visitor, startPkg)
		}

		doReverseSearch := func() error {
			if aggressive {
				// Aggressive search using git grep
				queue := []string{startPkg}
				pkgHops := map[string]int{startPkg: 0}
				head := 0
				for head < len(queue) {
					currentPkg := queue[head]
					head++

					currentHops := pkgHops[currentPkg]
					if currentHops >= hops {
						continue
					}

					importers, err := s.Walker.FindImportersAggressively(ctx, currentPkg)
					if err != nil {
						return fmt.Errorf("aggressive search for importers of %s failed: %w", currentPkg, err)
					}

					for _, importer := range importers {
						visitor.reverseDependencies[importer.ImportPath] = append(visitor.reverseDependencies[importer.ImportPath], currentPkg)
						if _, visited := pkgHops[importer.ImportPath]; !visited {
							pkgHops[importer.ImportPath] = currentHops + 1
							queue = append(queue, importer.ImportPath)
						}
					}
				}
				return nil
			}

			// Default search using pre-built map
			revDepMap, err := s.Walker.BuildReverseDependencyMap(ctx)
			if err != nil {
				return fmt.Errorf("could not build reverse dependency map: %w", err)
			}

			queue := []string{startPkg}
			pkgHops := map[string]int{startPkg: 0}
			head := 0
			for head < len(queue) {
				currentPkg := queue[head]
				head++

				currentHops := pkgHops[currentPkg]
				if currentHops >= hops {
					continue
				}

				importers := revDepMap[currentPkg]
				for _, importer := range importers {
					visitor.reverseDependencies[importer] = append(visitor.reverseDependencies[importer], currentPkg)
					if _, visited := pkgHops[importer]; !visited {
						pkgHops[importer] = currentHops + 1
						queue = append(queue, importer)
					}
				}
			}
			return nil
		}

		switch direction {
		case "forward":
			if err := doForwardSearch(); err != nil {
				return fmt.Errorf("walk failed for %q: %w", startPkg, err)
			}
		case "reverse":
			if err := doReverseSearch(); err != nil {
				return fmt.Errorf("find importers failed for %q: %w", startPkg, err)
			}
		case "bidi":
			if err := doForwardSearch(); err != nil {
				return fmt.Errorf("bidi walk (forward part) failed for %q: %w", startPkg, err)
			}
			if err := doReverseSearch(); err != nil {
				return fmt.Errorf("bidi walk (reverse part) failed for %q: %w", startPkg, err)
			}
		default:
			return fmt.Errorf("invalid direction: %q. must be one of forward, reverse, or bidi", direction)
		}

		var buf bytes.Buffer
		switch format {
		case "dot":
			if err := visitor.WriteDOT(&buf); err != nil {
				return fmt.Errorf("failed to generate DOT graph for %q: %w", startPkg, err)
			}
		case "mermaid":
			if err := visitor.WriteMermaid(&buf); err != nil {
				return fmt.Errorf("failed to generate Mermaid graph for %q: %w", startPkg, err)
			}
		case "json":
			if err := visitor.WriteJSON(&buf, startPkg, direction); err != nil {
				return fmt.Errorf("failed to generate JSON output for %q: %w", startPkg, err)
			}
		default:
			return fmt.Errorf("unsupported format: %q", format)
		}

		finalOutput.Write(buf.Bytes())
		if i < len(startPkgs)-1 {
			finalOutput.WriteString("\n\n")
		}
	}

	if output == "" || dryRun {
		if dryRun && output != "" {
			slog.InfoContext(ctx, "Dry run: skipping file write", "path", output)
		}
		_, err = os.Stdout.Write(finalOutput.Bytes())
		if err != nil {
			return fmt.Errorf("writing to stdout: %w", err)
		}
		return nil
	}
	return os.WriteFile(output, finalOutput.Bytes(), 0644)
}

type graphVisitor struct {
	startPkg            string
	s                   *goscan.Scanner
	hops                int
	full                bool
	short               bool
	granularity         string
	ignorePatterns      []string
	hidePatterns        []string
	dependencies        map[string][]string // from -> to[]
	reverseDependencies map[string][]string // to -> from[]
	packageHops         map[string]int      // package -> hop level
}

func (v *graphVisitor) Visit(pkg *goscan.PackageImports) ([]string, error) {
	currentHop, exists := v.packageHops[pkg.ImportPath]
	if !exists {
		return nil, fmt.Errorf("internal error: visiting package %q with no hop level assigned", pkg.ImportPath)
	}

	if currentHop >= v.hops {
		return nil, nil // Stop traversal from this node
	}

	var importsToFollow []string
	modulePath := v.s.ModulePath()

	var importsToProcess map[string][]string
	if v.granularity == "file" {
		importsToProcess = pkg.FileImports
	} else {
		importsToProcess = map[string][]string{pkg.ImportPath: pkg.Imports}
	}

	for source, imps := range importsToProcess {
		for _, imp := range imps {
			isIgnored := false
			for _, pattern := range v.ignorePatterns {
				// Check against full import path
				if matched, _ := filepath.Match(pattern, imp); matched {
					isIgnored = true
					break
				}

				// If short is enabled, also check against the short package path
				if v.short {
					modulePath := v.s.ModulePath()
					if modulePath != "" && strings.HasPrefix(imp, modulePath) {
						shortImp := strings.TrimPrefix(imp, modulePath)
						shortImp = strings.TrimPrefix(shortImp, "/")
						if matched, _ := filepath.Match(pattern, shortImp); matched {
							isIgnored = true
							break
						}
					}
				}
			}
			if isIgnored {
				continue
			}

			isInternalModule := modulePath != "" && strings.HasPrefix(imp, modulePath)

			// If not in full mode, we skip any dependency that is not part of the current module.
			if !v.full && !isInternalModule {
				continue
			}

			// Add the dependency to the graph data.
			v.dependencies[source] = append(v.dependencies[source], imp)

			// Add the dependency to the queue for the next level of the walk if it hasn't been visited.
			if _, visited := v.packageHops[imp]; !visited {
				v.packageHops[imp] = currentHop + 1
				importsToFollow = append(importsToFollow, imp)
			}
		}
	}

	return importsToFollow, nil
}

func (v *graphVisitor) isHidden(nodePath string) bool {
	for _, pattern := range v.hidePatterns {
		// Check against full import path
		if matched, _ := filepath.Match(pattern, nodePath); matched {
			return true
		}

		// If short is enabled, also check against the short package path
		if v.short {
			modulePath := v.s.ModulePath()
			if modulePath != "" && strings.HasPrefix(nodePath, modulePath) {
				shortPath := strings.TrimPrefix(nodePath, modulePath)
				shortPath = strings.TrimPrefix(shortPath, "/")
				if matched, _ := filepath.Match(pattern, shortPath); matched {
					return true
				}
			}
		}
	}
	return false
}

func (v *graphVisitor) WriteDOT(w io.Writer) error {
	fmt.Fprintln(w, "digraph dependencies {")
	fmt.Fprintln(w, `  rankdir="LR";`)
	if v.granularity == "package" {
		fmt.Fprintln(w, `  node [shape=box, style="rounded,filled", fillcolor=lightgrey];`)
	}

	allNodes := make(map[string]string) // name -> type ("file" or "package")

	// Collect all nodes from both dependency maps
	for from, toList := range v.dependencies {
		if v.granularity == "file" {
			allNodes[from] = "file"
		} else {
			allNodes[from] = "package"
		}
		for _, to := range toList {
			allNodes[to] = "package"
		}
	}
	for from, toList := range v.reverseDependencies {
		allNodes[from] = "package"
		for _, to := range toList {
			allNodes[to] = "package"
		}
	}

	sortedNodes := make([]string, 0, len(allNodes))
	for node := range allNodes {
		sortedNodes = append(sortedNodes, node)
	}
	sort.Strings(sortedNodes)

	modulePath := v.s.ModulePath()
	moduleRootDir := v.s.RootDir()

	for _, node := range sortedNodes {
		if v.isHidden(node) {
			continue
		}
		label := node
		if v.granularity == "file" {
			relPath, err := filepath.Rel(moduleRootDir, node)
			if err == nil {
				label = relPath
			}
		} else if v.short && modulePath != "" && strings.HasPrefix(node, modulePath) {
			label = strings.TrimPrefix(node, modulePath)
			label = strings.TrimPrefix(label, "/")
		}

		// Highlight start node
		if node == v.startPkg {
			var attributes string
			switch allNodes[node] {
			case "file":
				attributes = `shape=note, style="filled", fillcolor=lightblue`
			case "package":
				attributes = `shape=box, style="rounded,filled", fillcolor=lightblue`
			}
			fmt.Fprintf(w, `  "%s" [label="%s", %s];`+"\n", node, label, attributes)
			continue
		}

		switch allNodes[node] {
		case "file":
			fmt.Fprintf(w, `  "%s" [label="%s", shape=note, style=filled, fillcolor=khaki];`+"\n", node, label)
		case "package":
			if v.granularity == "package" {
				fmt.Fprintf(w, `  "%s" [label="%s"];`+"\n", node, label)
			} else {
				fmt.Fprintf(w, `  "%s" [label="%s", shape=box, style="rounded,filled", fillcolor=lightgrey];`+"\n", node, label)
			}
		}
	}

	fmt.Fprintln(w, "")

	// Write forward dependencies
	sortedFroms := make([]string, 0, len(v.dependencies))
	for from := range v.dependencies {
		sortedFroms = append(sortedFroms, from)
	}
	sort.Strings(sortedFroms)

	for _, from := range sortedFroms {
		toList := v.dependencies[from]
		sort.Strings(toList)
		for _, to := range toList {
			if v.isHidden(from) || v.isHidden(to) {
				continue
			}
			fmt.Fprintf(w, `  "%s" -> "%s";`+"\n", from, to)
		}
	}

	// Write reverse dependencies with a different style
	sortedFromsRev := make([]string, 0, len(v.reverseDependencies))
	for from := range v.reverseDependencies {
		sortedFromsRev = append(sortedFromsRev, from)
	}
	sort.Strings(sortedFromsRev)

	for _, from := range sortedFromsRev {
		toList := v.reverseDependencies[from]
		sort.Strings(toList)
		for _, to := range toList {
			if v.isHidden(from) || v.isHidden(to) {
				continue
			}
			// Using [dir=back] to indicate a reverse dependency
			fmt.Fprintf(w, `  "%s" -> "%s" [dir=back, style=dashed];`+"\n", to, from)
		}
	}

	fmt.Fprintln(w, "}")
	return nil
}

func (v *graphVisitor) WriteMermaid(w io.Writer) error {
	fmt.Fprintln(w, "graph LR")

	allNodes := make(map[string]string) // name -> type ("file" or "package")
	for from, toList := range v.dependencies {
		if v.granularity == "file" {
			allNodes[from] = "file"
		} else {
			allNodes[from] = "package"
		}
		for _, to := range toList {
			allNodes[to] = "package"
		}
	}
	for from, toList := range v.reverseDependencies {
		allNodes[from] = "package"
		for _, to := range toList {
			allNodes[to] = "package"
		}
	}

	sortedNodes := make([]string, 0, len(allNodes))
	for node := range allNodes {
		sortedNodes = append(sortedNodes, node)
	}
	sort.Strings(sortedNodes)

	nodeIDs := make(map[string]string)
	for i, node := range sortedNodes {
		nodeIDs[node] = fmt.Sprintf("id%d", i)
	}

	modulePath := v.s.ModulePath()
	moduleRootDir := v.s.RootDir()
	indent := "  "

	if v.short && modulePath != "" && v.granularity == "package" {
		fmt.Fprintf(w, "\n  subgraph module [%s]\n", modulePath)
		indent = "    "
	}

	fmt.Fprintln(w, "")

	for _, node := range sortedNodes {
		if v.isHidden(node) {
			continue
		}
		id := nodeIDs[node]
		label := node
		if v.granularity == "file" {
			relPath, err := filepath.Rel(moduleRootDir, node)
			if err == nil {
				label = relPath
			}
		} else if v.short && modulePath != "" && strings.HasPrefix(node, modulePath) {
			label = strings.TrimPrefix(node, modulePath)
			label = strings.TrimPrefix(label, "/")
		}

		switch allNodes[node] {
		case "file":
			fmt.Fprintf(w, `%s%s("%s")`+"\n", indent, id, label)
		case "package":
			fmt.Fprintf(w, `%s%s["%s"]`+"\n", indent, id, label)
		}
	}

	fmt.Fprintln(w, "")

	// Write forward dependencies
	sortedFroms := make([]string, 0, len(v.dependencies))
	for from := range v.dependencies {
		sortedFroms = append(sortedFroms, from)
	}
	sort.Strings(sortedFroms)

	for _, from := range sortedFroms {
		toList := v.dependencies[from]
		sort.Strings(toList)
		fromID := nodeIDs[from]
		for _, to := range toList {
			if v.isHidden(from) || v.isHidden(to) {
				continue
			}
			toID, ok := nodeIDs[to]
			if !ok {
				continue
			}
			fmt.Fprintf(w, "%s%s --> %s\n", indent, fromID, toID)
		}
	}

	// Write reverse dependencies
	sortedFromsRev := make([]string, 0, len(v.reverseDependencies))
	for from := range v.reverseDependencies {
		sortedFromsRev = append(sortedFromsRev, from)
	}
	sort.Strings(sortedFromsRev)

	for _, from := range sortedFromsRev {
		toList := v.reverseDependencies[from]
		sort.Strings(toList)
		fromID := nodeIDs[from]
		for _, to := range toList {
			if v.isHidden(from) || v.isHidden(to) {
				continue
			}
			toID, ok := nodeIDs[to]
			if !ok {
				continue
			}
			// Using a dashed line for reverse dependencies
			fmt.Fprintf(w, "%s%s -.-> %s\n", indent, fromID, toID)
		}
	}

	if v.short && modulePath != "" && v.granularity == "package" {
		fmt.Fprintln(w, "  end")
	}

	// Add styling for the start package
	if startNodeID, ok := nodeIDs[v.startPkg]; ok {
		fmt.Fprintf(w, "\n%sstyle %s fill:#add8e6,stroke:#333,stroke-width:2px\n", indent, startNodeID)
	}

	return nil
}

func (v *graphVisitor) WriteJSON(w io.Writer, startPkg, direction string) error {
	type jsonOutput struct {
		Config              map[string]interface{} `json:"config"`
		Dependencies        map[string][]string    `json:"dependencies"`
		ReverseDependencies map[string][]string    `json:"reverseDependencies"`
	}

	sortMap := func(m map[string][]string) map[string][]string {
		sortedMap := make(map[string][]string, len(m))
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sort.Strings(m[k])
			sortedMap[k] = m[k]
		}
		return sortedMap
	}

	// Filter hidden nodes
	filteredDeps := make(map[string][]string)
	for from, toList := range v.dependencies {
		if v.isHidden(from) {
			continue
		}
		var filteredToList []string
		for _, to := range toList {
			if !v.isHidden(to) {
				filteredToList = append(filteredToList, to)
			}
		}
		if len(filteredToList) > 0 {
			filteredDeps[from] = filteredToList
		}
	}

	filteredRevDeps := make(map[string][]string)
	for from, toList := range v.reverseDependencies {
		if v.isHidden(from) {
			continue
		}
		var filteredToList []string
		for _, to := range toList {
			if !v.isHidden(to) {
				filteredToList = append(filteredToList, to)
			}
		}
		if len(filteredToList) > 0 {
			filteredRevDeps[from] = filteredToList
		}
	}

	output := jsonOutput{
		Config: map[string]interface{}{
			"startPkg":  startPkg,
			"direction": direction,
			"hops":      v.hops,
		},
		Dependencies:        sortMap(filteredDeps),
		ReverseDependencies: sortMap(filteredRevDeps),
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("failed to encode dependencies to JSON: %w", err)
	}
	return nil
}
