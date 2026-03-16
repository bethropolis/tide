package history

import (
	"fmt"
	"sync"

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
)

const DefaultMaxHistory = 100

// EditorInterface defines the methods the history manager needs from the editor/buffer.
type EditorInterface interface {
	GetBuffer() buffer.Buffer
	SetCursor(types.Position)
	GetEventManager() *event.Manager
	ScrollToCursor()
}

// Manager handles the undo/redo stack.
type Manager struct {
	editor       EditorInterface
	changes      []Change
	currentIndex int // Index of the *next* change to potentially Redo
	maxHistory   int
	mutex        sync.Mutex

	// Transaction support
	inTransaction  bool
	transactionBuf []Change // Accumulates sub-changes during an open transaction
}

// NewManager creates a history manager.
func NewManager(editor EditorInterface, maxHistory int) *Manager {
	if maxHistory <= 0 {
		maxHistory = DefaultMaxHistory
	}
	return &Manager{
		editor:       editor,
		changes:      make([]Change, 0, maxHistory),
		currentIndex: 0,
		maxHistory:   maxHistory,
	}
}

// BeginTransaction starts grouping subsequent RecordChange calls into a single
// atomic undo/redo step. Calls to BeginTransaction while a transaction is already
// open are ignored (no nested transactions).
func (m *Manager) BeginTransaction() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.inTransaction {
		logger.Debugf("History: BeginTransaction called while already in transaction — ignored")
		return
	}
	m.inTransaction = true
	m.transactionBuf = m.transactionBuf[:0] // Reset accumulator
	logger.Debugf("History: Transaction started")
}

// EndTransaction finalises the open transaction, recording all accumulated
// sub-changes as a single TransactionAction entry on the undo stack.
// If no changes were accumulated (empty transaction), nothing is recorded.
func (m *Manager) EndTransaction(cursorBefore types.Position) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.inTransaction {
		logger.Debugf("History: EndTransaction called without matching BeginTransaction — ignored")
		return
	}
	m.inTransaction = false

	if len(m.transactionBuf) == 0 {
		logger.Debugf("History: Empty transaction, nothing to record")
		return
	}

	// Build the transaction Change
	txChange := Change{
		Type:         TransactionAction,
		CursorBefore: cursorBefore,
		Children:     make([]Change, len(m.transactionBuf)),
	}
	copy(txChange.Children, m.transactionBuf)
	m.transactionBuf = m.transactionBuf[:0]

	// Truncate redo history
	if m.currentIndex < len(m.changes) {
		m.changes = m.changes[:m.currentIndex]
	}

	m.changes = append(m.changes, txChange)
	if len(m.changes) > m.maxHistory {
		m.changes = m.changes[len(m.changes)-m.maxHistory:]
	}
	m.currentIndex = len(m.changes)

	logger.Debugf("History: Transaction committed with %d sub-change(s). Index: %d", len(txChange.Children), m.currentIndex)
}

// RecordChange adds a new change, clearing any redo history.
// When inside a transaction, the change is buffered instead of committed immediately.
func (m *Manager) RecordChange(change Change) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.inTransaction {
		m.transactionBuf = append(m.transactionBuf, change)
		logger.Debugf("History: Buffered change %v into open transaction (%d so far)", change.Type, len(m.transactionBuf))
		return
	}

	// If current index isn't at the end, truncate the redo history
	if m.currentIndex < len(m.changes) {
		m.changes = m.changes[:m.currentIndex]
	}

	// Add the new change
	m.changes = append(m.changes, change)

	// Limit history size
	if len(m.changes) > m.maxHistory {
		// Remove the oldest change (simple FIFO eviction)
		m.changes = m.changes[len(m.changes)-m.maxHistory:]
		// Adjust index if it was affected (shouldn't be if appending)
	}

	// Update current index to point after the new change
	m.currentIndex = len(m.changes)

	logger.Debugf("History: Recorded change %v. Index: %d, Count: %d", change.Type, m.currentIndex, len(m.changes))
}

// applyChange applies a single Change (insert or delete) to the buffer and returns EditInfo.
func (m *Manager) applyChange(c Change) (types.EditInfo, error) {
	buf := m.editor.GetBuffer()
	switch c.Type {
	case InsertAction:
		return buf.Insert(c.StartPosition, c.Text)
	case DeleteAction:
		return buf.Delete(c.StartPosition, c.EndPosition)
	}
	return types.EditInfo{}, fmt.Errorf("applyChange: unknown action type %v", c.Type)
}

// undoChange applies the inverse of a single Change and returns EditInfo.
func (m *Manager) undoChange(c Change) (types.EditInfo, error) {
	buf := m.editor.GetBuffer()
	switch c.Type {
	case InsertAction:
		// Undo insert → delete what was inserted
		return buf.Delete(c.StartPosition, c.EndPosition)
	case DeleteAction:
		// Undo delete → re-insert the deleted text
		return buf.Insert(c.StartPosition, c.Text)
	}
	return types.EditInfo{}, fmt.Errorf("undoChange: unknown action type %v", c.Type)
}

