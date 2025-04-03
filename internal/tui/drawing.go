// internal/tui/drawing.go
package tui

import (
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types" // Needed for Position type
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

// calculateVisualColumn computes the visual screen column width for a given rune index within a line.
// This is a duplicate of the function in core/editor.go to avoid circular imports
func calculateVisualColumn(line []byte, runeIndex int) int {
	if runeIndex <= 0 {
		return 0
	}
	str := string(line) // Convert once for iteration
	visualWidth := 0
	currentRuneIndex := 0

	gr := uniseg.NewGraphemes(str)

	for gr.Next() { // Iterate through grapheme clusters (user-perceived characters)
		if currentRuneIndex >= runeIndex {
			break // We've reached or passed the target rune index
		}
		// Get the runes within this grapheme cluster
		runes := gr.Runes()
		// Get the cluster's visual width
		width := gr.Width() // Use uniseg's width calculation

		visualWidth += width
		currentRuneIndex += len(runes) // Increment by the number of runes in the cluster
	}

	return visualWidth
}

// isPositionWithin checks if pos is within the range [start, end) considering lines and columns.
// Assumes start <= end (lexicographically normalized).
func isPositionWithin(pos, start, end types.Position) bool {
	if pos.Line < start.Line || pos.Line > end.Line {
		return false // Outside line range
	}
	if pos.Line == start.Line && pos.Col < start.Col {
		return false // Before start column on start line
	}
	// Important: The end position is *exclusive* for selection ranges.
	// A character at the exact end position is NOT selected.
	if pos.Line == end.Line && pos.Col >= end.Col {
		return false // At or after end column on end line
	}
	// Within line range, and also respects columns on boundary lines
	return true
}

// DrawBuffer draws the *visible* portion with selection highlighting.
func DrawBuffer(tuiManager *TUI, editor *core.Editor) {
	// Define styles
	style := tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)
	selectionStyle := style.Reverse(true) // Use reverse video for selection

	width, height := tuiManager.Size()
	viewY, viewX := editor.GetViewport()

	// Get normalized selection range and active status
	selStart, selEnd, selectionActive := editor.GetSelection()

	statusBarHeight := 1
	viewHeight := height - statusBarHeight
	if viewHeight <= 0 || width <= 0 {
		return
	}

	lines := editor.GetBuffer().Lines()

	for screenY := 0; screenY < viewHeight; screenY++ {
		bufferLineIdx := screenY + viewY
		if bufferLineIdx < 0 || bufferLineIdx >= len(lines) {
			continue
		}

		lineBytes := lines[bufferLineIdx]
		lineStr := string(lineBytes)
		gr := uniseg.NewGraphemes(lineStr)

		currentVisualX := 0
		currentRuneIndex := 0 // Track rune index on the line

		for gr.Next() {
			clusterRunes := gr.Runes()
			clusterWidth := gr.Width()
			clusterVisualStart := currentVisualX
			clusterVisualEnd := currentVisualX + clusterWidth

			// Check if cluster is horizontally visible
			if clusterVisualEnd > viewX && clusterVisualStart < viewX+width {
				// Determine style for this cluster
				currentStyle := style // Start with default
				currentPos := types.Position{Line: bufferLineIdx, Col: currentRuneIndex}

				// --- Apply Selection Style ---
				if selectionActive && isPositionWithin(currentPos, selStart, selEnd) {
					currentStyle = selectionStyle // Apply selection style
				}
				// --- TODO: Apply Syntax/Search Highlighting (could layer on top) ---

				// --- Draw the Cluster ---
				screenX := clusterVisualStart - viewX
				visualOffsetInCluster := 0 // Tracks visual offset *within* the cluster when drawing runes
				for _, r := range clusterRunes {
					// Calculate where this specific rune *starts* on screen
					// Use the visualOffsetInCluster to handle runes within a wide cluster correctly
					calculatedScreenX := screenX + visualOffsetInCluster

					if calculatedScreenX >= 0 && calculatedScreenX < width {
						if r == '\t' {
							// TODO: Fix tab expansion with visual widths
							tuiManager.screen.SetContent(calculatedScreenX, screenY, ' ', nil, currentStyle)
						} else {
							tuiManager.screen.SetContent(calculatedScreenX, screenY, r, nil, currentStyle)
						}
					}
					// Use uniseg to find the width of the *current rune* being drawn
					// to advance the visualOffsetInCluster correctly within the loop.
					// Note: A single grapheme cluster might contain multiple runes (e.g., base char + accent)
					// but tcell places based on the rune. Width check helps avoid overwriting.
					runeWidth := uniseg.StringWidth(string(r))
					if runeWidth < 1 {
						runeWidth = 1 // Treat zero-width runes as taking one logical cell space for offset calculation
					}
					visualOffsetInCluster += runeWidth
				}
			}

			// --- Update positions for next cluster ---
			currentVisualX += clusterWidth
			// Increment rune index by the number of runes in the *whole* cluster
			currentRuneIndex += len(clusterRunes)

			if currentVisualX >= viewX+width {
				break // Optimization
			}
		}
	}
}

// DrawCursor positions the terminal cursor using visual width calculations.
func DrawCursor(tuiManager *TUI, editor *core.Editor) {
	cursor := editor.GetCursor()
	viewY, viewX := editor.GetViewport()

	// Get current line to calculate visual offset
	lineBytes, err := editor.GetBuffer().Line(cursor.Line)
	cursorVisualCol := 0
	if err == nil {
		cursorVisualCol = calculateVisualColumn(lineBytes, cursor.Col)
	} else {
		// Fallback or log error if line cannot be read
		logger.Debugf("DrawCursor: Error getting line %d: %v", cursor.Line, err)
	}

	// Calculate screen position based on viewport and visual column
	screenX := cursorVisualCol - viewX // Position relative to viewport start
	screenY := cursor.Line - viewY     // Simple offset

	// Hide cursor if it's outside the drawable buffer area
	width, height := tuiManager.Size()
	statusBarHeight := 1
	viewHeight := height - statusBarHeight
	if screenX < 0 || screenX >= width || screenY < 0 || screenY >= viewHeight || viewHeight <= 0 || width <= 0 {
		tuiManager.screen.HideCursor()
	} else {
		tuiManager.screen.ShowCursor(screenX, screenY)
	}
}
