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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

// Run executes a symgo test case. It handles all setup and teardown.
// If the execution fails due to an error, timeout, or exceeded step limit,
// it will call t.Fatal with a detailed report, including an execution trace.
func Run(t *testing.T, tc TestCase, action func(t *testing.T, r *Result)) {
	t.Helper()
	res, err := runLogic(t, tc)
	if err != nil {
		if res != nil && res.Trace != nil {
			t.Fatalf("symgotest: test failed: %v\n\n%s", err, res.Trace.Format())
		}
		t.Fatalf("symgotest: test failed: %v", err)
	}
	action(t, res)
}

// runLogic contains the core logic of a test run, returning an error instead of calling t.Fatal.
func runLogic(t *testing.T, tc TestCase) (*Result, error) {
	// 1. Setup test environment
	dir, cleanup := scantest.WriteFiles(t, tc.Source)
	defer cleanup()

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
		goscan.WithWorkDir(dir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create scanner: %w", err)
	}

	tracer := NewExecutionTracer(scanner.Fset())

	pkgPath, fnName := splitQualifiedName(tc.EntryPoint)
	if pkgPath == "" {
		return nil, fmt.Errorf("invalid entry point format: %q. Expected 'path/to/package.FunctionName'", tc.EntryPoint)
	}

	interpreterOpts := []symgo.Option{
		symgo.WithLogger(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))),
		symgo.WithPrimaryAnalysisScope(pkgPath),
		symgo.WithTracer(tracer),
		symgo.WithMaxSteps(cfg.MaxSteps),
	}

	interpreter, err := symgo.NewInterpreter(scanner, interpreterOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create interpreter: %w", err)
	}

	// 3. Find entry point
	pkgs, err := scanner.Scan(ctx, "./...") // Scan the whole module recursively
	if err != nil {
		return nil, fmt.Errorf("failed to scan module: %w", err)
	}

	var entryPointPkg *goscan.Package
	for _, p := range pkgs {
		if p.ImportPath == pkgPath {
			entryPointPkg = p
			break
		}
	}
	if entryPointPkg == nil {
		return nil, fmt.Errorf("could not find package %q after scanning module", pkgPath)
	}

	fnObj, ok := interpreter.FindObjectInPackage(ctx, pkgPath, fnName)
	if !ok {
		return nil, fmt.Errorf("entry point function %q not found in package %q", fnName, pkgPath)
	}

	// 4. Execute function
	rawResult := interpreter.EvaluatorForTest().Apply(ctx, fnObj, tc.Args, entryPointPkg)

	// 5. Populate and return result
	finalReturnValue := rawResult
	if ret, ok := rawResult.(*object.ReturnValue); ok {
		finalReturnValue = ret.Value
	}

	res := &Result{
		ReturnValue: finalReturnValue,
		FinalEnv:    interpreter.GlobalEnvForTest(),
		Interpreter: interpreter,
		Trace:       tracer,
	}

	if err, ok := rawResult.(*object.Error); ok {
		res.Error = err
		res.ReturnValue = nil
		return res, err // Return the error to be formatted by the public Run function
	}

	// Check for context timeout
	if ctx.Err() == context.DeadlineExceeded {
		return res, fmt.Errorf("timeout exceeded (%v)", cfg.Timeout)
	}

	return res, nil
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

	// EntryPoint is the fully qualified name of the function to execute.
	// e.g., "example.com/me/main.main"
	EntryPoint string

	// Args are the symbolic objects to pass as arguments to the EntryPoint function.
	Args []object.Object

	// Options allow for customizing the test run's behavior.
	Options []Option
}

// Result contains the outcome of the symbolic execution.
type Result struct {
	// ReturnValue is the object returned from the EntryPoint function.
	ReturnValue object.Object

	// FinalEnv is the environment state after the EntryPoint function has completed.
	// This can be used to inspect the values of variables.
	FinalEnv *object.Environment

	// Trace is the detailed execution trace, useful for debugging.
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
	MaxSteps   int
	Timeout    time.Duration
	ScanPolicy symgo.ScanPolicyFunc
	Intrinsics map[string]symgo.IntrinsicFunc
}

// Option configures a test run.
type Option func(*config)

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

// AssertAs is a helper function that asserts the type of an object.Object.
// It fails the test if the object is not of the expected type.
func AssertAs[T object.Object](t *testing.T, obj object.Object) T {
	t.Helper()
	val, ok := obj.(T)
	if !ok {
		var zero T
		t.Fatalf("type assertion failed: expected %T, got %T", zero, obj)
	}
	return val
}

// AssertEqual is a helper function that asserts the value of an object.Object.
// It first asserts the object's type based on the type of the `expected` value,
// then compares the contained value.
func AssertEqual[T any](t *testing.T, obj object.Object, expected T) {
	t.Helper()

	switch v := any(expected).(type) {
	case int:
		integerObj := AssertAs[*object.Integer](t, obj)
		if diff := cmp.Diff(int64(v), integerObj.Value); diff != "" {
			t.Errorf("value mismatch, want=%d got=%d\n%s", v, integerObj.Value, diff)
		}
	case int64:
		integerObj := AssertAs[*object.Integer](t, obj)
		if diff := cmp.Diff(v, integerObj.Value); diff != "" {
			t.Errorf("value mismatch, want=%d got=%d\n%s", v, integerObj.Value, diff)
		}
	case string:
		stringObj := AssertAs[*object.String](t, obj)
		if diff := cmp.Diff(v, stringObj.Value); diff != "" {
			t.Errorf("value mismatch, want=%q got=%q\n%s", v, stringObj.Value, diff)
		}
	default:
		t.Fatalf("unsupported type %T for AssertEqual, actual value was: %s", expected, obj.Inspect())
	}
}
