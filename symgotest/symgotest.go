package symgotest

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

// Run executes a symgo test case. It handles all setup and teardown.
// Its behavior with respect to interpreter errors depends on the `ExpectError` flag in the TestCase.
func Run(t *testing.T, tc TestCase, action func(t *testing.T, r *Result)) {
	t.Helper()
	res := runLogic(t, tc)

	// If an unexpected error occurred, fail fatally.
	if res.Error != nil && !tc.ExpectError {
		if res.Trace != nil {
			t.Fatalf("symgotest: test failed unexpectedly: %v\n\n%s", res.Error, res.Trace.Format())
		}
		t.Fatalf("symgotest: test failed unexpectedly: %v", res.Error)
	}

	// If an error was expected but none occurred, fail fatally.
	if res.Error == nil && tc.ExpectError {
		t.Fatalf("symgotest: expected an error, but test completed successfully")
	}

	// Otherwise, proceed to the action function for user assertions.
	action(t, res)
}

// runLogic contains the core logic of a test run. It always returns a Result,
// with any errors (setup or runtime) populated in the Result.Error field.
func runLogic(t *testing.T, tc TestCase) *Result {
	// 1. Setup test environment
	dir, cleanup := scantest.WriteFiles(t, tc.Source)
	defer cleanup()

	workDir := dir
	if tc.WorkDir != "" {
		workDir = filepath.Join(dir, tc.WorkDir)
	}

	// 2. Create scanner and interpreter
	cfg := &config{
		Timeout:  5 * time.Second, // Default timeout
		MaxSteps: 10000,           // Default max steps
	}
	for _, opt := range tc.Options {
		opt(cfg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	scanner, err := goscan.New(
		goscan.WithWorkDir(workDir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		return &Result{Error: &object.Error{Message: fmt.Sprintf("failed to create scanner: %v", err)}}
	}

	// Use provided tracer or default to internal one.
	var tracer object.Tracer
	var internalTracer *ExecutionTracer
	if cfg.Tracer != nil {
		tracer = cfg.Tracer
	} else {
		internalTracer = NewExecutionTracer(scanner.Fset())
		tracer = internalTracer
	}
	res := &Result{Trace: internalTracer} // Initialize result early

	pkgPath, fnName := splitQualifiedName(tc.EntryPoint)
	if pkgPath == "" {
		res.Error = &object.Error{Message: fmt.Sprintf("invalid entry point format: %q. Expected 'path/to/package.FunctionName'", tc.EntryPoint)}
		return res
	}

	interpreterOpts := []symgo.Option{
		symgo.WithLogger(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))),
		symgo.WithTracer(tracer),
		symgo.WithMaxSteps(cfg.MaxSteps),
	}
	if cfg.ScanPolicy != nil {
		interpreterOpts = append(interpreterOpts, symgo.WithScanPolicy(cfg.ScanPolicy))
	} else {
		modules := scanner.Modules()
		if len(modules) > 0 {
			modulePaths := make([]string, len(modules))
			for i, mod := range modules {
				modulePaths[i] = mod.Path
			}
			defaultPolicy := func(pkgPath string) bool {
				for _, modPath := range modulePaths {
					if strings.HasPrefix(pkgPath, modPath) {
						return true
					}
				}
				return false
			}
			interpreterOpts = append(interpreterOpts, symgo.WithScanPolicy(defaultPolicy))
		} else {
			interpreterOpts = append(interpreterOpts, symgo.WithPrimaryAnalysisScope(pkgPath))
		}
	}

	interpreter, err := symgo.NewInterpreter(scanner, interpreterOpts...)
	if err != nil {
		res.Error = &object.Error{Message: fmt.Sprintf("failed to create interpreter: %v", err)}
		return res
	}
	res.Interpreter = interpreter
	res.FinalEnv = interpreter.GlobalEnvForTest()

	if cfg.Intrinsics != nil {
		for name, handler := range cfg.Intrinsics {
			interpreter.RegisterIntrinsic(name, handler)
		}
	}
	if cfg.DefaultIntrinsic != nil {
		interpreter.RegisterDefaultIntrinsic(cfg.DefaultIntrinsic)
	}

	if cfg.SetupFunc != nil {
		if err := cfg.SetupFunc(interpreter); err != nil {
			res.Error = &object.Error{Message: fmt.Sprintf("WithSetup function failed: %v", err)}
			return res
		}
	}

	pkgs, err := scanner.Scan(ctx, "./...")
	if err != nil {
		res.Error = &object.Error{Message: fmt.Sprintf("failed to scan module: %v", err)}
		return res
	}

	var entryPointPkg *goscan.Package
	for _, p := range pkgs {
		if p.ImportPath == pkgPath {
			entryPointPkg = p
			break
		}
	}
	if entryPointPkg == nil {
		res.Error = &object.Error{Message: fmt.Sprintf("could not find package %q after scanning module", pkgPath)}
		return res
	}

	fnObj, ok := interpreter.FindObjectInPackage(ctx, pkgPath, fnName)
	if !ok {
		res.Error = &object.Error{Message: fmt.Sprintf("entry point function %q not found in package %q", fnName, pkgPath)}
		return res
	}

	rawResult := interpreter.EvaluatorForTest().Apply(ctx, fnObj, tc.Args, entryPointPkg)

	if err, ok := rawResult.(*object.Error); ok {
		res.Error = err
		res.ReturnValue = nil
	} else {
		finalReturnValue := rawResult
		if ret, ok := rawResult.(*object.ReturnValue); ok {
			finalReturnValue = ret.Value
		}
		res.ReturnValue = finalReturnValue
	}

	if ctx.Err() == context.DeadlineExceeded {
		res.Error = &object.Error{Message: fmt.Sprintf("timeout exceeded (%v)", cfg.Timeout)}
	}

	return res
}

// splitQualifiedName splits a name like "pkg/path.Name" into "pkg/path" and "Name".
func splitQualifiedName(name string) (pkgPath, typeName string) {
	lastDot := strings.LastIndex(name, ".")
	if lastDot == -1 {
		return "", name
	}
	return name[:lastDot], name[lastDot+1:]
}

// RunExpression is a convenience wrapper around Run for testing a single expression.
func RunExpression(t *testing.T, expr string, action func(t *testing.T, r *Result)) {
	t.Helper()
	source := fmt.Sprintf("package main\n\nfunc main() any {\n\treturn %s\n}", expr)
	tc := TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/main",
			"main.go": source,
		},
		EntryPoint: "example.com/main.main",
	}
	Run(t, tc, action)
}

