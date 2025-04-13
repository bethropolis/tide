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
	GetBuffer() buffer.Buffer // Still needed to get the initial bytes
	GetCurrentTree() *sitter.Tree
	UpdateSyntaxHighlights(highlights hl.HighlightResult, tree *sitter.Tree)
	FilePath() string
}

// Manager handles debounced asynchronous syntax highlighting.
type Manager struct {
	editor           EditorInterface
	highlighter      *hl.Highlighter
	appRedraw        func()
	mutex            sync.RWMutex // Protects syntaxHighlights, syntaxTree
	debMutex         sync.Mutex   // Protects debouncer state (timer, pending*, isRunning)
	timer            *time.Timer
	pendingCtx       context.Context
	cancelFunc       context.CancelFunc
	isRunning        bool
	pendingEdits     []types.EditInfo
	syntaxHighlights hl.HighlightResult
	syntaxTree       *sitter.Tree
}

// NewManager creates a new highlight manager.
func NewManager(editor EditorInterface, highlighter *hl.Highlighter, redrawFunc func()) *Manager {
	if redrawFunc == nil {
		redrawFunc = func() { logger.Warnf("Highlight Manager: redrawFunc is nil!") }
	}
	return &Manager{
		editor:           editor,
		highlighter:      highlighter,
		appRedraw:        redrawFunc,
		pendingEdits:     make([]types.EditInfo, 0, 5),
		syntaxHighlights: make(hl.HighlightResult),
	}
}

// AccumulateEdit adds an edit to the pending list and triggers/resets the timer.
func (m *Manager) AccumulateEdit(edit types.EditInfo) {
	m.debMutex.Lock() // Use debouncer mutex
	defer m.debMutex.Unlock()

	m.pendingEdits = append(m.pendingEdits, edit)
	logger.DebugTagf("highlight", "HighlightManager: Accumulated edit: %+v", edit)

	if m.timer != nil {
		m.timer.Reset(DebounceHighlightDuration)
		logger.DebugTagf("highlight", "HighlightManager: Debounce timer reset.")
		return
	}
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
	ctx := m.pendingCtx // Capture context
	m.pendingCtx = nil
	m.cancelFunc = nil

	// --- Capture Edits ---
	editsToProcess := make([]types.EditInfo, len(m.pendingEdits))
	copy(editsToProcess, m.pendingEdits)
	m.pendingEdits = m.pendingEdits[:0] // Clear pending edits

	// --- Snapshot Buffer Data (Under Lock) ---
	// Get the buffer ONCE under lock and snapshot its bytes.
	currentBuffer := m.editor.GetBuffer()
	snapshotBytes := currentBuffer.Bytes() // <<< Create snapshot
	filePath := m.editor.FilePath()        // Get file path

	m.debMutex.Unlock() // Unlock debouncer state *before* starting goroutine

	logger.DebugTagf("highlight", "HighlightManager: Debounce finished, starting background highlight task for %d edits...", len(editsToProcess))

	// --- Start Background Goroutine ---
	// Pass the snapshot []byte instead of the buffer interface
	go func(snapshot []byte, fp string, edits []types.EditInfo, taskCtx context.Context) {
		defer func() {
			m.debMutex.Lock()
			m.isRunning = false
			logger.DebugTagf("highlight", "HighlightManager: Background highlight task finished.")
			m.debMutex.Unlock()
		}()

		// --- Get Old Tree and Apply Edits ---
		oldTree := m.GetCurrentTree() // Get tree safely
		if oldTree != nil {
			for _, edit := range edits {
				inputEdit := sitter.EditInput{
					StartIndex: edit.StartIndex, OldEndIndex: edit.OldEndIndex, NewEndIndex: edit.NewEndIndex,
					StartPoint: edit.StartPosition, OldEndPoint: edit.OldEndPosition, NewEndPoint: edit.NewEndPosition,
				}
				logger.DebugTagf("highlight", "HighlightManager: Applying edit to tree: %+v", inputEdit)
				oldTree.Edit(inputEdit)
			}
		} else {
			logger.DebugTagf("highlight", "HighlightManager: No previous tree found, performing full parse.")
		}

		// --- Get Language AND Query bytes ---
		lang, queryBytes := m.highlighter.GetLanguage(fp)
		if lang == nil {
			logger.DebugTagf("highlight", "HighlightManager: No language detected for '%s', clearing highlights.", fp)
			m.UpdateHighlights(make(hl.HighlightResult), nil)
			m.appRedraw()
			return
		}

		// --- Perform Highlighting (Pass snapshot bytes) ---
		newHighlights, newTree, err := m.highlighter.HighlightBuffer(taskCtx, snapshot, lang, queryBytes, oldTree) // <<< Pass snapshot

		if err != nil {
			if taskCtx.Err() == context.Canceled {
				logger.DebugTagf("highlight", "HighlightManager: Highlight task cancelled.")
			} else {
				logger.Warnf("HighlightManager: Background highlighting failed: %v", err)
				m.UpdateHighlights(make(hl.HighlightResult), nil) // Clear highlights
			}
			m.appRedraw()
			return
		}

		logger.DebugTagf("highlight", "HighlightManager: Background task generated %d lines of highlights.", len(newHighlights))
		m.UpdateHighlights(newHighlights, newTree)
		m.appRedraw()

	}(snapshotBytes, filePath, editsToProcess, ctx) // <<< Pass snapshotBytes
}

// Shutdown cancels any pending/running tasks.
func (m *Manager) Shutdown() {
	m.debMutex.Lock()
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

	m.mutex.Lock()
	if m.syntaxTree != nil {
		m.syntaxTree.Close()
		m.syntaxTree = nil
	}
	m.mutex.Unlock()
}

// UpdateHighlights updates syntax highlights with thread safety.
func (m *Manager) UpdateHighlights(newHighlights hl.HighlightResult, newTree *sitter.Tree) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.syntaxTree != nil && m.syntaxTree != newTree {
		m.syntaxTree.Close()
	}
	m.syntaxHighlights = newHighlights
	m.syntaxTree = newTree
	logger.DebugTagf("highlight", "HighlightManager state updated. Tree: %p", newTree)
}

// GetHighlightsForLine gets syntax highlights for a line (thread-safe).
func (m *Manager) GetHighlightsForLine(lineNum int) []types.StyledRange {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

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
	return m.syntaxTree
}

// ClearHighlights explicitly clears the syntax highlighting state.
func (m *Manager) ClearHighlights() {
	m.UpdateHighlights(make(hl.HighlightResult), nil)
}

// --- Search Highlighting Methods (Stubs - To be removed or delegated if needed later) ---

func (m *Manager) HighlightSearchMatches(term string) {
	logger.Warnf("HighlightSearchMatches called on highlight.Manager - should be moved to find.Manager")
}
func (m *Manager) HasHighlights() bool                    { return false } // Syntax manager doesn't own search highlights
func (m *Manager) GetHighlights() []types.HighlightRegion { return nil }
