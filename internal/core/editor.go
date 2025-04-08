package core

import (
	"sync"

	"errors"

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/config"
	"github.com/bethropolis/tide/internal/core/clipboard"
	"github.com/bethropolis/tide/internal/core/cursor" // Add history import
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
	Cursor     types.Position
	viewWidth  int // Cached terminal width
	viewHeight int // Cached terminal height (excluding status bar)
	ScrollOff  int // Number of lines to keep visible above/below cursor

	// Selection State
	selecting      bool           // True if selection is active
	selectionStart types.Position // Anchor point of the selection
	selectionEnd   types.Position // Other end of the selection (usually Cursor position)

	// Clipboard State
	clipboard []byte // Internal register for yank/put

	// Event Manager
	eventManager *event.Manager // Added for dispatching events on delete etc.

	// Find State
	searchHighlights []types.HighlightRegion // Store regions to highlight (renamed from 'highlights')

	// Syntax Highlighting State
	highlighter      *hl.Highlighter    // Highlighter service instance
	syntaxHighlights hl.HighlightResult // Store computed syntax highlights
	syntaxTree       *sitter.Tree       // Store the current syntax tree
	highlightMutex   sync.RWMutex       // Mutex to protect syntaxHighlights & syntaxTree

	// Sub-system Managers
	clipboardManager *clipboard.Manager
	cursorManager    *cursor.Manager
	selectionManager *selection.Manager
	textOps          *text.Operations
	historyManager   *history.Manager // Add history manager
}

// NewEditor creates a new Editor instance with a given buffer.
func NewEditor(buf buffer.Buffer) *Editor {
	e := &Editor{
		buffer:           buf,
		Cursor:           types.Position{Line: 0, Col: 0},
		ScrollOff:        config.DefaultScrollOff,
		selecting:        false,
		selectionStart:   types.Position{Line: -1, Col: -1},
		selectionEnd:     types.Position{Line: -1, Col: -1},
		clipboard:        nil,
		searchHighlights: make([]types.HighlightRegion, 0),
		syntaxHighlights: make(hl.HighlightResult),
		syntaxTree:       nil,
		highlightMutex:   sync.RWMutex{},
	}

	// Initialize all managers directly in NewEditor
	e.textOps = text.NewOperations(e)
	e.cursorManager = cursor.NewManager(e)
	e.selectionManager = selection.NewManager(e)
	e.clipboardManager = clipboard.NewManager(e)
	e.historyManager = history.NewManager(e, history.DefaultMaxHistory) // Initialize history manager

	logger.Debugf("Editor created and managers initialized.")
	return e
}

// SetEventManager sets the event manager for dispatching events
func (e *Editor) SetEventManager(mgr *event.Manager) {
	e.eventManager = mgr
}

// SetHighlighter injects the highlighter service.
func (e *Editor) SetHighlighter(h *hl.Highlighter) {
	e.highlighter = h
}

// GetBuffer returns the editor's buffer.
func (e *Editor) GetBuffer() buffer.Buffer {
	return e.buffer
}

// GetCursor returns the current cursor position.
func (e *Editor) GetCursor() types.Position {
	// For now, we return the Editor's own cursor field
	// Eventually, this could delegate to cursorManager.GetPosition()
	return e.Cursor
}

// GetViewport returns the viewport position by delegating to the cursor manager
func (e *Editor) GetViewport() (int, int) {
	if e.cursorManager == nil {
		logger.Warnf("Editor.GetViewport called before cursorManager initialized")
		return 0, 0 // Fallback
	}
	return e.cursorManager.GetViewport()
}

// GetCurrentTree safely gets the current tree (needed for incremental parse)
func (e *Editor) GetCurrentTree() *sitter.Tree {
	e.highlightMutex.RLock()
	defer e.highlightMutex.RUnlock()
	return e.syntaxTree
}

// GetEventManager returns the event manager
func (e *Editor) GetEventManager() *event.Manager {
	return e.eventManager
}

// GetSelecting returns whether selection is active
func (e *Editor) GetSelecting() bool {
	return e.selecting
}

// HasSelection returns whether there's an active selection
func (e *Editor) HasSelection() bool {
	return e.selecting && (e.selectionStart != e.selectionEnd)
}

