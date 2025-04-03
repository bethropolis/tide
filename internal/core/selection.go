package core

import (
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
)

// HasSelection returns true if there is an active selection.
func (e *Editor) HasSelection() bool {
	return e.selecting && e.selectionStart != e.selectionEnd
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
	e.selecting = false
	// Optionally reset positions, or keep them for potential re-activation? Reset for now.
	e.selectionStart = types.Position{Line: -1, Col: -1}
	e.selectionEnd = types.Position{Line: -1, Col: -1}
	logger.Debugf("Editor: Selection cleared") // Debug log
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
