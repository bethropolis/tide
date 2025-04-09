package core

import (
	"sync"

	"errors"

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/config"
	"github.com/bethropolis/tide/internal/core/clipboard"
	"github.com/bethropolis/tide/internal/core/cursor" // Add history import
	"github.com/bethropolis/tide/internal/core/find"
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
	findManager      *find.Manager    // Add find manager field
}

// NewEditor creates a new Editor instance with a given buffer.
func NewEditor(buf buffer.Buffer) *Editor {
	e := &Editor{
		buffer:           buf,
		scrollOff:        config.Get().Editor.ScrollOff,
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
	e.findManager = find.NewManager(e)                                  // Initialize find manager

	logger.DebugTagf("core", "Editor created and managers initialized.")
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
// Returns two invalid positions and false if no selection is active.
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
// Called typically when a Shift + movement key is pressed.
func (e *Editor) StartOrUpdateSelection() {
	if e.selectionManager == nil {
		logger.Warnf("Editor.StartOrUpdateSelection called before selectionManager initialized")
		return
	}
	e.selectionManager.StartOrUpdateSelection()
}

// SetCursor sets the cursor position
func (e *Editor) SetCursor(pos types.Position) {
	if e.cursorManager == nil {
		logger.Warnf("Editor.SetCursor called before cursorManager initialized")
		return
	}
	e.cursorManager.SetPosition(pos)
}

// ScrollToCursor ensures cursor remains visible
func (e *Editor) ScrollToCursor() {
	if e.cursorManager != nil {
		e.cursorManager.ScrollToCursor()
	}
}

func (e *Editor) HasHighlights() bool {
	if e.findManager == nil {
		logger.Warnf("Editor.HasHighlights called before findManager initialized")
		return false
	}
	return e.findManager.HasHighlights()
}

func (e *Editor) ClearHighlights() {
	if e.findManager == nil {
		logger.Warnf("Editor.ClearHighlights called before findManager initialized")
		return
	}
	e.findManager.ClearHighlights()
}

func (e *Editor) GetHighlights() []types.HighlightRegion {
	if e.findManager == nil {
		logger.Warnf("Editor.GetHighlights called before findManager initialized")
		return nil
	}
	return e.findManager.GetHighlights()
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
	logger.DebugTagf("core", "Editor: TriggerSyntaxHighlight called (handled by app manager)")
}

// SetViewSize updates the view dimensions
func (e *Editor) SetViewSize(width, height int) {
	e.viewWidth = width

	// Calculate the usable height (excluding status bar)
	adjustedHeight := height
	if adjustedHeight > config.Get().Editor.StatusBarHeight {
		adjustedHeight -= config.Get().Editor.StatusBarHeight
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

// GetCurrentSearchHighlights delegates to findManager
func (e *Editor) GetCurrentSearchHighlights() []types.HighlightRegion {
	if e.findManager == nil {
		return nil
	}
	return e.findManager.GetHighlights()
}

// GetFindManager provides direct access to the find manager
func (e *Editor) GetFindManager() *find.Manager {
	return e.findManager
}

// ScrollOff returns the scrolloff setting (lines to keep visible above/below cursor)
func (e *Editor) ScrollOff() int {
	return e.scrollOff
}