// RunStatements is a convenience wrapper for testing a block of statements.
func RunStatements(t *testing.T, stmts string, action func(t *testing.T, r *Result)) {
	t.Helper()
	source := fmt.Sprintf("package main\n\nfunc main() {\n%s\n}", stmts)

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", source, 0)
	if err != nil {
		t.Fatalf("failed to parse statements: %v", err)
	}

	scanner, err := goscan.New()
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}
	interpreter, err := symgo.NewInterpreter(scanner)
	if err != nil {
		t.Fatalf("failed to create interpreter: %v", err)
	}

	// Evaluate the statements in the global environment
	globalEnv := interpreter.GlobalEnvForTest()
	for _, decl := range f.Decls {
		if fnDecl, ok := decl.(*ast.FuncDecl); ok && fnDecl.Name.Name == "main" {
			for _, stmt := range fnDecl.Body.List {
				_, err := interpreter.EvalWithEnv(context.Background(), stmt, globalEnv, nil)
				if err != nil {
					t.Fatalf("statement evaluation failed: %v", err)
				}
			}
		}
	}

	res := &Result{
		FinalEnv:    globalEnv,
		Interpreter: interpreter,
	}
	action(t, res)
}

// TestCase defines the inputs for a single symgo test.
type TestCase struct {
	// Source provides the file contents for the test, mapping filename to content.
	// A `go.mod` file is typically required.
	Source map[string]string

	// WorkDir specifies the working directory relative to the source root.
	// This is useful for multi-module workspaces. If empty, the root is used.
	WorkDir string

	// EntryPoint is the fully qualified name of the function to execute.
	// e.g., "example.com/me/main.main"
	EntryPoint string

	// Args are the symbolic objects to pass as arguments to the EntryPoint function.
	Args []object.Object

	// Options allow for customizing the test run's behavior.
	Options []Option

	// ExpectError, if true, treats a runtime error from the interpreter as an
	// expected outcome rather than a fatal test failure. The error can then be
	// inspected in the Result.
	ExpectError bool
}

// Result contains the outcome of the symbolic execution.
type Result struct {
	// ReturnValue is the object returned from the EntryPoint function.
	ReturnValue object.Object

	// FinalEnv is the environment state after the EntryPoint function has completed.
	// This can be used to inspect the values of variables.
	FinalEnv *object.Environment

	// Trace is the detailed execution trace, useful for debugging.
	// This is only populated if the default internal tracer is used.
	Trace *ExecutionTracer

	// Error is any runtime error returned by the interpreter during execution.
	Error *object.Error

	// Interpreter provides access to the configured interpreter for advanced assertions.
	Interpreter *symgo.Interpreter
}

// TraceEvent stores information about a single step in the execution trace.
type TraceEvent struct {
	Step   int
	Source string
	Pos    string
}

