# Symgo Error Analysis - Standard Library Testing

This document analyzes **ERROR-level** logs encountered when running `goinspect -pkg` on all Go standard library packages.
The goal is to identify and categorize critical issues in the symgo evaluator to improve its completeness.

**Total packages tested:** 171
**Packages with ERROR logs:** 43
**Success rate (no ERROR logs):** 74.9%
**Total ERROR log lines:** 2270

## Error Category Summary

| Priority | Error Type | Packages | Description |
|----------|------------|----------|-------------|
| High | `goto_unsupported` | 14 | Goto statement not supported |
| High | `identifier_not_found` | 11 | Identifier resolution failure (often generic type parameters) |
| Medium | `const_overflow` | 9 | Integer constant overflow (uint64 or large constants) |
| Medium | `selector_type_error` | 8 | Type error in selector expression |
| Low | `hex_parse_error` | 6 | Literal parsing error (hex literals with too many digits) |
| Medium | `unknown_operator` | 4 | Unsupported operator for complex types |
| High | `invalid_indirect` | 3 | Dereferencing nil pointer |

## Detailed Error Analysis

### Goto Unsupported

**Priority:** High

**Description:** Goto statement not supported

**Affected packages (14):**

- `bytes`
- `debug/dwarf`
- `go/build`
- `go/constant`
- `go/doc/comment`
- `index/suffixarray`
- `math/rand`
- `net`
- `os`
- `regexp`
- `runtime`
- `strconv`
- `strings`
- `text/scanner`

**Example errors:**

**Package `bytes`:**
```
time=2025-10-05T10:18:46.056+09:00 level=ERROR msg="unsupported branch statement: goto" symgo.in_func=EqualFold symgo.in_func_pos="" symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_branch_stmt.go:25 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/bytes/bytes.go:1228:4
```

**Package `debug/dwarf`:**
```
time=2025-10-05T10:18:49.525+09:00 level=ERROR msg="unsupported branch statement: goto" symgo.in_func=readType symgo.in_func_pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/debug/dwarf/type.go:376:9 symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_branch_stmt.go:25 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/debug/dwarf/type.go:513:4
```

**Package `go/build`:**
```
time=2025-10-05T10:18:52.083+09:00 level=ERROR msg="unsupported branch statement: goto" symgo.in_func=Import symgo.in_func_pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/go/build/build.go:524:9 symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_branch_stmt.go:25 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/go/build/build.go:633:5
```

**Analysis:**

The symgo evaluator does not currently support `goto` statements. This is a common control flow construct in low-level code, particularly in:
- Parser implementations (e.g., `go/constant`, `text/scanner`)
- String processing (e.g., `bytes`, `strings`)
- Network and system code (e.g., `net`, `os`, `runtime`)

**Recommended solution:**
- Implement label tracking in the evaluator
- Add support for jumping to labeled statements
- Alternative: Transform goto into structured control flow during preprocessing

---

### Identifier Not Found

**Priority:** High

**Description:** Identifier resolution failure (often generic type parameters)

**Affected packages (11):**

- `arena`
- `crypto/x509`
- `database/sql`
- `go/types`
- `math/rand/v2`
- `os`
- `reflect`
- `slices`
- `sync/atomic`
- `syscall/js`
- `time`

**Example errors:**

**Package `arena`:**
```
time=2025-10-05T10:18:45.809+09:00 level=ERROR msg="identifier not found: T" symgo.in_func=New symgo.in_func_pos="" symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_ident.go:104 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/arena/arena.go:66:59
```

*Specific issue: `T`*

**Package `crypto/x509`:**
```
time=2025-10-05T10:18:48.793+09:00 level=ERROR msg="identifier not found: dnsNames" symgo.in_func=<closure> symgo.in_func_pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/crypto/x509/parser.go:658:113 symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_ident.go:104 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/crypto/x509/parser.go:578:23
```

*Specific issue: `dnsNames`*

**Package `database/sql`:**
```
time=2025-10-05T10:18:49.135+09:00 level=ERROR msg="identifier not found: T" symgo.in_func=Scan symgo.in_func_pos="" symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_ident.go:104 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/database/sql/sql.go:422:23
```

*Specific issue: `T`*

**Analysis:**

