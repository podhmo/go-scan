package main

import (
	"bytes"
	"context"
	"encoding/json"
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
		startPkg    string
		hops        int
		ignore      string
		output      string
		format      string
		granularity string
		full        bool
		short       bool
		direction   string
		aggressive  bool
	)

	flag.StringVar(&startPkg, "start-pkg", "", "The root package to start the dependency walk from (required)")
	flag.IntVar(&hops, "hops", 1, "Maximum number of hops to walk from the start package")
	flag.StringVar(&ignore, "ignore", "", "A comma-separated list of package patterns to ignore")
	flag.StringVar(&output, "output", "", "Output file path for the graph (defaults to stdout)")
	flag.StringVar(&format, "format", "dot", "Output format (dot, mermaid, or json)")
	flag.StringVar(&granularity, "granularity", "package", "Dependency granularity (package or file)")
	flag.BoolVar(&full, "full", false, "Include dependencies outside the current module")
	flag.BoolVar(&short, "short", false, "Omit module prefix from package paths in the output")
	flag.StringVar(&direction, "direction", "forward", "Direction of dependency walk (forward, reverse, bidi)")
	flag.BoolVar(&aggressive, "aggressive", false, "Use aggressive git-grep based search for reverse mode")
	flag.Parse()

	if startPkg == "" {
		log.Fatal("-start-pkg is required")
	}

	if err := run(context.Background(), startPkg, hops, ignore, output, format, granularity, full, short, direction, aggressive); err != nil {
		log.Fatalf("Error: %+v", err)
	}
}

func run(ctx context.Context, startPkg string, hops int, ignore string, output string, format string, granularity string, full bool, short bool, direction string, aggressive bool) error {
	s, err := goscan.New()
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	ignorePatterns := []string{}
	if ignore != "" {
		ignorePatterns = strings.Split(ignore, ",")
	}

	visitor := &graphVisitor{
		s:                   s,
		hops:                hops,
		full:                full,
		short:               short,
		granularity:         granularity,
		ignorePatterns:      ignorePatterns,
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
		return s.Walk(ctx, startPkg, visitor)
	}

	doReverseSearch := func() error {
		var importers []*goscan.PackageImports
		var err error
		if aggressive {
			importers, err = s.FindImportersAggressively(ctx, startPkg)
		} else {
			importers, err = s.FindImporters(ctx, startPkg)
		}
		if err != nil {
			return err
		}
		for _, imp := range importers {
			visitor.reverseDependencies[imp.ImportPath] = append(visitor.reverseDependencies[imp.ImportPath], startPkg)
		}
		return nil
	}

	switch direction {
	case "forward":
		if err := doForwardSearch(); err != nil {
			return fmt.Errorf("walk failed: %w", err)
		}
	case "reverse":
		if err := doReverseSearch(); err != nil {
			return fmt.Errorf("find importers failed: %w", err)
		}
	case "bidi":
		if err := doForwardSearch(); err != nil {
			return fmt.Errorf("bidi walk (forward part) failed: %w", err)
		}
		if err := doReverseSearch(); err != nil {
			return fmt.Errorf("bidi walk (reverse part) failed: %w", err)
		}
	default:
		return fmt.Errorf("invalid direction: %q. must be one of forward, reverse, or bidi", direction)
	}

	var buf bytes.Buffer
	switch format {
	case "dot":
		if err := visitor.WriteDOT(&buf); err != nil {
			return fmt.Errorf("failed to generate DOT graph: %w", err)
		}
	case "mermaid":
		if err := visitor.WriteMermaid(&buf); err != nil {
			return fmt.Errorf("failed to generate Mermaid graph: %w", err)
		}
	case "json":
		if err := visitor.WriteJSON(&buf, startPkg, direction); err != nil {
			return fmt.Errorf("failed to generate JSON output: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %q", format)
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
	s                   *goscan.Scanner
	hops                int
	full                bool
	short               bool
	granularity         string
	ignorePatterns      []string
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
				if matched, _ := filepath.Match(pattern, imp); matched {
					isIgnored = true
					break
				}
			}
			if isIgnored {
				continue
			}

			v.dependencies[source] = append(v.dependencies[source], imp)

			isInternal := modulePath != "" && strings.HasPrefix(imp, modulePath)
			if !v.full && !isInternal {
				continue
			}

			if _, visited := v.packageHops[imp]; !visited {
				v.packageHops[imp] = currentHop + 1
				importsToFollow = append(importsToFollow, imp)
			}
		}
	}

	return importsToFollow, nil
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

	output := jsonOutput{
		Config: map[string]interface{}{
			"startPkg":  startPkg,
			"direction": direction,
			"hops":      v.hops,
		},
		Dependencies:        sortMap(v.dependencies),
		ReverseDependencies: sortMap(v.reverseDependencies),
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("failed to encode dependencies to JSON: %w", err)
	}
	return nil
}
