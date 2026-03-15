# Tide Codebase Evaluation

Based on the initial code review, here is a list of proposed fixes, feature suggestions, code restructure, and improvements to elevate the Tide editor.

## Fixes

1. **Piece Table `Delete` performance bottleneck** 
   * **Location**: `internal/buffer/piece_table.go`
   * **Issue**: The `Delete` function states it is using a "simpler (but less optimal) approach". Currently, it converts the entire buffer text to a byte slice `Bytes()`, slices out the deleted text, and creates a completely new piece table. This acts as a single contiguous array operations, resulting in `O(N)` memory allocation and copying on every deletion.
   * **Fix**: Rewrite `Delete` to slice the existing `pieces` array, splitting the targeted pieces and removing the removed range incrementally without rebuilding the text array.

2. **Synchronous highlighting block on launch**
   * **Location**: `internal/app/app.go`
   * **Issue**: The initial syntax highlighting uses `HighlightBuffer` synchronously on the main thread during `NewApp`. For very large files, this can cause a noticeable delay or UI freeze before the main editing loop starts or the first frame is drawn.
   * **Fix**: Push the initial tree-sitter highlighting into a background goroutine and dispatch an event (e.g., `TypeHighlightComplete`) to request a redraw once it's done.

3. **Silent load failures on non-existent file errors**
   * **Location**: `internal/app/app.go`
   * **Issue**: If `buf.Load(filePath)` returns an error (like permission denied), it logs a warning but proceeds to open an empty buffer. If the user hits save, they might create a local file with the same name, or clobber things due to state desync. 
   * **Fix**: Halt editor execution or show a prominent error UI overlay instead of proceeding with an empty buffer when a file exists but cannot be read.

## Feature Suggestions

1. **Global/Range Find and Replace (`:%s`)**
   * **Details**: Currently `:s/` only works on the current line. Implement standard Vim-like global buffer replace `:%s/pattern/replace/g` and range replace `:'<,'>s/...`.
   
2. **Visual Mode Capabilities**
   * **Details**: `ModeVisual` and `ModeVisualLine` exist in `modehandler.go` but lack completeness. Implement full actions: `d` (delete selection), `y` (yank selection), and allow pasting `p` over an active selection.
   
3. **Atomic Undo/Redo Transactions**
   * **Details**: The README explicitly points out global replace/plugin operations don't play well with Undo. Implement `GetHistoryManager().BeginTransaction()` and `EndTransaction()` to group multiple buffer edits into a single atomic history item.
   
4. **Terminal Mouse Drag Selection**
   * **Details**: `ModeHandler` handles click events but ignores drag events. Listen to `tcell.EventMouse` motion with Button1 held to actively update the `selectionManager` visually.

## Code Restructure and Improvements

1. **Dirty-Line / Delta Rendering (`tcell`)**
   * **Improvement**: Currently, actions trigger `a.requestRedraw()` which clears and draws the entire viewport from scratch. Introduce a "dirty lines" system to `core.Editor`. The `TUI` can then only redraw lines that have changed or scrolled, vastly improving responsiveness over SSH or slow terminals.
   
2. **Optimize Line Indexing in Buffer**
   * **Improvement**: `PieceTable`'s `positionToOffset` iterates through the lines array decoding `utf8` runes linearly from `0` to `pos.Line`. While acceptable for small files, this `O(N)` traversal degrades heavily. Implementing a cached binary search tree or interval tree for line byte offsets will make cursor jumps `O(log N)`.

3. **Strict Model-View Separation**
   * **Improvement**: Move UI-coupled event logic out of the `core`. The `editor.go` file references `redrawing` functions and specific `tcell` styling constraints. Ensure `core` operates strictly on data structures (Buffers, Cursors), emitting `event.Manager` events that the `app` or `tui` listens to for triggering view refreshes.
