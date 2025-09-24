package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"go/printer"
	"go/token"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/locator"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

var (
	flagPkg               = flag.String("pkg", "", "package pattern (required)")
	flagIncludeUnexported = flag.Bool("include-unexported", false, "include unexported functions")
	flagShort             = flag.Bool("short", false, "short output")
	flagExpand            = flag.Bool("expand", false, "expand output")
	flagVerbose           = flag.Bool("v", false, "verbose output")
)

func main() {
	flag.Parse()

	if *flagPkg == "" {
		flag.Usage()
		log.Fatal("--pkg flag is required")
	}

	if err := run(context.Background()); err != nil {
		log.Fatalf("toplevel: %+v", err)
	}
}

func run(ctx context.Context) error {
	patterns := []string{*flagPkg}
	if flag.NArg() > 0 {
		patterns = flag.Args()
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	s, err := goscan.New(
		goscan.WithWorkDir(wd),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		return fmt.Errorf("failed to create scanner: %w", err)
	}

	// Resolve patterns to a list of import paths for the analysis scope.
	loc, err := locator.New(wd, locator.WithGoModuleResolver())
	if err != nil {
		return fmt.Errorf("failed to create locator: %w", err)
	}
	analysisPkgs, err := resolveTargetPackages(ctx, []*locator.Locator{loc}, patterns, nil, wd)
	if err != nil {
		return fmt.Errorf("failed to resolve target packages: %w", err)
	}
	analysisPatterns := keys(analysisPkgs)


	graph := NewCallGraph()

	interp, err := symgo.NewInterpreter(s,
		symgo.WithPrimaryAnalysisScope(analysisPatterns...),
	)
	if err != nil {
		return fmt.Errorf("failed to create interpreter: %w", err)
	}

	interp.RegisterDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
		callee, ok := args[0].(*object.Function)
		if !ok {
			return nil
		}

		callerFrame := i.CurrentFrame()
		if callerFrame == nil || callerFrame.Fn == nil {
			return nil
		}
		caller := callerFrame.Fn

		if caller.Def == nil || callee.Def == nil {
			return nil
		}
		if caller.Def == callee.Def {
			return nil
		}
		graph.AddEdge(caller.Def, callee.Def)
		return nil
	})

	// Scan using the raw patterns provided by the user.
	pkgs, err := s.Scan(ctx, patterns...)
	if err != nil {
		return fmt.Errorf("failed to scan packages for entry points: %w", err)
	}

	var entryPoints []*object.Function
	for _, pkg := range pkgs {
		allFunctions := pkg.Functions
		sort.SliceStable(allFunctions, func(i, j int) bool {
			return allFunctions[i].Name < allFunctions[j].Name
		})

		for _, fnInfo := range allFunctions {
			if !*flagIncludeUnexported && !fnInfo.AstDecl.Name.IsExported() {
				continue
			}

			obj, ok := interp.FindObjectInPackage(ctx, pkg.ImportPath, fnInfo.Name)
			if !ok {
				log.Printf("WARN: could not find function object %s in package %s", fnInfo.Name, pkg.ImportPath)
				continue
			}
			fn, ok := obj.(*object.Function)
			if !ok {
				log.Printf("WARN: object %s in package %s is not a function", fnInfo.Name, pkg.ImportPath)
				continue
			}
			entryPoints = append(entryPoints, fn)
		}
	}

	// logger.Info("starting analysis", "entrypoints", len(entryPoints))

	for _, ep := range entryPoints {
		if _, err := interp.Apply(ctx, ep, nil, ep.Package); err != nil {
			// logger.Warn("symbolic execution failed for entry point", "function", ep.Def.Name, "error", err)
		}
	}

	p := &Printer{
		Fset:        s.Fset(),
		Graph:       graph,
		Output:      os.Stdout,
		EntryPoints: entryPoints,
	}
	p.Fprint()

	return nil
}

