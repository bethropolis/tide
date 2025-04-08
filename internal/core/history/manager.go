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

// RecordChange adds a new change, clearing any redo history.
func (m *Manager) RecordChange(change Change) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

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

	buf := m.editor.GetBuffer()
	var editInfo types.EditInfo
	var err error

	// Apply the inverse operation
	switch changeToUndo.Type {
	case InsertAction:
		// Undo an insert by deleting the inserted text
		// The range to delete is from StartPosition to EndPosition of the original insert
		editInfo, err = buf.Delete(changeToUndo.StartPosition, changeToUndo.EndPosition)
		if err != nil {
			logger.Errorf("History: Error undoing insert (delete failed): %v", err)
			m.currentIndex++ // Revert index change on error
			return false, fmt.Errorf("undo failed: %w", err)
		}
		logger.Debugf("History: Undid insert via delete.")

	case DeleteAction:
		// Undo a delete by inserting the deleted text back
		// Insert at the StartPosition where the delete originally happened
		editInfo, err = buf.Insert(changeToUndo.StartPosition, changeToUndo.Text)
		if err != nil {
			logger.Errorf("History: Error undoing delete (insert failed): %v", err)
			m.currentIndex++ // Revert index change on error
			return false, fmt.Errorf("undo failed: %w", err)
		}
		logger.Debugf("History: Undid delete via insert.")
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
	logger.Debugf("History: Redoing change %d (%v), Text: %q, StartPos: %v, EndPos: %v",
		m.currentIndex, changeToRedo.Type, string(changeToRedo.Text), changeToRedo.StartPosition, changeToRedo.EndPosition)

	buf := m.editor.GetBuffer()
	var editInfo types.EditInfo
	var err error
	var finalCursor types.Position // Calculate cursor after redo

	// Apply the original operation
	switch changeToRedo.Type {
	case InsertAction:
		editInfo, err = buf.Insert(changeToRedo.StartPosition, changeToRedo.Text)
		if err == nil {
			// Cursor after insert is typically at the end of the inserted text
			finalCursor = changeToRedo.EndPosition
			logger.Debugf("History: Redid insert at %v, setting cursor to %v", changeToRedo.StartPosition, finalCursor)
		} else {
			logger.Errorf("History: Error redoing insert: %v", err)
		}
	case DeleteAction:
		editInfo, err = buf.Delete(changeToRedo.StartPosition, changeToRedo.EndPosition)
		if err == nil {
			// Cursor after delete is typically at the start of where the deletion happened
			finalCursor = changeToRedo.StartPosition
			logger.Debugf("History: Redid delete from %v to %v, setting cursor to %v",
				changeToRedo.StartPosition, changeToRedo.EndPosition, finalCursor)
		} else {
			logger.Errorf("History: Error redoing delete: %v", err)
		}
	}

	if err != nil {
		// Don't advance index if redo failed
		return false, fmt.Errorf("redo failed: %w", err)
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