// ExecutionTracer captures the execution flow of the symgo evaluator.
type ExecutionTracer struct {
	Events []TraceEvent
	mu     sync.Mutex
	fset   *token.FileSet
}

// NewExecutionTracer creates a new tracer.
func NewExecutionTracer(fset *token.FileSet) *ExecutionTracer {
	return &ExecutionTracer{
		Events: make([]TraceEvent, 0, 100),
		fset:   fset,
	}
}

// Trace implements the symgo.Tracer interface.
func (t *ExecutionTracer) Trace(event object.TraceEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var buf bytes.Buffer
	if t.fset != nil && event.Node != nil && event.Node.Pos().IsValid() {
		printer.Fprint(&buf, t.fset, event.Node)
	}

	pos := ""
	if t.fset != nil && event.Node != nil && event.Node.Pos().IsValid() {
		pos = t.fset.Position(event.Node.Pos()).String()
	}

	t.Events = append(t.Events, TraceEvent{
		Step:   event.Step,
		Source: buf.String(),
		Pos:    pos,
	})
}

// Format returns a string representation of the captured trace.
func (t *ExecutionTracer) Format() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	var b strings.Builder
	b.WriteString("Execution Trace:\n")
	for _, ev := range t.Events {
		fmt.Fprintf(&b, "[Step %d] at %s\n\t%s\n", ev.Step, ev.Pos, ev.Source)
	}
	return b.String()
}

// config holds the configuration for a test run.
type config struct {
	MaxSteps         int
	Timeout          time.Duration
	ScanPolicy       symgo.ScanPolicyFunc
	Intrinsics       map[string]symgo.IntrinsicFunc
	DefaultIntrinsic symgo.IntrinsicFunc
	SetupFunc        func(interp *symgo.Interpreter) error
	Tracer           object.Tracer
}

// Option configures a test run.
type Option func(*config)

// WithTracer provides a custom tracer for the symgo interpreter.
// If this option is used, the default execution trace in the Result will not be populated.
func WithTracer(tracer object.Tracer) Option {
	return func(c *config) {
		c.Tracer = tracer
	}
}

// WithSetup provides a hook to perform arbitrary configuration on the interpreter
// after it has been created but before analysis begins. This is useful for
// advanced setup like `BindInterface`.
func WithSetup(f func(interp *symgo.Interpreter) error) Option {
	return func(c *config) {
		c.SetupFunc = f
	}
}

// WithMaxSteps sets a limit on the number of evaluation steps to prevent
// infinite loops. If the limit is exceeded, the test fails.
// Default: 10,000
func WithMaxSteps(limit int) Option {
	return func(c *config) {
		c.MaxSteps = limit
	}
}

// WithTimeout sets a time limit for the entire test run.
// Default: 5 seconds
func WithTimeout(d time.Duration) Option {
	return func(c *config) {
		c.Timeout = d
	}
}

// WithScanPolicy defines which packages are "in-policy" (evaluated recursively)
// versus "out-of-policy" (treated as symbolic placeholders).
func WithScanPolicy(policy symgo.ScanPolicyFunc) Option {
	return func(c *config) {
		c.ScanPolicy = policy
	}
}

// WithIntrinsic registers a custom handler for a specific function call,
// allowing for mocking or spying. This is a cleaner alternative to

// registering intrinsics on the interpreter manually.
func WithIntrinsic(name string, handler symgo.IntrinsicFunc) Option {
	return func(c *config) {
		if c.Intrinsics == nil {
			c.Intrinsics = make(map[string]symgo.IntrinsicFunc)
		}
		c.Intrinsics[name] = handler
	}
}

// WithDefaultIntrinsic registers a handler that is called for any function
// that does not have a specific intrinsic registered.
func WithDefaultIntrinsic(handler symgo.IntrinsicFunc) Option {
	return func(c *config) {
		c.DefaultIntrinsic = handler
	}
}

// AssertAs unwraps the result and asserts the type of the nth return value.
// For single return values, use index 0. It fails the test if the index is
// out of bounds or if the type assertion fails.
func AssertAs[T object.Object](r *Result, t *testing.T, index int) T {
	t.Helper()
	var obj object.Object

	if mr, ok := r.ReturnValue.(*object.MultiReturn); ok {
		if index < 0 || index >= len(mr.Values) {
			t.Fatalf("index %d out of bounds for multi-return value with %d values", index, len(mr.Values))
		}
		obj = mr.Values[index]
	} else {
		if index != 0 {
			t.Fatalf("index %d out of bounds for single return value", index)
		}
		obj = r.ReturnValue
	}

	val, ok := obj.(T)
	if !ok {
		var zero T
		t.Fatalf("type assertion failed: expected %T, got %T", zero, obj)
	}
	return val
}
