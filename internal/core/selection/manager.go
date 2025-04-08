package selection

import (
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
)

// Manager handles text selection operations
type Manager struct {
	editor         EditorInterface
	selecting      bool
	selectionStart types.Position
	selectionEnd   types.Position
}

// EditorInterface defines what the selection manager needs from editor
type EditorInterface interface {
	GetCursor() types.Position
}

// NewManager creates a new selection manager
func NewManager(editor EditorInterface) *Manager {
	return &Manager{
		editor:         editor,
		selecting:      false,
		selectionStart: types.Position{Line: -1, Col: -1},
		selectionEnd:   types.Position{Line: -1, Col: -1},
	}
}

// HasSelection returns whether there is an active selection
func (m *Manager) HasSelection() bool {
	return m.selecting && !positionsEqual(m.selectionStart, m.selectionEnd)
}

// positionsEqual checks if two positions are equal
func positionsEqual(p1, p2 types.Position) bool {
	return p1.Line == p2.Line && p1.Col == p2.Col
}

// GetSelection returns the normalized selection range
func (m *Manager) GetSelection() (start types.Position, end types.Position, ok bool) {
	if !m.HasSelection() {
		return types.Position{Line: -1, Col: -1}, types.Position{Line: -1, Col: -1}, false
	}

	start = m.selectionStart
	end = m.selectionEnd

	// Normalize to ensure start <= end
	if start.Line > end.Line || (start.Line == end.Line && start.Col > end.Col) {
		start, end = end, start
	}
	return start, end, true
}

// ClearSelection resets the selection state
func (m *Manager) ClearSelection() {
	if m.selecting {
		m.selecting = false
		m.selectionStart = types.Position{Line: -1, Col: -1}
		m.selectionEnd = types.Position{Line: -1, Col: -1}
		logger.Debugf("Selection: Cleared")
	}
}

// StartOrUpdateSelection manages selection state
func (m *Manager) StartOrUpdateSelection() {
	currentCursor := m.editor.GetCursor()

	if !m.selecting {
		m.selectionStart = currentCursor
		m.selecting = true
		logger.Debugf("Selection: Started at %v", m.selectionStart)
	}
	m.selectionEnd = currentCursor
}

// UpdateSelectionEnd updates just the end position
func (m *Manager) UpdateSelectionEnd() {
	if m.selecting {
		m.selectionEnd = m.editor.GetCursor()
	}
}

// IsPositionInSelection checks if a position is within the current selection
func (m *Manager) IsPositionInSelection(pos types.Position) bool {
	if !m.HasSelection() {
		return false
	}

	start, end, _ := m.GetSelection()

	// Check if position is within range
	if pos.Line < start.Line || pos.Line > end.Line {
		return false
	}

	if pos.Line == start.Line && pos.Col < start.Col {
		return false
	}

	if pos.Line == end.Line && pos.Col >= end.Col {
		return false
	}

	return true
}
