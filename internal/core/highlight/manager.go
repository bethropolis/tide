package highlight

import (
	"context"
	"sync"
	"time"

	"github.com/bethropolis/tide/internal/buffer"
	hl "github.com/bethropolis/tide/internal/highlighter"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// Debounce duration for highlighting updates
const DebounceHighlightDuration = 75 * time.Millisecond

// EditorInterface defines methods the highlight manager needs from the editor.
type EditorInterface interface {
	GetBuffer() buffer.Buffer
	GetCurrentTree() *sitter.Tree
	UpdateSyntaxHighlights(highlights hl.HighlightResult, tree *sitter.Tree)
	FilePath() string // Need file path for language detection
}

// Manager handles debounced asynchronous syntax highlighting.
type Manager struct {
	editor       EditorInterface    // Interface to access editor state needed for highlighting
	highlighter  *hl.Highlighter    // The tree-sitter highlighting service
	appRedraw    func()             // Function to request app redraw
	mutex        sync.RWMutex       // Protects internal state below
	debMutex     sync.Mutex         // Separate mutex for debouncer state
	timer        *time.Timer        // Debounce timer
	pendingCtx   context.Context    // Context for the pending update
	cancelFunc   context.CancelFunc // Function to cancel pending update
	isRunning    bool               // Is a background task currently running?
	pendingEdits []types.EditInfo   // Store pending edits

	// State owned by the manager
	syntaxHighlights hl.HighlightResult      // Store computed syntax highlights
	syntaxTree       *sitter.Tree            // Store the current syntax tree
	searchHighlights []types.HighlightRegion // Store search highlight regions
}

// NewManager creates a new highlight manager.
func NewManager(editor EditorInterface, highlighter *hl.Highlighter, redrawFunc func()) *Manager {
	if redrawFunc == nil {
		// Provide a no-op function if nil is passed, although it shouldn't happen
		redrawFunc = func() { logger.Warnf("Highlight Manager: redrawFunc is nil!") }
	}
	return &Manager{
		editor:           editor,
		highlighter:      highlighter,
		appRedraw:        redrawFunc,
		pendingEdits:     make([]types.EditInfo, 0, 5), // Initialize slice with capacity
		syntaxHighlights: make(hl.HighlightResult),     // Initialize map
		searchHighlights: make([]types.HighlightRegion, 0),
		// syntaxTree is initially nil
	}
}

// AccumulateEdit adds an edit to the pending list and triggers/resets the timer.
func (m *Manager) AccumulateEdit(edit types.EditInfo) {
	m.debMutex.Lock() // Use debouncer mutex
	defer m.debMutex.Unlock()

	// Add edit to list
	m.pendingEdits = append(m.pendingEdits, edit)
	logger.DebugTagf("highlight", "HighlightManager: Accumulated edit: %+v", edit)

	// Reset or start timer (debouncing)
	if m.timer != nil {
		m.timer.Reset(DebounceHighlightDuration)
		logger.DebugTagf("highlight", "HighlightManager: Debounce timer reset.")
		return
	}
	// Cancel previous context if timer wasn't running but a task might be pending cancellation
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	m.pendingCtx, m.cancelFunc = context.WithCancel(context.Background())
	logger.DebugTagf("highlight", "HighlightManager: Starting debounce timer (%v).", DebounceHighlightDuration)
	m.timer = time.AfterFunc(DebounceHighlightDuration, m.runHighlightUpdate)
}

// runHighlightUpdate applies pending edits and starts background task.
func (m *Manager) runHighlightUpdate() {
	m.debMutex.Lock() // Lock debouncer state
	m.timer = nil     // Timer fired

	if m.isRunning {
		logger.DebugTagf("highlight", "HighlightManager: Update skipped, another highlight task is already running.")
		m.debMutex.Unlock()
		return
	}

	if len(m.pendingEdits) == 0 {
		logger.DebugTagf("highlight", "HighlightManager: No pending edits, skipping highlight run.")
		m.debMutex.Unlock()
		return
	}

	m.isRunning = true
	ctx := m.pendingCtx
	m.pendingCtx = nil
	m.cancelFunc = nil

	// --- Capture pending edits and clear the list ---
	editsToProcess := make([]types.EditInfo, len(m.pendingEdits))
	copy(editsToProcess, m.pendingEdits)
	m.pendingEdits = m.pendingEdits[:0] // Clear pending edits efficiently

	// --- Get state needed for background task (Snapshot under lock) ---
	currentBuffer := m.editor.GetBuffer() // Get buffer interface
	filePath := m.editor.FilePath()       // Get file path via interface

	m.debMutex.Unlock() // Unlock debouncer state before starting goroutine

	logger.DebugTagf("highlight", "HighlightManager: Debounce finished, starting background highlight task for %d edits...", len(editsToProcess))

	// --- Start Background Goroutine ---
	go func(buf buffer.Buffer, fp string, edits []types.EditInfo, taskCtx context.Context) {
		defer func() {
			m.debMutex.Lock() // Lock debouncer state to update isRunning
			m.isRunning = false
			logger.DebugTagf("highlight", "HighlightManager: Background highlight task finished.")
			m.debMutex.Unlock()
		}()

		// --- Get Old Tree and Apply Edits ---
		oldTree := m.GetCurrentTree() // Get tree safely using internal method
		if oldTree != nil {
			for _, edit := range edits {
				// Fix field names according to the go-tree-sitter library's API
				inputEdit := sitter.EditInput{
					StartIndex:  edit.StartIndex,
					OldEndIndex: edit.OldEndIndex,
					NewEndIndex: edit.NewEndIndex,
					StartPoint:  edit.StartPosition,
					OldEndPoint: edit.OldEndPosition,
					NewEndPoint: edit.NewEndPosition,
				}
				logger.DebugTagf("highlight", "HighlightManager: Applying edit to tree: %+v", inputEdit)
				oldTree.Edit(inputEdit) // APPLY EDIT TO TREE
			}
		} else {
			logger.DebugTagf("highlight", "HighlightManager: No previous tree found, performing full parse.")
		}

		// --- Get Language AND Query bytes ---
		lang, queryBytes := m.highlighter.GetLanguage(fp) // Get both language and query
		if lang == nil {
			logger.DebugTagf("highlight", "HighlightManager: No language detected for '%s', clearing highlights.", fp)
			m.UpdateHighlights(make(hl.HighlightResult), nil) // Update manager's state
			m.appRedraw()
			return
		}

		// --- Perform Highlighting ---
		// Pass language AND query bytes to HighlightBuffer
		newHighlights, newTree, err := m.highlighter.HighlightBuffer(taskCtx, buf, lang, queryBytes, oldTree)

		// Check for errors or cancellation
		if err != nil {
			if taskCtx.Err() == context.Canceled {
				logger.DebugTagf("highlight", "HighlightManager: Highlight task cancelled.")
			} else {
				logger.Warnf("HighlightManager: Background highlighting failed: %v", err)
				m.UpdateHighlights(make(hl.HighlightResult), nil) // Clear highlights on error
			}
			m.appRedraw() // Request redraw even on error/cancel to clear highlights
			return
		}

		logger.DebugTagf("highlight", "HighlightManager: Background task generated %d lines of highlights.", len(newHighlights))
		m.UpdateHighlights(newHighlights, newTree) // Update manager's state
		m.appRedraw()                              // Request redraw after successful update

	}(currentBuffer, filePath, editsToProcess, ctx)
}

// Shutdown cancels any pending/running tasks.
func (m *Manager) Shutdown() {
	m.debMutex.Lock() // Lock debouncer state
	defer m.debMutex.Unlock()
	if m.cancelFunc != nil {
		logger.DebugTagf("highlight", "HighlightingManager: Shutting down, cancelling pending/running task.")
		m.cancelFunc()
		m.cancelFunc = nil
	}
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	// Also close the current syntax tree if it exists
	m.mutex.Lock()
	if m.syntaxTree != nil {
		m.syntaxTree.Close()
		m.syntaxTree = nil
	}
	m.mutex.Unlock()
}

// --- Methods to access highlighting state ---

// UpdateHighlights updates syntax highlights with thread safety. Called internally or by initial sync highlight.
func (m *Manager) UpdateHighlights(newHighlights hl.HighlightResult, newTree *sitter.Tree) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Close old tree if needed
	if m.syntaxTree != nil && m.syntaxTree != newTree { // Avoid closing the tree we just received
		m.syntaxTree.Close()
	}

	m.syntaxHighlights = newHighlights
	m.syntaxTree = newTree // Store directly as concrete type
	logger.DebugTagf("highlight", "HighlightManager state updated. Tree: %p", newTree)
}