// resolveTargetPackages (from find-orphans)
func resolveTargetPackages(ctx context.Context, locators []*locator.Locator, patterns []string, excludeDirs []string, rootDir string) (map[string]bool, error) {
	targetPackages := make(map[string]bool)
	excludeMap := make(map[string]bool)
	for _, dir := range excludeDirs {
		excludeMap[dir] = true
	}

	for _, pattern := range patterns {
		isRecursive := strings.HasSuffix(pattern, "/...")
		cleanPattern := strings.TrimSuffix(pattern, "/...")

		isFilePathPattern := strings.HasPrefix(pattern, ".") || filepath.IsAbs(pattern)

		if isFilePathPattern {
			root := filepath.Clean(filepath.Join(rootDir, cleanPattern))
			err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					if excludeMap[d.Name()] || (d.Name() != "." && strings.HasPrefix(d.Name(), ".")) {
						return filepath.SkipDir
					}
					if !isRecursive && path != root {
						return filepath.SkipDir
					}
				}
				if !d.IsDir() && !strings.HasSuffix(d.Name(), ".go") {
					return nil
				}
				dirPath := path
				if !d.IsDir() {
					dirPath = filepath.Dir(path)
				}
				goFiles, err := os.ReadDir(dirPath)
				if err != nil {
					return nil
				}
				hasGo := false
				for _, f := range goFiles {
					if !f.IsDir() && strings.HasSuffix(f.Name(), ".go") {
						hasGo = true
						break
					}
				}
				if !hasGo {
					return nil
				}
				importPath, err := pathToImport(locators, dirPath)
				if err != nil {
					return nil
				}
				if _, exists := targetPackages[importPath]; !exists {
					targetPackages[importPath] = true
				}
				return nil
			})
			if err != nil {
				return nil, fmt.Errorf("failed to walk directory for file path pattern %s: %w", pattern, err)
			}
		} else {
			targetPackages[cleanPattern] = true
		}
	}
	return targetPackages, nil
}

func pathToImport(locators []*locator.Locator, path string) (string, error) {
	for _, loc := range locators {
		importPath, err := loc.PathToImport(path)
		if err == nil {
			return importPath, nil
		}
	}
	return "", fmt.Errorf("path %q does not belong to any known module", path)
}

func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}


type CallGraph struct {
	nodes map[*scanner.FunctionInfo]bool
	edges map[*scanner.FunctionInfo]map[*scanner.FunctionInfo]bool
}

func NewCallGraph() *CallGraph {
	return &CallGraph{
		nodes: make(map[*scanner.FunctionInfo]bool),
		edges: make(map[*scanner.FunctionInfo]map[*scanner.FunctionInfo]bool),
	}
}

func (g *CallGraph) AddEdge(from, to *scanner.FunctionInfo) {
	if from == nil || to == nil {
		return
	}
	g.nodes[from] = true
	g.nodes[to] = true
	if _, ok := g.edges[from]; !ok {
		g.edges[from] = make(map[*scanner.FunctionInfo]bool)
	}
	g.edges[from][to] = true
}

type Printer struct {
	Fset        *token.FileSet
	Graph       *CallGraph
	Output      io.Writer
	EntryPoints []*object.Function
}

func (p *Printer) Fprint() {
	printedPackages := make(map[string]bool)
	for _, epObj := range p.EntryPoints {
		ep := epObj.Def
		if _, ok := p.Graph.edges[ep]; ok {
			pkgPath := epObj.Package.ImportPath
			if !printedPackages[pkgPath] {
				fmt.Fprintf(p.Output, "package %s\n\n", pkgPath)
				printedPackages[pkgPath] = true
			}
			p.printNode(ep, "", make(map[*scanner.FunctionInfo]bool))
			fmt.Fprintln(p.Output)
		}
	}
}

func (p *Printer) printNode(fn *scanner.FunctionInfo, indent string, visited map[*scanner.FunctionInfo]bool) {
	if visited[fn] {
		return
	}
	visited[fn] = true

	fmt.Fprintf(p.Output, "%s  %s\n", indent, formatFunction(p.Fset, fn))

	if callees, ok := p.Graph.edges[fn]; ok {
		sortedCallees := make([]*scanner.FunctionInfo, 0, len(callees))
		for callee := range callees {
			sortedCallees = append(sortedCallees, callee)
		}
		sort.SliceStable(sortedCallees, func(i, j int) bool {
			return sortedCallees[i].Name < sortedCallees[j].Name
		})

		for _, callee := range sortedCallees {
			p.printNode(callee, indent+"  ", visited)
		}
	}

	delete(visited, fn)
}

func formatFunction(fset *token.FileSet, fn *scanner.FunctionInfo) string {
	var b bytes.Buffer
	b.WriteString("func ")
	if fn.Receiver != nil {
		b.WriteString("(")
		printer.Fprint(&b, fset, fn.AstDecl.Recv.List[0].Type)
		b.WriteString(")")
		b.WriteString(".")
	}
	b.WriteString(fn.Name)
	printer.Fprint(&b, fset, fn.AstDecl.Type)

	return strings.ReplaceAll(b.String(), "\n", " ")
}