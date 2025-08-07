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
