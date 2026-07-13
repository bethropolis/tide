package core

import (
	"errors"

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/config"
	"github.com/bethropolis/tide/internal/core/clipboard"
	"github.com/bethropolis/tide/internal/core/cursor"
	"github.com/bethropolis/tide/internal/core/find"
	"github.com/bethropolis/tide/internal/core/highlight" // Import core highlight
	"github.com/bethropolis/tide/internal/core/history"
	"github.com/bethropolis/tide/internal/core/selection"
	"github.com/bethropolis/tide/internal/core/text"
	"github.com/bethropolis/tide/internal/event"
	hl "github.com/bethropolis/tide/internal/highlighter"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// Editor coordinates all editing operations
type Editor struct {
	buffer     buffer.Buffer
	viewWidth  int // Cached terminal width
	viewHeight int // Cached terminal height (excluding status bar)
	scrollOff  int // Number of lines to keep visible above/below cursor

	// Event Manager
	eventManager *event.Manager // Added for dispatching events on delete etc.

	// Highlighter Service (passed in)
	highlighter *hl.Highlighter // Highlighter service instance

	// Sub-system Managers
	clipboardManager *clipboard.Manager
	cursorManager    *cursor.Manager
	selectionManager *selection.Manager
	textOps          *text.Operations
	historyManager   *history.Manager
	findManager      *find.Manager
	highlightManager *highlight.Manager // Use the core highlight manager

	// Dirty-line tracking: set of buffer line indices that changed since last draw.
	// When forceFullRedraw is true the entire viewport must be redrawn.
	dirtyLines      map[int]struct{}
	forceFullRedraw bool
}

// NewEditor creates a new Editor instance with a given buffer.
// eventManager is used by the highlight manager to dispatch TypeHighlightComplete
// when a background highlighting pass completes; pass nil to disable.
func NewEditor(buf buffer.Buffer, highlighterService *hl.Highlighter, eventManager *event.Manager) *Editor {
	cfg := config.Get() // Get loaded config
	e := &Editor{
		buffer:      buf,
		scrollOff:   cfg.Editor.ScrollOff,
		highlighter: highlighterService, // Store the service
	}

	// Initialize managers that depend on the editor (e)
	e.textOps = text.NewOperations(e)
	e.cursorManager = cursor.NewManager(e)
	e.selectionManager = selection.NewManager(e)
	e.clipboardManager = clipboard.NewManager(e, cfg.Editor.SystemClipboard)
	e.historyManager = history.NewManager(e, history.DefaultMaxHistory)
	e.findManager = find.NewManager(e)
	// Initialize highlight manager with the event manager so it can fire
	// TypeHighlightComplete when a background pass finishes.
	e.highlightManager = highlight.NewManager(e, e.highlighter, eventManager)
	e.eventManager = eventManager
	e.dirtyLines = make(map[int]struct{})
	e.forceFullRedraw = true // First draw is always a full redraw

	logger.DebugTagf("core", "Editor created and managers initialized. System Clipboard: %v", cfg.Editor.SystemClipboard)
	return e
}

// SetEventManager sets the event manager for dispatching events
func (e *Editor) SetEventManager(mgr *event.Manager) {
	e.eventManager = mgr
}

// SetHighlighter (optional) allows changing the highlighter service later if needed.
func (e *Editor) SetHighlighter(h *hl.Highlighter) {
	e.highlighter = h
	// TODO: Should potentially trigger a re-highlight if service changes?
	// The highlight manager might need to be updated too.
}

// GetBuffer returns the editor's buffer.
func (e *Editor) GetBuffer() buffer.Buffer {
	return e.buffer
}

// GetCursor returns the current cursor position.
func (e *Editor) GetCursor() types.Position {
	if e.cursorManager == nil {
		logger.Warnf("Editor.GetCursor called before cursorManager initialized")
		return types.Position{Line: 0, Col: 0} // Fallback
	}
	return e.cursorManager.GetPosition()
}

// GetViewport returns the viewport position by delegating to the cursor manager
func (e *Editor) GetViewport() (int, int) {
	if e.cursorManager == nil {
		logger.Warnf("Editor.GetViewport called before cursorManager initialized")
		return 0, 0 // Fallback
	}
	return e.cursorManager.GetViewport()
}

// GetCurrentTree safely gets the current tree from the highlight manager
func (e *Editor) GetCurrentTree() *sitter.Tree {
	if e.highlightManager == nil {
		logger.Warnf("Editor.GetCurrentTree called before highlightManager initialized")
		return nil
	}
	return e.highlightManager.GetCurrentTree()
}

// GetEventManager returns the event manager
func (e *Editor) GetEventManager() *event.Manager {
	return e.eventManager
}

// --- Selection Methods ---

// GetSelecting returns whether selection is active
func (e *Editor) GetSelecting() bool {
	if e.selectionManager == nil {
		logger.Warnf("Editor.GetSelecting called before selectionManager initialized")
		return false
	}
	return e.selectionManager.IsSelecting()
}

// HasSelection returns whether there's an active selection
func (e *Editor) HasSelection() bool {
	if e.selectionManager == nil {
		logger.Warnf("Editor.HasSelection called before selectionManager initialized")
		return false
	}
	return e.selectionManager.HasSelection()
}

// GetSelection returns the normalized selection range (start <= end).
func (e *Editor) GetSelection() (start types.Position, end types.Position, ok bool) {
	if e.selectionManager == nil {
		logger.Warnf("Editor.GetSelection called before selectionManager initialized")
		return types.Position{Line: -1, Col: -1}, types.Position{Line: -1, Col: -1}, false
	}
	return e.selectionManager.GetSelection()
}

// ClearSelection resets the selection state.
func (e *Editor) ClearSelection() {
	if e.selectionManager == nil {
		logger.Warnf("Editor.ClearSelection called before selectionManager initialized")
		return
	}
	e.selectionManager.ClearSelection()
}

// StartOrUpdateSelection manages selection state during movement.
func (e *Editor) StartOrUpdateSelection() {
	if e.selectionManager == nil {
		logger.Warnf("Editor.StartOrUpdateSelection called before selectionManager initialized")
		return
	}
	e.selectionManager.StartOrUpdateSelection()
}

// IsLinewise returns true if the current selection is line-wise.
func (e *Editor) IsLinewise() bool {
	if e.selectionManager == nil {
		return false
	}
	return e.selectionManager.IsLinewise()
}

// SetLinewise marks the current selection as line-wise (Vim 'V' behaviour).
func (e *Editor) SetLinewise(lw bool) {
	if e.selectionManager == nil {
		return
	}
	e.selectionManager.SetLinewise(lw)
}

// IsBlockwise returns whether the selection is block-wise (Vim Ctrl+V mode).
func (e *Editor) IsBlockwise() bool {
	if e.selectionManager == nil {
		return false
	}
	return e.selectionManager.IsBlockwise()
}

// SetBlockwise marks the current selection as block-wise (Vim Ctrl+V behaviour).
func (e *Editor) SetBlockwise(bw bool) {
	if e.selectionManager == nil {
		return
	}
	e.selectionManager.SetBlockwise(bw)
}

// GetBlockRange returns the normalized block selection as a rectangle.
func (e *Editor) GetBlockRange() (startLine, endLine, startCol, endCol int) {
	if e.selectionManager == nil {
		return -1, -1, -1, -1
	}
	return e.selectionManager.GetBlockRange()
}

// --- Cursor Methods ---

// SetCursor sets the cursor position
func (e *Editor) SetCursor(pos types.Position) {
	if e.cursorManager == nil {
		logger.Warnf("Editor.SetCursor called before cursorManager initialized")
		return
	}
	e.cursorManager.SetPosition(pos)
	// Selection update during movement is handled in editor_methods.go/MoveCursor
}

// ScrollToCursor ensures cursor remains visible
func (e *Editor) ScrollToCursor() {
	if e.cursorManager != nil {
		e.cursorManager.ScrollToCursor()
	}
}

// --- Search Highlighting Methods (Delegated to Find Manager) ---

// HasHighlights checks for search highlights.
func (e *Editor) HasHighlights() bool {
	if e.findManager == nil {
		logger.Warnf("Editor.HasHighlights called before findManager initialized")
		return false
	}
	return e.findManager.HasHighlights()
}

// ClearHighlights clears search highlights.
func (e *Editor) ClearHighlights() {
	if e.findManager == nil {
		logger.Warnf("Editor.ClearHighlights called before findManager initialized")
		return
	}
	e.findManager.ClearHighlights()
}

// GetCurrentSearchHighlights delegates to findManager
func (e *Editor) GetCurrentSearchHighlights() []types.HighlightRegion {
	if e.findManager == nil {
		return nil
	}
	return e.findManager.GetHighlights()
}

// --- Syntax Highlighting Methods (Delegated to Highlight Manager) ---

// UpdateSyntaxHighlights tells the highlight manager to update its internal state.
// This might be called by the highlight manager's background task.
func (e *Editor) UpdateSyntaxHighlights(newHighlights hl.HighlightResult, newTree *sitter.Tree) {
	if e.highlightManager == nil {
		logger.Warnf("Editor.UpdateSyntaxHighlights called before highlightManager initialized")
		return
	}
	e.highlightManager.UpdateHighlights(newHighlights, newTree)
}

// GetSyntaxHighlightsForLine returns highlights for a specific line from the manager.
func (e *Editor) GetSyntaxHighlightsForLine(lineNum int) []types.StyledRange {
	if e.highlightManager == nil {
		return nil
	}
	return e.highlightManager.GetHighlightsForLine(lineNum)
}

// GetHighlightManager returns the highlight manager instance. Needed for App to call AccumulateEdit.
func (e *Editor) GetHighlightManager() *highlight.Manager {
	return e.highlightManager
}

// FilePath returns the file path from the buffer. Required by highlight manager.
func (e *Editor) FilePath() string {
	return e.buffer.FilePath()
}

// --- View Size ---

// SetViewSize updates the view dimensions
func (e *Editor) SetViewSize(width, height int) {
	e.viewWidth = width

	// Calculate the usable height (excluding status bar)
	adjustedHeight := height
	cfg := config.Get() // Get config for status bar height
	if adjustedHeight > cfg.Editor.StatusBarHeight {
		adjustedHeight -= cfg.Editor.StatusBarHeight
	} else {
		adjustedHeight = 0
	}
	e.viewHeight = adjustedHeight

	// Inform the cursor manager of the new view size.
	// Pass adjustedHeight (usable text area height) so ScrollToCursor and
	// PageMove use the correct boundary — not the full terminal height which
	// would include the status bar row(s).
	if e.cursorManager != nil {
		e.cursorManager.SetViewSize(width, adjustedHeight)
	}
}

// --- History Methods ---

// GetHistoryManager returns the history manager for undo/redo
func (e *Editor) GetHistoryManager() *history.Manager {
	return e.historyManager
}

// Undo reverts the last text change
func (e *Editor) Undo() (bool, error) {
	if e.historyManager == nil {
		logger.Warnf("Editor.Undo: historyManager is nil")
		return false, errors.New("history manager not initialized")
	}
	return e.historyManager.Undo()
}

// Redo reapplies a previously undone change
func (e *Editor) Redo() (bool, error) {
	if e.historyManager == nil {
		logger.Warnf("Editor.Redo: historyManager is nil")
		return false, errors.New("history manager not initialized")
	}
	return e.historyManager.Redo()
}

// ClearHistory clears the undo/redo stack
func (e *Editor) ClearHistory() {
	if e.historyManager == nil {
		logger.Warnf("Editor.ClearHistory: historyManager is nil")
		return
	}
	e.historyManager.Clear()
}

// --- Find Manager Access ---

// GetFindManager provides direct access to the find manager
func (e *Editor) GetFindManager() *find.Manager {
	return e.findManager
}

// --- Scroll Offset ---

// ScrollOff returns the scrolloff setting
func (e *Editor) ScrollOff() int {
	return e.scrollOff
}

// --- Dirty-Line Tracking ---

// MarkDirty marks a specific buffer line as needing a redraw.
func (e *Editor) MarkDirty(line int) {
	if e.dirtyLines == nil {
		e.dirtyLines = make(map[int]struct{})
	}
	e.dirtyLines[line] = struct{}{}
}

// MarkAllDirty signals that every visible line must be redrawn.
// This is used on viewport scroll, theme change, search highlight update, etc.
func (e *Editor) MarkAllDirty() {
	e.forceFullRedraw = true
}

// IsDirty reports whether the given buffer line needs to be redrawn.
// Returns true also when a full redraw has been requested.
func (e *Editor) IsDirty(line int) bool {
	if e.forceFullRedraw {
		return true
	}
	if e.dirtyLines == nil {
		return false
	}
	_, ok := e.dirtyLines[line]
	return ok
}

// NeedsFullRedraw reports whether the entire viewport should be cleared and redrawn.
func (e *Editor) NeedsFullRedraw() bool {
	return e.forceFullRedraw
}

// ClearDirty resets all dirty-line tracking state after a frame has been drawn.
func (e *Editor) ClearDirty() {
	e.forceFullRedraw = false
	if e.dirtyLines != nil {
		e.dirtyLines = make(map[int]struct{})
	}
}