This error typically occurs with:
- Generic type parameters (e.g., `T` in generic functions)
- Type constraints and interface types
- Scoping issues with nested function declarations

**Recommended solution:**
- Improve type parameter tracking in function signatures
- Add proper scope handling for generic types
- Ensure type constraints are registered in the symbol table

---

### Const Overflow

**Priority:** Medium

**Description:** Integer constant overflow (uint64 or large constants)

**Affected packages (9):**

- `hash/crc64`
- `hash/fnv`
- `math`
- `math/rand`
- `os`
- `runtime`
- `strconv`
- `syscall`
- `time`

**Example errors:**

**Package `hash/crc64`:**
```
time=2025-10-05T10:18:53.634+09:00 level=ERROR msg="could not convert constant to int64: 15564440312192434176" symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_ident.go:117 symgo.pos=0
```

*Specific issue: `15564440312192434176`*

**Package `hash/fnv`:**
```
time=2025-10-05T10:18:53.690+09:00 level=ERROR msg="could not convert constant to int64: 14695981039346656037" symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_ident.go:117 symgo.pos=0
```

*Specific issue: `14695981039346656037`*

**Package `math`:**
```
time=2025-10-05T10:18:55.393+09:00 level=ERROR msg="could not convert constant to int64: 18442240474082181120" symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_ident.go:117 symgo.pos=0
```

*Specific issue: `18442240474082181120`*

**Analysis:**

Large integer constants that exceed `int64` range (but fit in `uint64`) cause conversion errors.
Common in:
- Syscall constants (file descriptor flags, ioctl values)
- Time constants (maximum duration values)
- Hash functions (64-bit magic numbers)

**Recommended solution:**
- Add support for `uint64` constants
- Use arbitrary precision integers for constant evaluation
- Properly detect constant type from usage context

---

### Selector Type Error

**Priority:** Medium

**Description:** Type error in selector expression

**Affected packages (8):**

- `crypto/ed25519`
- `crypto/x509`
- `embed`
- `net`
- `os`
- `runtime`
- `syscall/js`
- `time`

**Example errors:**

**Package `crypto/ed25519`:**
```
time=2025-10-05T10:18:47.494+09:00 level=ERROR msg="expected a package, instance, or pointer on the left side of selector, but got SLICE" symgo.in_func=GenerateKey symgo.in_func_pos="" symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_selector_expr.go:561 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/crypto/ed25519/ed25519.go:148:15
```

*Specific issue: `SLICE`*

**Package `crypto/x509`:**
```
time=2025-10-05T10:18:48.843+09:00 level=ERROR msg="expected a package, instance, or pointer on the left side of selector, but got INTEGER" symgo.in_func=signTBS symgo.in_func_pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/crypto/x509/x509.go:1781:20 symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_selector_expr.go:561 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/crypto/x509/x509.go:1573:14
```

*Specific issue: `INTEGER`*

**Package `embed`:**
```
time=2025-10-05T10:18:49.997+09:00 level=ERROR msg="expected a package, instance, or pointer on the left side of selector, but got INTEGER" symgo.in_func=Type symgo.in_func_pos="" symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_selector_expr.go:561 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/embed/embed.go:224:53
```

*Specific issue: `INTEGER`*

**Analysis:**

Type inference fails for intermediate expressions in selector chains.
Often occurs when:
- Arithmetic operations produce unexpected types
- Type conversions aren't properly tracked
- Constants are assigned incorrect types

**Recommended solution:**
- Improve type inference for binary operations
- Better track constant types through expressions
- Add type coercion rules for selector expressions

---

### Hex Parse Error

**Priority:** Low

**Description:** Literal parsing error (hex literals with too many digits)

**Affected packages (6):**

- `crypto/des`
- `math/cmplx`
- `math/rand/v2`
- `net/http`
- `runtime`
- `strconv`

**Example errors:**

**Package `crypto/des`:**
```
time=2025-10-05T10:18:47.114+09:00 level=ERROR msg="could not parse \"0xaaaaaaaa55555555\" as integer" symgo.in_func=permuteInitialBlock symgo.in_func_pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/crypto/des/block.go:14:6 symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_basic_lit.go:18 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/crypto/des/block.go:160:15
```

*Specific issue: `0xaaaaaaaa55555555`*

