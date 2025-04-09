package highlight

import (
	"context"
	"sync"
	"time"

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/highlighter"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// EditorInterface defines methods needed from editor
type EditorInterface interface {
	GetBuffer() buffer.Buffer
	GetCurrentTree() *sitter.Tree
	UpdateSyntaxHighlights(highlights highlighter.HighlightResult, tree *sitter.Tree)
}

// Manager handles debounced asynchronous syntax highlighting
const DebounceHighlightDuration = 65 * time.Millisecond

// Manager handles syntax highlighting
type Manager struct {
	editor      EditorInterface
	highlighter *highlighter.Highlighter
	appRedraw   func() // Function to request app redraw

	mu           sync.Mutex // Protects timer and pending state
	timer        *time.Timer
	pendingCtx   context.Context    // Context for the pending update
	cancelFunc   context.CancelFunc // Function to cancel pending update
	isRunning    bool               // Is a background task currently running?
	pendingEdits []types.EditInfo   // Store pending edits
}

// NewManager creates a new highlighting manager
func NewManager(editor EditorInterface, highlighter *highlighter.Highlighter, redrawFunc func()) *Manager {
	return &Manager{
		editor:       editor,
		highlighter:  highlighter,
		appRedraw:    redrawFunc,
		pendingEdits: make([]types.EditInfo, 0, 5),
	}
}

// AccumulateEdit adds an edit to the pending list and triggers/resets the timer
func (m *Manager) AccumulateEdit(edit types.EditInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Add edit to list
	m.pendingEdits = append(m.pendingEdits, edit)
	logger.DebugTagf("highlight","HighlightingManager: Accumulated edit: %+v", edit)

	// Reset or start timer (debouncing)
	if m.timer != nil {
		m.timer.Reset(DebounceHighlightDuration)
		logger.DebugTagf("highlight","HighlightingManager: Debounce timer reset.")
		return
	}
	if m.cancelFunc != nil {
		m.cancelFunc() // Cancel previous context if timer wasn't running
	}
	m.pendingCtx, m.cancelFunc = context.WithCancel(context.Background())
	logger.DebugTagf("highlight","HighlightingManager: Starting debounce timer (%v).", DebounceHighlightDuration)
	m.timer = time.AfterFunc(DebounceHighlightDuration, m.runHighlightUpdate)
}

// runHighlightUpdate applies pending edits and starts background task
func (m *Manager) runHighlightUpdate() {
	m.mu.Lock()
	m.timer = nil // Timer fired

	if m.isRunning {
		logger.DebugTagf("highlight", "HighlightingManager: Update skipped, another highlight task is already running.")
		m.mu.Unlock()
		return
	}

	if len(m.pendingEdits) == 0 {
		logger.DebugTagf("highlight","HighlightingManager: No pending edits, skipping highlight run.")
		m.mu.Unlock()
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

	// --- Get state needed for background task ---
	currentBuffer := m.editor.GetBuffer()
	var filePath string
	if bufWithFP, ok := currentBuffer.(interface{ FilePath() string }); ok {
		filePath = bufWithFP.FilePath()
	}

	m.mu.Unlock() // Unlock before starting goroutine

	logger.DebugTagf("highlight","HighlightingManager: Debounce finished, starting background highlight task for %d edits...", len(editsToProcess))

	// --- Start Background Goroutine ---
	go func(buf buffer.Buffer, fp string, edits []types.EditInfo, taskCtx context.Context) {
		defer func() {
			m.mu.Lock()
			m.isRunning = false
			logger.DebugTagf("highlight","HighlightingManager: Background highlight task finished.")
			m.mu.Unlock()
		}()

		// --- Get Old Tree and Apply Edits ---
		oldTree := m.editor.GetCurrentTree() // Get tree safely
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
				logger.DebugTagf("highlight","HighlightingManager: Applying edit to tree: %+v", inputEdit)
				oldTree.Edit(inputEdit) // APPLY EDIT TO TREE
			}
		} else {
			logger.DebugTagf("highlight","HighlightingManager: No previous tree found, performing full parse.")
		}

		// --- Get Language AND Query bytes ---
		lang, queryBytes := m.highlighter.GetLanguage(fp) // Get both language and query
		if lang == nil {
			logger.DebugTagf("highlight","HighlightingManager: No language detected for '%s', clearing highlights.", fp)
			m.editor.UpdateSyntaxHighlights(make(highlighter.HighlightResult), nil)
			m.appRedraw()
			return
		}

		// --- Perform Highlighting ---
		// Pass language AND query bytes to HighlightBuffer
		newHighlights, newTree, err := m.highlighter.HighlightBuffer(taskCtx, buf, lang, queryBytes, oldTree)

		// Check for errors or cancellation
		if err != nil {
			if taskCtx.Err() == context.Canceled {
				logger.DebugTagf("highlight","HighlightingManager: Highlight task cancelled.")
			} else {
				logger.Warnf("HighlightingManager: Background highlighting failed: %v", err)
				m.editor.UpdateSyntaxHighlights(make(highlighter.HighlightResult), nil)
			}
			m.appRedraw()
			return
		}

		logger.DebugTagf("highlight","HighlightingManager: Background task generated %d lines of highlights.", len(newHighlights))
		m.editor.UpdateSyntaxHighlights(newHighlights, newTree)
		m.appRedraw()

	}(currentBuffer, filePath, editsToProcess, ctx)
}

// Shutdown cancels any pending/running tasks
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancelFunc != nil {
		logger.DebugTagf("highlight","HighlightingManager: Shutting down, cancelling pending/running task.")
		m.cancelFunc()
		m.cancelFunc = nil
	}
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
}
