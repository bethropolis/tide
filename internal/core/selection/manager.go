package selection

import (
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
)

// Manager handles text selection state and logic.
type Manager struct {
	editor EditorInterface // Interface to get cursor position

	// --- State owned by Selection Manager ---
	selecting      bool
	selectionStart types.Position // Anchor point
	selectionEnd   types.Position // Usually follows cursor
}

// EditorInterface defines what the selection manager needs from editor.
type EditorInterface interface {
	GetCursor() types.Position // Need current cursor pos
}

// NewManager creates a new selection manager.
func NewManager(editor EditorInterface) *Manager {
	return &Manager{
		editor:         editor,
		selecting:      false,
		selectionStart: types.Position{Line: -1, Col: -1}, // Invalid start means no selection
		selectionEnd:   types.Position{Line: -1, Col: -1},
	}
}

// HasSelection returns whether there is an active selection.
func (m *Manager) HasSelection() bool {
	// A selection is active if 'selecting' is true AND start/end differ.
	return m.selecting && !positionsEqual(m.selectionStart, m.selectionEnd)
}

// positionsEqual checks if two positions are equal.
func positionsEqual(p1, p2 types.Position) bool {
	return p1.Line == p2.Line && p1.Col == p2.Col
}

// GetSelection returns the normalized selection range (start <= end).
func (m *Manager) GetSelection() (start types.Position, end types.Position, ok bool) {
	if !m.selecting { // Check 'selecting' flag first
		return types.Position{Line: -1, Col: -1}, types.Position{Line: -1, Col: -1}, false
	}

	start = m.selectionStart
	end = m.selectionEnd

	// Handle case where selection hasn't moved yet (start==end)
	if positionsEqual(start, end) {
		return start, end, false // Valid anchor, but no range selected yet
	}

	// Normalize: Ensure start is lexicographically before or equal to end
	if start.Line > end.Line || (start.Line == end.Line && start.Col > end.Col) {
		start, end = end, start // Swap
	}
	return start, end, true // ok is true only if selecting and start != end
}

// ClearSelection resets the selection state.
func (m *Manager) ClearSelection() {
	if m.selecting { // Only log if selection was actually active
		logger.DebugTagf("core", "Selection Manager: Cleared")
	}
	m.selecting = false
	m.selectionStart = types.Position{Line: -1, Col: -1}
	m.selectionEnd = types.Position{Line: -1, Col: -1}
}

// StartOrUpdateSelection is called when selection should start or extend (e.g., Shift+Move).
func (m *Manager) StartOrUpdateSelection() {
	currentCursor := m.editor.GetCursor() // Get current cursor position

	if !m.selecting {
		// If not currently selecting, start a new selection anchored here
		m.selectionStart = currentCursor
		m.selecting = true
		logger.DebugTagf("core", "Selection Manager: Started at %v", m.selectionStart)
	}
	// Always update the end position to follow the cursor during selection movement
	m.selectionEnd = currentCursor
}

// UpdateSelectionEnd updates just the end position of the selection
// to match the current cursor position.
func (m *Manager) UpdateSelectionEnd() {
	if m.selecting {
		currentCursor := m.editor.GetCursor()
		m.selectionEnd = currentCursor
		logger.DebugTagf("core", "Selection Manager: Updated end to %v", m.selectionEnd)
	}
}

// IsSelecting returns the raw selecting flag state.
func (m *Manager) IsSelecting() bool {
	return m.selecting
}