// Undo reverts the last recorded change.
func (m *Manager) Undo() (bool, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.currentIndex <= 0 {
		logger.Debugf("History: Nothing to undo.")
		return false, nil // Nothing to undo
	}

	// Get the last applied change
	m.currentIndex--
	changeToUndo := m.changes[m.currentIndex]
	logger.Debugf("History: Undoing change %d (%v)", m.currentIndex, changeToUndo.Type)

	var editInfo types.EditInfo
	var err error

	if changeToUndo.Type == TransactionAction {
		// Undo sub-changes in reverse order
		for i := len(changeToUndo.Children) - 1; i >= 0; i-- {
			editInfo, err = m.undoChange(changeToUndo.Children[i])
			if err != nil {
				logger.Errorf("History: Error undoing transaction sub-change %d: %v", i, err)
				m.currentIndex++ // Revert index change on error
				return false, fmt.Errorf("undo transaction failed at step %d: %w", i, err)
			}
		}
	} else {
		editInfo, err = m.undoChange(changeToUndo)
		if err != nil {
			logger.Errorf("History: Error undoing change: %v", err)
			m.currentIndex++ // Revert index change on error
			return false, fmt.Errorf("undo failed: %w", err)
		}
	}

	// Restore cursor position
	m.editor.SetCursor(changeToUndo.CursorBefore)

	// Dispatch buffer modified event
	eventMgr := m.editor.GetEventManager()
	if eventMgr != nil {
		eventMgr.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
		logger.Debugf("History: Dispatched BufferModified after Undo.")
	}

	return true, nil
}

// Redo reapplies the last undone change.
func (m *Manager) Redo() (bool, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.currentIndex >= len(m.changes) {
		logger.Debugf("History: Nothing to redo. currentIndex=%d, len(changes)=%d", m.currentIndex, len(m.changes))
		return false, nil // Nothing to redo
	}

	// Get the next change to redo
	changeToRedo := m.changes[m.currentIndex]
	logger.DebugTagf("core", "History: Redoing change %d (%v)", m.currentIndex, changeToRedo.Type)

	var editInfo types.EditInfo
	var err error
	var finalCursor types.Position

	if changeToRedo.Type == TransactionAction {
		// Redo sub-changes in original order
		for i, child := range changeToRedo.Children {
			editInfo, err = m.applyChange(child)
			if err != nil {
				logger.Errorf("History: Error redoing transaction sub-change %d: %v", i, err)
				return false, fmt.Errorf("redo transaction failed at step %d: %w", i, err)
			}
		}
		// Cursor lands at the end of the last applied sub-change
		if len(changeToRedo.Children) > 0 {
			last := changeToRedo.Children[len(changeToRedo.Children)-1]
			if last.Type == InsertAction {
				finalCursor = last.EndPosition
			} else {
				finalCursor = last.StartPosition
			}
		}
	} else {
		switch changeToRedo.Type {
		case InsertAction:
			editInfo, err = m.applyChange(changeToRedo)
			if err == nil {
				finalCursor = changeToRedo.EndPosition
				logger.Debugf("History: Redid insert at %v, setting cursor to %v", changeToRedo.StartPosition, finalCursor)
			}
		case DeleteAction:
			editInfo, err = m.applyChange(changeToRedo)
			if err == nil {
				finalCursor = changeToRedo.StartPosition
				logger.Debugf("History: Redid delete from %v to %v, setting cursor to %v",
					changeToRedo.StartPosition, changeToRedo.EndPosition, finalCursor)
			}
		}
		if err != nil {
			logger.Errorf("History: Error redoing change: %v", err)
			return false, fmt.Errorf("redo failed: %w", err)
		}
	}

	// Restore cursor position (to position *after* the change)
	m.editor.SetCursor(finalCursor)
	m.editor.ScrollToCursor() // Ensure cursor is visible

	// Dispatch buffer modified event
	eventMgr := m.editor.GetEventManager()
	if eventMgr != nil {
		eventMgr.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
		logger.Debugf("History: Dispatched BufferModified after Redo.")
	}

	// Move index forward
	m.currentIndex++
	logger.Debugf("History: Redo completed. New currentIndex=%d", m.currentIndex)

	return true, nil
}

// Clear resets the history stack. Call this on file load.
func (m *Manager) Clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.changes = m.changes[:0] // Clear slice while keeping allocated capacity
	m.currentIndex = 0
	m.inTransaction = false
	m.transactionBuf = m.transactionBuf[:0]
	logger.Debugf("History: Cleared.")
}

// CanUndo returns true if there are changes that can be undone.
func (m *Manager) CanUndo() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.currentIndex > 0
}

// CanRedo returns true if there are changes that can be redone.
func (m *Manager) CanRedo() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.currentIndex < len(m.changes)
}

// InTransaction returns whether a transaction is currently open.
func (m *Manager) InTransaction() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.inTransaction
}
