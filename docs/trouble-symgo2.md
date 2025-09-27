# Trouble with Method Values as Higher-Order Function Arguments

This document details the investigation into a bug where `symgo` fails to resolve an identifier within a method that is passed as a value to a higher-order function.

## 1. Symptom

When running `goinspect` on the `net/http/httptest` package, the analysis fails with an "identifier not found" error.

```sh
$ goinspect -pkg net/http/httptest > /dev/null

level=ERROR msg="identifier not found: s" \
  symgo.in_func=Close \
  symgo.pos=/opt/homebrew/Cellar/go/1.24.3/libexec/src/net/http/httptest/server.go:255:2
```

The error occurs within the `(*Server).Close` method, which contains the following code:

```go
// /opt/homebrew/Cellar/go/1.24.3/libexec/src/net/http/httptest/server.go

func (s *Server) Close() {
    // ...
    t := time.AfterFunc(5*time.Second, s.logCloseHangDebugInfo)
    // ...
}

func (s *Server) logCloseHangDebugInfo() {
	s.mu.Lock() // ERROR: identifier not found: s
    // ...
}
```

The issue is triggered when `symgo` attempts to analyze the body of `logCloseHangDebugInfo`, which was passed as an argument to `time.AfterFunc`.

## 2. Root Cause Analysis

Analysis of the debug logs (`-log-level debug`) revealed the precise mechanism of the failure.

1.  **`evalCallExpr` Heuristic**: The `symgo` evaluator's `evalCallExpr` function contains a heuristic to proactively discover function calls within arguments. When it sees a call like `time.AfterFunc(..., s.logCloseHangDebugInfo)`, it identifies `s.logCloseHangDebugInfo` as a function-like argument.

2.  **`scanFunctionLiteral` is Triggered**: This heuristic immediately invokes `scanFunctionLiteral` on the `object.Function` representing `s.logCloseHangDebugInfo`. The purpose of this scan is to trace any calls *inside* the function body without fully evaluating the higher-order function (`time.AfterFunc`) it is passed to.

3.  **The Flaw: Missing Receiver Context**: The core of the problem lies in `scanFunctionLiteral`. This function creates a new, temporary environment to symbolically execute the function's body. It correctly populates this environment with symbolic placeholders for the function's *parameters*. However, it **does not account for the function's receiver**.

4.  **Execution Failure**: When `scanFunctionLiteral` begins evaluating the body of `logCloseHangDebugInfo`, its environment is missing the receiver `s`. The first statement it encounters is `s.mu.Lock()`. The evaluator looks for `s` in the current (temporary) environment, cannot find it, and throws the "identifier not found: s" error.

The `object.Function` created for the method value correctly contains the receiver instance in its `Receiver` field. The failure is that `scanFunctionLiteral` does not utilize this information when constructing the evaluation environment.

## 3. Path to Resolution

The fix requires modifying `scanFunctionLiteral` in `symgo/evaluator/evaluator.go`. The function must be updated to check if the `object.Function` it is scanning has a non-nil `Receiver`.

If a receiver exists, `scanFunctionLiteral` must:
1.  Identify the receiver's name from the function's AST declaration (`fn.Decl.Recv`).
2.  Bind the `fn.Receiver` object to that name within the new, temporary environment before evaluating the function body.

This will mirror the logic already present in `extendFunctionEnv` for regular function calls, ensuring that the method's receiver is correctly in scope during the symbolic scan.