// GetSelection returns the normalized selection range (start <= end).
// Returns two invalid positions and false if no selection is active.
func (e *Editor) GetSelection() (start types.Position, end types.Position, ok bool) {
	if !e.HasSelection() {
		return types.Position{Line: -1, Col: -1}, types.Position{Line: -1, Col: -1}, false
	}

	start = e.selectionStart
	end = e.selectionEnd

	// Normalize: Ensure start is lexicographically before or equal to end
	if start.Line > end.Line || (start.Line == end.Line && start.Col > end.Col) {
		start, end = end, start // Swap
	}
	return start, end, true
}

// ClearSelection resets the selection state.
func (e *Editor) ClearSelection() {
	if e.selecting {
		e.selecting = false
		// Optionally reset positions, or keep them for potential re-activation? Reset for now.
		e.selectionStart = types.Position{Line: -1, Col: -1}
		e.selectionEnd = types.Position{Line: -1, Col: -1}
		logger.Debugf("Editor: Selection cleared") // Debug log
	}
}

// StartOrUpdateSelection manages selection state during movement.
// Called typically when a Shift + movement key is pressed.
func (e *Editor) StartOrUpdateSelection() {
	if !e.selecting {
		// Start a new selection - anchor at the *current* cursor position *before* movement
		e.selectionStart = e.Cursor
		e.selecting = true
		logger.Debugf("Editor: Selection started at %v", e.selectionStart) // Debug log
	}
	// The *other* end of the selection always follows the cursor during selection movement
	e.selectionEnd = e.Cursor // Update end to current cursor position
}

// SetCursor sets the cursor position
func (e *Editor) SetCursor(pos types.Position) {
	e.Cursor = pos // Update the editor's internal cursor state

	// Update the cursor manager with the new position
	if e.cursorManager != nil {
		e.cursorManager.SetPosition(pos) // This will handle clamping and scrolling
		// Sync back the possibly clamped position
		e.Cursor = e.cursorManager.GetPosition()
	}

	// Update selection end if we're selecting
	if e.selecting {
		e.selectionEnd = e.Cursor
	}
}

// ScrollToCursor ensures cursor remains visible
func (e *Editor) ScrollToCursor() {
	if e.cursorManager != nil {
		e.cursorManager.ScrollToCursor()
	}
}

func (e *Editor) HasHighlights() bool {
	return len(e.searchHighlights) > 0
}

func (e *Editor) ClearHighlights() {
	if len(e.searchHighlights) > 0 {
		logger.Debugf("Editor: Clearing %d highlights", len(e.searchHighlights))
		e.searchHighlights = e.searchHighlights[:0]
	}
}

func (e *Editor) GetHighlights() []types.HighlightRegion {
	return e.searchHighlights
}

// UpdateSyntaxHighlights updates the highlighting
func (e *Editor) UpdateSyntaxHighlights(newHighlights hl.HighlightResult, newTree *sitter.Tree) {
	e.highlightMutex.Lock()
	defer e.highlightMutex.Unlock()

	// Close old tree if it exists
	if e.syntaxTree != nil {
		e.syntaxTree.Close() // Call Close() directly
	}

	e.syntaxHighlights = newHighlights
	e.syntaxTree = newTree
}

// TriggerSyntaxHighlight triggers a highlight operation
func (e *Editor) TriggerSyntaxHighlight() {
	logger.Debugf("Editor: TriggerSyntaxHighlight called (handled by app manager)")
}

// SetViewSize updates the view dimensions
func (e *Editor) SetViewSize(width, height int) {
	e.viewWidth = width

	// Calculate the usable height (excluding status bar)
	adjustedHeight := height
	if adjustedHeight > config.StatusBarHeight {
		adjustedHeight -= config.StatusBarHeight
	} else {
		adjustedHeight = 0
	}
	e.viewHeight = adjustedHeight

	// Inform the cursor manager of the new view size
	if e.cursorManager != nil {
		e.cursorManager.SetViewSize(width, height)
	}
}

// GetSyntaxHighlightsForLine returns highlights for a specific line
func (e *Editor) GetSyntaxHighlightsForLine(lineNum int) []types.StyledRange {
	e.highlightMutex.RLock()
	defer e.highlightMutex.RUnlock()

	if styles, ok := e.syntaxHighlights[lineNum]; ok {
		return styles
	}
	return nil
}

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