**Package `math/cmplx`:**
```
time=2025-10-05T10:18:55.704+09:00 level=ERROR msg="could not parse \"0xfe13abe8fa9a6ee0\" as integer" symgo.in_func=reducePi symgo.in_func_pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/math/cmplx/tan.go:225:6 symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_basic_lit.go:18 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/math/cmplx/tan.go:172:3
```

*Specific issue: `0xfe13abe8fa9a6ee0`*

**Package `math/rand/v2`:**
```
time=2025-10-05T10:18:55.840+09:00 level=ERROR msg="could not parse \"0xda942042e4dd58b5\" as integer" symgo.in_func=Uint64 symgo.in_func_pos="" symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_basic_lit.go:18 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/math/rand/v2/pcg.go:100:19
```

*Specific issue: `0xda942042e4dd58b5`*

**Analysis:**

Hexadecimal literals with values exceeding int64 range fail to parse.
Related to the const_overflow issue.

**Recommended solution:**
- Parse hex literals as uint64 or arbitrary precision
- Determine actual type from usage context

---

### Unknown Operator

**Priority:** Medium

**Description:** Unsupported operator for complex types

**Affected packages (4):**

- `archive/tar`
- `math/cmplx`
- `syscall`
- `time`

**Example errors:**

**Package `archive/tar`:**
```
time=2025-10-05T10:18:45.593+09:00 level=ERROR msg="unknown complex operator: |" symgo.in_func=allowedFormats symgo.in_func_pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/archive/tar/writer.go:97:34 symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_binary_expr.go:92 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/archive/tar/common.go:344:11
```

*Specific issue: `|`*

**Package `math/cmplx`:**
```
time=2025-10-05T10:18:55.703+09:00 level=ERROR msg="unknown complex operator: >" symgo.in_func=Sqrt symgo.in_func_pos="" symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_binary_expr.go:92 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/math/cmplx/sqrt.go:94:5
```

*Specific issue: `>`*

**Package `syscall`:**
```
time=2025-10-05T10:19:08.524+09:00 level=ERROR msg="unknown complex operator: %" symgo.in_func=NsecToTimeval symgo.in_func_pos="" symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_binary_expr.go:92 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/syscall/timestruct.go:29:10
```

*Specific issue: `%`*

**Analysis:**

Binary operators on `complex` types (complex64, complex128) are not fully implemented.
Missing operators include: `|` (bitwise OR), `>` (comparison)

**Recommended solution:**
- Implement complex number comparison operators
- Note: Bitwise operators on complex numbers may indicate type inference errors

---

### Invalid Indirect

**Priority:** High

**Description:** Dereferencing nil pointer

**Affected packages (3):**

- `archive/tar`
- `crypto/x509`
- `runtime`

**Example errors:**

**Package `archive/tar`:**
```
time=2025-10-05T10:18:45.592+09:00 level=ERROR msg="invalid indirect of nil (type *object.Nil)" symgo.in_func=WriteHeader symgo.in_func_pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/archive/tar/writer.go:430:13 symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_star_expr.go:104 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/archive/tar/writer.go:74:11
```

**Package `crypto/x509`:**
```
time=2025-10-05T10:18:48.898+09:00 level=ERROR msg="invalid indirect of nil (type *object.Nil)" symgo.in_func=parseECPrivateKey symgo.in_func_pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/crypto/x509/sec1.go:38:9 symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_star_expr.go:104 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/crypto/x509/sec1.go:101:29
```

**Package `runtime`:**
```
time=2025-10-05T10:19:05.735+09:00 level=ERROR msg="invalid indirect of nil (type *object.Nil)" symgo.in_func=write symgo.in_func_pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/runtime/cpuprof.go:88:3 symgo.exec_pos=/home/po/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator_eval_star_expr.go:104 symgo.pos=/home/po/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.24.3.linux-amd64/src/runtime/profbuf.go:351:54
```

**Analysis:**

Attempting to dereference a nil pointer or uninitialized value.
Often happens when:
- Control flow analysis doesn't track nil checks
- Variables aren't properly initialized in evaluation

**Recommended solution:**
- Improve control flow analysis for nil checks
- Add better handling of uninitialized pointers
- Track pointer nullability through execution

---

## Priority-Based Implementation Plan

### High Priority

