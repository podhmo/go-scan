# Undefined Method on Pointer to Named Map Type

This document outlines the analysis and resolution plan for a bug in the `symgo` symbolic execution engine where it fails to resolve a method call on a pointer to a named type that is based on a map.

## 1. Problem Description

The `symgo` evaluator throws an `undefined method or field` error when analyzing code that calls a method on a pointer to a named map type. The method is defined on the value receiver of the type, a pattern that the standard Go compiler handles transparently via automatic dereferencing.

### Example Log Output

```
level=ERROR msg="undefined method or field: Has for pointer type MAP" \
in_func=ExtractEmailNotificationSettingDisableTopic \
exec_pos=.../symgo/evaluator/evaluator.go:2261 \
pos=.../mail/extract.go:8:5
```

### Example Code That Fails

```go
// mail/model.go
type UnsubscribedGroups map[Group]bool

func (s UnsubscribedGroups) Has(group Group) bool { // Method on VALUE receiver
	return s[group]
}

func NewUnsubscribedGroups(tags []string) *UnsubscribedGroups { // Returns POINTER
	result := UnsubscribedGroups{}
	// ... logic to populate map ...
	return &result
}

// mail/extract.go
func ExtractEmailNotificationSettingDisableTopic(tags []string) {
	unsubscribe := NewUnsubscribedGroups(tags) // unsubscribe is *UnsubscribedGroups
	if unsubscribe.Has(GroupTopic) { // ERROR: symgo fails to find .Has
		// ...
	}
}
```

## 2. Analysis

The error message `undefined method or field: Has for pointer type MAP` is the key.

1.  **Type Information Loss**: The evaluator correctly identifies that `unsubscribe` is a pointer. However, instead of seeing it as a pointer to the named type `*UnsubscribedGroups`, it seems to resolve it to a generic `pointer type MAP`. It loses the specific type name (`UnsubscribedGroups`).
2.  **Incorrect Method Set**: Because the specific type information is lost, `symgo` looks for the `Has` method on the method set of a generic `*MAP`. This method set is empty.
3.  **Missing Automatic Dereference**: The Go compiler would handle this by checking the method set of the pointer type `*UnsubscribedGroups`, not finding `Has`, and then automatically checking the method set of the value type `UnsubscribedGroups`, where it would find it. `symgo`'s logic is failing to perform this second step, likely because it has already lost the type name.

The root cause is likely in how the evaluator handles method calls (`evalSelectorExpr`) on pointer objects, especially when the pointer's underlying element is a named type wrapping a built-in like a map. The type information is not being preserved or accessed correctly during method resolution.

## 3. Plan for Resolution

1.  **Create a Failing Test Case**:
    *   Add a new test file in the `symgo/` directory.
    *   This test will define a self-contained Go program as a string that replicates the bug pattern:
        *   A named type based on `map`.
        *   A method on the value receiver of that type.
        *   A function returning a pointer to that type.
        *   A call to the method on the pointer.
    *   The test will use `symgo` to analyze this code and assert that the analysis completes without error and correctly identifies the function calls.

2.  **Implement the Fix in the Evaluator**:
    *   The primary location for the fix is the `evalSelectorExpr` function in `symgo/evaluator/evaluator.go`.
    *   The logic must be updated to handle method calls on `object.Pointer` types.
    *   When a method is not found on the pointer's direct method set, the evaluator must look at the `Value` the pointer points to.
    *   It must then correctly retrieve the method set from that underlying value type, even if it's a named map or other named built-in. This means ensuring the full `object.Type` (including its name) is preserved and inspected.

3.  **Verify the Fix**:
    *   Run the newly created test to confirm it passes.
    *   Run the entire `symgo` test suite (`go test ./...`) to ensure no regressions have been introduced.

4.  **Update `TODO.md`**:
    *   Once the fix is verified, update the main `TODO.md` file to reflect that this task has been completed.
    *   Add a new entry under an appropriate "Implemented" section detailing the fix.