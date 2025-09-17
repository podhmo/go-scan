package symgotest_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestRunner_Apply_Simple(t *testing.T) {
	source := `
func add(a, b int) int {
	return a + b
}
`
	runner := symgotest.NewRunner(t, source)
	result := runner.Apply("add", &object.Integer{Value: 2}, &object.Integer{Value: 3})

	symgotest.AssertSuccess(t, result)

	retVal, ok := result.ReturnValue.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected a ReturnValue, got %T", result.ReturnValue)
	}

	// For simple value checks, we can inspect the object directly.
	if intObj, ok := retVal.Value.(*object.Integer); !ok || intObj.Value != 5 {
		t.Errorf("expected return value to be integer 5, but got %v", retVal.Value)
	}
}

func TestRunner_Apply_WithError(t *testing.T) {
	source := `
func main() {
	panic("something went wrong")
}
`
	runner := symgotest.NewRunner(t, source)
	result := runner.Apply("main")

	symgotest.AssertError(t, result, "panic: something went wrong")
}

func TestRunner_TrackCalls_SingleFile(t *testing.T) {
	source := `
func main() {
	doWork()
}
func doWork() {}
`
	runner := symgotest.NewRunner(t, source).TrackCalls()
	result := runner.Apply("main")

	symgotest.AssertSuccess(t, result)
	symgotest.AssertCalled(t, result, "example.com/simple.doWork")
	symgotest.AssertNotCalled(t, result, "example.com/simple.otherFunc")
}

func TestRunner_TrackCalls_MultiFile(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/app",
		"main.go": `package main
import "example.com/app/service"
func main() {
    service.Run()
}`,
		"service/service.go": `package service
import "example.com/app/worker"
func Run() {
    worker.DoWork()
}`,
		"worker/worker.go": `package worker
func DoWork() {}`,
	}

	runner := symgotest.NewRunnerWithMultiFiles(t, files).TrackCalls()
	result := runner.Apply("main")

	symgotest.AssertSuccess(t, result)

	// Check the whole call chain
	symgotest.AssertCalled(t, result, "example.com/app.main")
	symgotest.AssertCalled(t, result, "example.com/app/service.Run")
	symgotest.AssertCalled(t, result, "example.com/app/worker.DoWork")
}

func TestAssertions(t *testing.T) {
	t.Run("AssertCalled with failure", func(t *testing.T) {
		mockT := new(testing.T)
		result := &symgotest.RunResult{
			FunctionsCalled: []string{"foo.bar"},
		}
		symgotest.AssertCalled(mockT, result, "foo.baz")
		if !mockT.Failed() {
			t.Error("AssertCalled should have failed but didn't")
		}
	})

	t.Run("AssertNotCalled with failure", func(t *testing.T) {
		mockT := new(testing.T)
		result := &symgotest.RunResult{
			FunctionsCalled: []string{"foo.bar"},
		}
		symgotest.AssertNotCalled(mockT, result, "foo.bar")
		if !mockT.Failed() {
			t.Error("AssertNotCalled should have failed but didn't")
		}
	})
}
