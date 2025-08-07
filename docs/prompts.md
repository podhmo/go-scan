# Prompt for Updating TODO.md from Plan Documents

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

### 計画ドキュメントの完了処理

`plan-*.md`で定義されたタスクがすべて完了した場合、以下の手順でドキュメントを更新してください。

1.  **完了済み`plan-*.md`の更新**:
    *   `docs/plan-overlay.md`の形式に倣い、完了した`plan-*.md`ファイルの先頭に以下のNoteを追加します。

    ```markdown
    > [!NOTE]
    > This feature has been implemented.
    ```

2.  **`TODO.md`の更新**:
    *   関連するタスクリストの項目を完了済みにします（例：`[ ]`を`[x]`に変更）。
    *   もし、そのタスクが特定の機能セクションの最後の未完了タスクであった場合は、セクション全体を「To Be Implemented」から「Implemented」に移動します。

3.  **未完了タスクの扱い**:
    *   もし、`plan-*.md`のタスクをすべて完了できなかった場合は、残りのタスクをサブタスクとして`TODO.md`に追記してください。
