### Prompt for Updating TODO.md from Plan Documents

**Task:** Update the `TODO.md` file by transcribing unimplemented tasks from all `docs/plan-*.md` documents.

**Detailed Requirements:**

1.  **Preserve Existing Content**:
    *   The existing instructional header/note at the top of `TODO.md` must be preserved.
    *   The entire existing `## Implemented` section must be preserved as is.

2.  **Generate "To Be Implemented" Section**:
    *   The content under the `## To Be Implemented` section should be completely replaced with a newly generated list.

3.  **Source of Tasks**:
    *   The tasks for the new list must be sourced from all `docs/plan-*.md` files.

4.  **Filtering Logic**:
    *   Only extract tasks that are **unimplemented**.
    *   An unimplemented task is a list item (e.g., `- [ ] ...` or `* ...`) found in a `plan-*.md` file that is **not** marked with `> [!NOTE] This feature has been implemented.`
    *   Specifically, look for items under sections like "Future Tasks (TODO)", "Incremental Development Plan (TODO)", or "Implementation Steps" in plans that are not yet complete.

5.  **Formatting Requirements**:
    *   Group the extracted tasks by their source file.
    -   Each group must have a level-3 heading (`###`) that includes the name of the feature and a clickable markdown link to the source `plan-*.md` file.
        *   **Correct Format:** `### Feature Name ([docs/plan-name.md](./docs/plan-name.md))`
        *   **Incorrect Format:** `### Feature Name (plan-name.md)`
    -   Each task should be a list item with a checkbox: `- [ ] Task description`.

By following these instructions, the `TODO.md` file will be correctly updated to reflect the current project status based on the detailed planning documents, while respecting the existing structure and manual content of the file.

---

### Prompt for Finalizing Plan Documents

When all tasks defined in a `plan-*.md` document are completed, follow these steps to update the documentation:

1.  **Update the completed `plan-*.md`**:
    *   Following the format of `docs/plan-overlay.md`, add the following note at the beginning of the completed `plan-*.md` file.

    ```markdown
    > [!NOTE]
    > This feature has been implemented.
    ```

2.  **Update `TODO.md`**:
    *   Mark the relevant task list items as complete (e.g., change `[ ]` to `[x]`).
    *   If the task was the last remaining item in a feature section, move the entire section from "To Be Implemented" to "Implemented".

3.  **Handling Incomplete Tasks**:
    *   If you were unable to complete all tasks in the `plan-*.md`, please add the remaining tasks as sub-tasks in `TODO.md`.

---

### Prompt for Finalizing and Refactoring TODO.md

**Task:** Periodically refactor the `TODO.md` file to maintain readability and accurately reflect high-level project progress. This involves summarizing completed work and cleaning up the task lists.

**Trigger:** This process should be initiated when the `## To Be Implemented` section becomes cluttered with numerous completed items, making it difficult to see what work is still pending.

**Refactoring Guidelines:**

1.  **Identify Completed Sections:** Locate any feature sections under `## To Be Implemented` where all sub-tasks are marked as complete (`[x]`).

2.  **Migrate and Summarize:** Move these completed sections into the `## Implemented` section. When migrating, transform the detailed checklist into a concise summary based on the following rules:
    *   **For Feature Additions:** Describe the new feature that was implemented. The goal is to capture the "what" and "why" of the change, preserving the description of the feature itself.
    *   **For Bug Fixes & Miscellaneous Tasks:** Group related fixes or smaller tasks into a single, summarized bullet point.
    *   **Preserve Key Information:** In all summaries, you **must** preserve:
        *   Any links to `docs/plan-*.md` or other documentation that explain the decision-making process.
        *   Clear descriptions of major decisions made.

3.  **Clean Up Pending Tasks:** Ensure the `## To Be Implemented` section is left in a clean state, containing only actionable tasks that are genuinely incomplete.

**Example Transformation:**

*   **Before (in `To Be Implemented`):**
    ```markdown
    ### `minigo` FFI and Language Limitations ([docs/trouble-minigo-stdlib-limitations.md](./docs/trouble-minigo-stdlib-limitations.md))
    - [x] **Implement Method Calls on Go Objects**: ...
    - [x] **Graceful Error Handling for Go Functions**: ...
    - [x] **Fix FFI method call return handling**: ...
    ```

*   **After (in `Implemented`):**
    ```markdown
    - Resolved numerous FFI and language limitations to improve stdlib compatibility, including method calls on Go objects and graceful error handling. See ([docs/trouble-minigo-stdlib-limitations.md](./docs/trouble-minigo-stdlib-limitations.md)) for details.
    ```

---

### Debugging Log: Cross-Package Interface Discovery

**Objective:** Implement robust, order-independent discovery of interface implementations, as defined by the `TestInterfaceDiscoveryCrossPkg` test case.

**Initial State:** The existing implementation was stateless and failed all scenarios in `TestInterfaceDiscoveryCrossPkg`.

**Key Steps & Discoveries:**

1.  **Build Error Resolution:** The initial effort involved fixing a cascade of build errors across the `symgo`, `evaluator`, and `scanner` packages. This was caused by outdated test files and API mismatches, such as the removal of `goscan.Implements` and changes in type names (e.g., `scanner.KindInterface` to `scanner.InterfaceKind`). This phase required significant code correction to get the project into a compilable state.

2.  **Core Logic Failure:** After fixing the build, `TestInterfaceDiscoveryCrossPkg` still failed with the error `Result value is not a string, but *object.SymbolicPlaceholder`. This indicated that the interface method call (`i.Do()`) was not being resolved to its concrete implementation.

3.  **Recursion Hypothesis:**
    *   An attempt was made to fix the issue by forcing a full package re-scan in `goscan.go` if all of a package's files had been visited individually.
    *   This led to a new problem: the test process was being `Killed`, indicating an infinite loop or memory exhaustion.
    *   The root cause was identified as a recursive loop:
        1.  `symgo.checkImplements` calls `goscan.ScanPackage`.
        2.  `goscan.ScanPackage` re-scans the source file.
        3.  The `scanner`'s AST walk triggers a callback to the `evaluator`.
        4.  The `evaluator` calls back to `symgo.HandleTypeDefinition`.
        5.  `symgo.HandleTypeDefinition` calls `checkImplements` again, creating an infinite loop.

4.  **Proposed Solution & Current Blocker:**
    *   The correct fix is to break this recursion. The chosen strategy is to use a `context.Context` value to flag when `checkImplements` is running. The `HandleTypeDefinition` callback should then check for this flag and exit early to prevent re-entry.
    *   **Blocker:** Implementing this requires plumbing the `context.Context` through the `scanner` package's internal parsing functions (`scanGoFiles` -> `parseGenDecl` -> etc.). My attempts to apply the necessary patches have been consistently failing due to a persistent mismatch between my local view of the files and their state on the server. I have been unable to resolve this synchronization issue, preventing the final fix from being applied.

**Current Status:** The code is **not functional**. The recursion bug persists, and I am unable to apply the fix. This submission is being made at the user's explicit request to save the current state.
