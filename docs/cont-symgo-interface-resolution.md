### **Initial Prompt**

The user wants me to continue working on the "symgo`: Implement Robust Interface Resolution" task.

Based on `docs/plan-symgo-interface-resolution.md`, there are four failing tests:
1.  `TestInterfaceResolution`: `Finalize()` method is not working correctly.
2.  `TestInterfaceBinding`: `BindInterface()` mechanism is not working.
3.  `TestEval_InterfaceMethodCall_AcrossControlFlow`: State merging across control flow paths is not handled.
4.  `TestDefaultIntrinsic_InterfaceMethodCall`: Test assertion is incorrect.

The `TODO.md` mentions `TestInterfaceResolution` and `TestInterfaceBinding` are failing. The main focus of *this* task seems to be `Finalize()` and `BindInterface()`.

### **Goal**

The primary goal is to fix the failing tests related to interface resolution in the `symgo` symbolic execution engine. This involves ensuring that the `Finalize` method correctly resolves interface method calls to their concrete implementations and that the `BindInterface` function works as expected for manual interface-to-type bindings.

### **Initial Implementation Attempt**

My initial approach was to fix the build errors that were present in the codebase. I found that the `evaluator.go` file had several issues, including duplicate function declarations and an incorrectly named field in the `Evaluator` struct (`scanner` vs. `goScanner`). I applied a series of patches to fix these build errors.

### **Roadblocks & Key Discoveries**

After fixing the build, I discovered that the original four logic-related tests were still failing. This indicated that the problem was not just in the build process, but in the core logic of the evaluator.

My key discovery was that the method resolution logic, particularly how it handles Go's method sets for interfaces, was flawed. The `findMethodOnType` and `isImplementer` functions did not correctly account for pointer receivers. For example, a type `T` can satisfy an interface even if some of its methods have a `*T` receiver, but the existing code did not check for this correctly.

### **Major Refactoring Effort**

Based on this discovery, I shifted my focus from fixing build errors to refactoring the method resolution logic. I attempted several patches to `symgo/evaluator/accessor.go` and `symgo/evaluator/evaluator.go`.

My attempts included:
1.  Modifying `findDirectMethodInfoOnType` in `accessor.go` to correctly compare type names and package paths, and to handle pointer vs. value receivers according to Go's method set rules.
2.  Modifying `isImplementer` in `evaluator.go` to check for method implementation on both value and pointer receivers.

These refactoring attempts were complex and, due to issues with the development environment, I was unable to apply the patches correctly and verify their effectiveness.

### **Current Status**

The codebase is in a state where the build is fixed, but the original four logic tests related to interface resolution are still failing. My last attempts to patch the method resolution logic were unsuccessful due to environmental issues.

The failing tests are:
-   `TestInterfaceResolution`
-   `TestInterfaceBinding`
-   `TestEval_InterfaceMethodCall_AcrossControlFlow`
-   `TestDefaultIntrinsic_InterfaceMethodCall`

### **References**

A future agent should consult the following files for context:
-   `docs/plan-symgo-interface-resolution.md`
-   `symgo/evaluator/evaluator.go`
-   `symgo/evaluator/accessor.go`
-   The Go specification on method sets.

### **TODO / Next Steps**

1.  **Fix Method Set Logic**: The core task is to fix the method resolution logic. I recommend focusing on the `isImplementer` function in `symgo/evaluator/evaluator.go`. The function should be modified to check for methods on both the value type (`T`) and a pointer to the value type (`*T`) when determining if a type implements an interface. Here is the code I was trying to write:
    ```go
    // isImplementer checks if a given concrete type implements an interface.
    func (e *Evaluator) isImplementer(ctx context.Context, concreteType *scanner.TypeInfo, interfaceType *scanner.TypeInfo) bool {
        if concreteType == nil || interfaceType == nil || interfaceType.Interface == nil {
            return false
        }

        // For every method in the interface...
        for _, ifaceMethodInfo := range interfaceType.Interface.Methods {
            // ...find a matching method in the concrete type.
            // A concrete type T can implement an interface method with a *T receiver.
            // So we need to check both T and *T.
            concreteMethodInfo := e.accessor.findMethodInfoOnType(ctx, concreteType, ifaceMethodInfo.Name)

            if concreteMethodInfo == nil && !strings.HasPrefix(concreteType.Name, "*") {
                // If not found on T, check on *T.
                // Create a synthetic pointer type for the check.
                pointerType := *concreteType
                pointerType.Name = "*" + concreteType.Name
                // We need to be careful not to lose the underlying struct info.
                // This is a shallow copy, but it should be sufficient for the accessor.
                concreteMethodInfo = e.accessor.findMethodInfoOnType(ctx, &pointerType, ifaceMethodInfo.Name)
            }

            if concreteMethodInfo == nil {
                return false // Method not found
            }

            // Compare signatures
            if len(ifaceMethodInfo.Parameters) != len(concreteMethodInfo.Parameters) {
                return false
            }
            if len(ifaceMethodInfo.Results) != len(concreteMethodInfo.Results) {
                return false
            }

            for i, p1 := range ifaceMethodInfo.Parameters {
                p2 := concreteMethodInfo.Parameters[i]
                if !e.fieldTypeEquals(p1.Type, p2.Type) {
                    return false
                }
            }

            for i, r1 := range ifaceMethodInfo.Results {
                r2 := concreteMethodInfo.Results[i]
                if !e.fieldTypeEquals(r1.Type, r2.Type) {
                    return false
                }
            }
        }
        return true
    }
    ```
2.  **Run Tests**: After applying the patch, run `go test -v ./symgo/...` to verify the fix.
3.  **Submit**: Once all tests are passing, submit the changes.
