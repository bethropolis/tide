package app

import (
	"context"
	"sync"
	"time"

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/highlighter"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// Reduce debounce duration from 200ms to 75ms for more responsive highlighting
const highlightDebounceDuration = 65 * time.Millisecond

// HighlightingManager handles debounced asynchronous syntax highlighting.
type HighlightingManager struct {
	editor      *core.Editor
	highlighter *highlighter.Highlighter
	appRedraw   func() // Function to request app redraw

	mu           sync.Mutex // Protects timer and pending state
	timer        *time.Timer
	pendingCtx   context.Context    // Context for the pending update
	cancelFunc   context.CancelFunc // Function to cancel pending update
	isRunning    bool               // Is a background task currently running?
	pendingEdits []types.EditInfo   // Store pending edits
}

// NewHighlightingManager creates a manager.
func NewHighlightingManager(editor *core.Editor, highlighter *highlighter.Highlighter, redrawFunc func()) *HighlightingManager {
	return &HighlightingManager{
		editor:       editor,
		highlighter:  highlighter,
		appRedraw:    redrawFunc,
		pendingEdits: make([]types.EditInfo, 0, 5), // Initialize slice with capacity
	}
}

// AccumulateEdit adds an edit to the pending list and triggers/resets the timer.
func (hm *HighlightingManager) AccumulateEdit(edit types.EditInfo) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	// Add edit to list
	hm.pendingEdits = append(hm.pendingEdits, edit)
	logger.Debugf("HighlightingManager: Accumulated edit: %+v", edit)

	// Reset or start timer (debouncing)
	if hm.timer != nil {
		hm.timer.Reset(highlightDebounceDuration)
		logger.Debugf("HighlightingManager: Debounce timer reset.")
		return
	}
	if hm.cancelFunc != nil {
		hm.cancelFunc() // Cancel previous context if timer wasn't running
	}
	hm.pendingCtx, hm.cancelFunc = context.WithCancel(context.Background())
	logger.Debugf("HighlightingManager: Starting debounce timer (%v).", highlightDebounceDuration)
	hm.timer = time.AfterFunc(highlightDebounceDuration, hm.runHighlightUpdate)
}

// runHighlightUpdate applies pending edits and starts background task.
func (hm *HighlightingManager) runHighlightUpdate() {
	hm.mu.Lock()
	hm.timer = nil // Timer fired

	if hm.isRunning {
		logger.Debugf("HighlightingManager: Update skipped, another highlight task is already running.")
		hm.mu.Unlock()
		return
	}

	if len(hm.pendingEdits) == 0 {
		logger.Debugf("HighlightingManager: No pending edits, skipping highlight run.")
		hm.mu.Unlock()
		return
	}

	hm.isRunning = true
	ctx := hm.pendingCtx
	hm.pendingCtx = nil
	hm.cancelFunc = nil

	// --- Capture pending edits and clear the list ---
	editsToProcess := make([]types.EditInfo, len(hm.pendingEdits))
	copy(editsToProcess, hm.pendingEdits)
	hm.pendingEdits = hm.pendingEdits[:0] // Clear pending edits efficiently

	// --- Get state needed for background task ---
	currentBuffer := hm.editor.GetBuffer()
	var filePath string
	if bufWithFP, ok := currentBuffer.(interface{ FilePath() string }); ok {
		filePath = bufWithFP.FilePath()
	}

	hm.mu.Unlock() // Unlock before starting goroutine

	logger.Debugf("HighlightingManager: Debounce finished, starting background highlight task for %d edits...", len(editsToProcess))

	// --- Start Background Goroutine ---
	go func(buf buffer.Buffer, fp string, edits []types.EditInfo, taskCtx context.Context) {
		defer func() {
			hm.mu.Lock()
			hm.isRunning = false
			logger.Debugf("HighlightingManager: Background highlight task finished.")
			hm.mu.Unlock()
		}()

		// --- Get Old Tree and Apply Edits ---
		oldTree := hm.editor.GetCurrentTree() // Get tree safely
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
				logger.Debugf("HighlightingManager: Applying edit to tree: %+v", inputEdit)
				oldTree.Edit(inputEdit) // APPLY EDIT TO TREE
			}
		} else {
			logger.Debugf("HighlightingManager: No previous tree found, performing full parse.")
		}

		// --- Get Language AND Query bytes ---
		lang, queryBytes := hm.highlighter.GetLanguage(fp) // Get both language and query
		if lang == nil {
			logger.Debugf("HighlightingManager: No language detected for '%s', clearing highlights.", fp)
			hm.editor.UpdateSyntaxHighlights(make(highlighter.HighlightResult), nil)
			hm.appRedraw()
			return
		}

		// --- Perform Highlighting ---
		// Pass language AND query bytes to HighlightBuffer
		newHighlights, newTree, err := hm.highlighter.HighlightBuffer(taskCtx, buf, lang, queryBytes, oldTree)

		// Check for errors or cancellation
		if err != nil {
			if taskCtx.Err() == context.Canceled {
				logger.Debugf("HighlightingManager: Highlight task cancelled.")
			} else {
				logger.Warnf("HighlightingManager: Background highlighting failed: %v", err)
				hm.editor.UpdateSyntaxHighlights(make(highlighter.HighlightResult), nil)
			}
			hm.appRedraw()
			return
		}

		logger.Debugf("HighlightingManager: Background task generated %d lines of highlights.", len(newHighlights))
		hm.editor.UpdateSyntaxHighlights(newHighlights, newTree)
		hm.appRedraw()

	}(currentBuffer, filePath, editsToProcess, ctx)
}

// Shutdown cancels any pending/running tasks.
func (hm *HighlightingManager) Shutdown() {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	if hm.cancelFunc != nil {
		logger.Debugf("HighlightingManager: Shutting down, cancelling pending/running task.")
		hm.cancelFunc()
		hm.cancelFunc = nil
	}
	if hm.timer != nil {
		hm.timer.Stop()
		hm.timer = nil
	}
}

// TriggerUpdate is kept for backward compatibility but now does nothing
// as AccumulateEdit is the new entry point
func (hm *HighlightingManager) TriggerUpdate() {
	logger.Debugf("HighlightingManager: TriggerUpdate called (deprecated, use AccumulateEdit instead)")
}
