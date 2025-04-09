package render

import (
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/theme"
	"github.com/bethropolis/tide/internal/tui"
	"github.com/bethropolis/tide/internal/types"
)

// Buffer draws the editor buffer with syntax highlighting and selection
func Buffer(tuiManager *tui.TUI, editor *core.Editor, activeTheme *theme.Theme) {
	// This function will contain the code moved from tui.DrawBuffer
	// Keeping the same implementation but in a separate package
	tui.DrawBuffer(tuiManager, editor, activeTheme)
}

// Cursor positions the terminal cursor using visual width calculations
func Cursor(tuiManager *tui.TUI, editor *core.Editor) {
	// This function will contain the code moved from tui.DrawCursor
	tui.DrawCursor(tuiManager, editor)
}

// IsPositionWithin checks if pos is within the range [start, end) considering lines and columns
func IsPositionWithin(pos, start, end types.Position) bool {
	if pos.Line < start.Line || pos.Line > end.Line {
		return false // Outside line range
	}
	if pos.Line == start.Line && pos.Col < start.Col {
		return false // Before start column on start line
	}
	// Important: The end position is *exclusive* for selection ranges
	if pos.Line == end.Line && pos.Col >= end.Col {
		return false // At or after end column on end line
	}
	// Within line range, and also respects columns on boundary lines
	return true
}