// GetHighlightsForLine gets syntax highlights for a line (thread-safe).
func (m *Manager) GetHighlightsForLine(lineNum int) []types.StyledRange {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Return a copy of the slice to prevent race conditions if the caller modifies it
	if styles, ok := m.syntaxHighlights[lineNum]; ok {
		result := make([]types.StyledRange, len(styles))
		copy(result, styles)
		return result
	}
	return nil
}

// GetCurrentTree gets the current syntax tree (thread-safe).
func (m *Manager) GetCurrentTree() *sitter.Tree {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	// Note: Returning the tree directly. The caller must not modify it.
	return m.syntaxTree
}

// ClearHighlights explicitly clears the syntax highlighting state.
func (m *Manager) ClearHighlights() {
	m.UpdateHighlights(make(hl.HighlightResult), nil)
}

// --- Search Highlighting Methods (will be moved to find manager) ---

// HighlightSearchMatches adds search highlights
func (m *Manager) HighlightSearchMatches(term string) {
	// This method will move to find manager, keeping stub for compatibility
	logger.Warnf("HighlightSearchMatches called on highlight.Manager - should be moved to find.Manager")
}

// HasHighlights checks if there are active highlights
func (m *Manager) HasHighlights() bool {
	return len(m.searchHighlights) > 0
}

// GetHighlights returns all search highlights
func (m *Manager) GetHighlights() []types.HighlightRegion {
	return m.searchHighlights
}