1. **Goto Statement Support** (14 packages)
   - Most common high-priority issue
   - Blocking analysis of core packages like `runtime`, `os`, `net`
   - Implementation: Add label table and jump mechanism to evaluator

2. **Generic Type Parameter Resolution** (11 packages)
   - Critical for modern Go code (Go 1.18+)
   - Affects generic containers and algorithms
   - Implementation: Extend scope system to track type parameters

3. **Nil Pointer Dereferencing** (3 packages)
   - Can cause evaluation crashes
   - Implementation: Better control flow and initialization tracking

### Medium Priority

4. **Integer Constant Overflow** (9 packages)
   - Blocks system-level packages
   - Implementation: Add uint64 support in constant evaluator

5. **Selector Type Errors** (8 packages)
   - Type inference quality issue
   - Implementation: Improve type propagation in expressions

6. **Complex Type Operators** (4 packages)
   - Limited impact (mostly math packages)
   - Implementation: Add comparison operators for complex types

### Low Priority

7. **Hex Literal Parsing** (6 packages)
   - Related to const_overflow issue
   - Will be fixed by uint64 support

## Testing Coverage by Category

**Packages with NO errors:** 128

Examples of fully working packages:

- `archive/zip`
- `bufio`
- `builtin`
- `cmp`
- `compress/bzip2`
- `compress/flate`
- `compress/gzip`
- `compress/lzw`
- `compress/zlib`
- `container/heap`
- `container/list`
- `container/ring`
- `context`
- `crypto`
- `crypto/aes`
- *...and 113 more*

## Impact Assessment

### By Package Category

| Category | Total | Errors | Success Rate |
|----------|-------|--------|--------------|
| archive | 2 | 1 | 50.0% |
| arena | 1 | 1 | 0.0% |
| bufio | 1 | 0 | 100.0% |
| builtin | 1 | 0 | 100.0% |
| bytes | 1 | 1 | 0.0% |
| cmp | 1 | 0 | 100.0% |
| compress | 5 | 0 | 100.0% |
| container | 3 | 0 | 100.0% |
| context | 1 | 0 | 100.0% |
| crypto | 23 | 5 | 78.3% |
| database | 2 | 1 | 50.0% |
| debug | 7 | 1 | 85.7% |
| embed | 1 | 1 | 0.0% |
| encoding | 12 | 1 | 91.7% |
| errors | 1 | 0 | 100.0% |
| expvar | 1 | 0 | 100.0% |
| flag | 1 | 0 | 100.0% |
| fmt | 1 | 0 | 100.0% |
| go | 14 | 5 | 64.3% |
| hash | 6 | 2 | 66.7% |
| html | 2 | 0 | 100.0% |
| image | 7 | 1 | 85.7% |
| index | 1 | 1 | 0.0% |
| io | 3 | 0 | 100.0% |
| iter | 1 | 0 | 100.0% |
| log | 3 | 0 | 100.0% |
| maps | 1 | 0 | 100.0% |
| math | 6 | 5 | 16.7% |
| mime | 3 | 0 | 100.0% |
| net | 16 | 2 | 87.5% |
| os | 4 | 1 | 75.0% |
| path | 2 | 0 | 100.0% |
| plugin | 1 | 0 | 100.0% |
| reflect | 1 | 1 | 0.0% |
| regexp | 2 | 2 | 0.0% |
| runtime | 10 | 2 | 80.0% |
| slices | 1 | 1 | 0.0% |
| sort | 1 | 0 | 100.0% |
| strconv | 1 | 1 | 0.0% |
| strings | 1 | 1 | 0.0% |
| sync | 2 | 1 | 50.0% |
| syscall | 2 | 2 | 0.0% |
| testing | 5 | 1 | 80.0% |
| text | 4 | 1 | 75.0% |
| time | 2 | 1 | 50.0% |
| unicode | 3 | 0 | 100.0% |
| unsafe | 1 | 0 | 100.0% |

## Conclusion

The symgo evaluator successfully handles **75.1%** of standard library packages without critical errors.
The main areas for improvement are:

1. **Control flow** - goto statement support
2. **Type system** - generic type parameters and better type inference
3. **Constants** - uint64 and large integer handling

Addressing these three areas would significantly improve coverage, potentially reaching 90%+ success rate.
