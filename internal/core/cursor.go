package core

import (
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/logger"
	"github.com/rivo/uniseg"
)

// MoveCursor moves the cursor AND adjusts the viewport, handling line wraps.
func (e *Editor) MoveCursor(deltaLine, deltaCol int) {
	currentLine := e.Cursor.Line
	currentCol := e.Cursor.Col
	lineCount := e.buffer.LineCount()

	// --- Handle Horizontal Wrap-Around FIRST ---
	// If deltaLine is non-zero, we are moving vertically, wrap doesn't apply here.
	if deltaLine == 0 && lineCount > 0 {
		if deltaCol > 0 { // Attempting to move Right
			lineBytes, err := e.buffer.Line(currentLine)
			// Only wrap if we successfully read the current line
			if err == nil {
				maxCol := utf8.RuneCount(lineBytes)
				if currentCol >= maxCol && currentLine < lineCount-1 { // At or past EOL and not on the last line
					e.Cursor.Line++  // Move to next line
					e.Cursor.Col = 0 // Move to beginning of that line
					e.ScrollToCursor()
					return // Wrap handled
				}
			}
		} else if deltaCol < 0 { // Attempting to move Left
			// No need to read line content for BOL check
			if currentCol <= 0 && currentLine > 0 { // At or before BOL and not on the first line
				e.Cursor.Line-- // Move to previous line
				// Now read the previous line to find its end
				prevLineBytes, err := e.buffer.Line(e.Cursor.Line)
				if err == nil {
					e.Cursor.Col = utf8.RuneCount(prevLineBytes) // Move to end of that line
				} else {
					e.Cursor.Col = 0 // Fallback if error reading prev line
				}
				e.ScrollToCursor()
				return // Wrap handled
			}
		}
	}

	// --- Default Movement & Clamping (if wrap didn't happen or wasn't applicable) ---
	targetLine := currentLine + deltaLine
	targetCol := currentCol + deltaCol // Calculate potential column based on current

	// Clamp targetLine vertically
	if targetLine < 0 {
		targetLine = 0
	}
	if lineCount == 0 {
		targetLine = 0 // Should only be line 0 if buffer isn't empty per convention
	} else if targetLine >= lineCount {
		targetLine = lineCount - 1
	}

	// Clamp targetCol horizontally based on the *target* line's content
	if targetCol < 0 {
		targetCol = 0
	}
	if lineCount > 0 {
		targetLineBytes, err := e.buffer.Line(targetLine)
		if err == nil {
			maxCol := utf8.RuneCount(targetLineBytes)
			// If moving vertically (deltaLine != 0), ensure Col doesn't exceed maxCol of new line
			// If moving horizontally (deltaCol != 0), only clamp if targetCol goes beyond maxCol
			if targetCol > maxCol {
				targetCol = maxCol
			}
		} else {
			// Error fetching target line's content, reset column
			targetCol = 0
		}
	} else {
		targetCol = 0 // No lines in buffer
	}

	// Assign final clamped position
	e.Cursor.Line = targetLine
	e.Cursor.Col = targetCol

	// --- Update selection end if active ---
	if e.selecting {
		e.selectionEnd = e.Cursor
	}

	// Ensure cursor is visible after move/clamp
	e.ScrollToCursor()
}

// calculateVisualColumn computes the visual screen column width for a given rune index within a line.
// It correctly handles multi-width characters and grapheme clusters.
func calculateVisualColumn(line []byte, runeIndex int) int {
	if runeIndex <= 0 {
		return 0
	}
	str := string(line)
	visualWidth := 0
	currentRuneIndex := 0
	gr := uniseg.NewGraphemes(str)

	for gr.Next() {
		if currentRuneIndex >= runeIndex {
			break
		}
		runes := gr.Runes()
		width := gr.Width()
		visualWidth += width
		currentRuneIndex += len(runes)
	}
	return visualWidth
}

// ScrollToCursor adjusts the viewport incorporating ScrollOff and visual width.
func (e *Editor) ScrollToCursor() {
	if e.viewHeight <= 0 || e.viewWidth <= 0 {
		return // Cannot scroll if view has no dimensions
	}

	// Effective scrolloff (cannot be larger than half the view height)
	effectiveScrollOff := e.ScrollOff
	if effectiveScrollOff*2 >= e.viewHeight {
		if e.viewHeight > 0 {
			effectiveScrollOff = (e.viewHeight - 1) / 2 // Max half the height
		} else {
			effectiveScrollOff = 0
		}
	}

	// Vertical scrolling with scrolloff
	if e.Cursor.Line < e.ViewportY+effectiveScrollOff {
		// Cursor is within scrolloff distance from the top edge (or above)
		e.ViewportY = e.Cursor.Line - effectiveScrollOff
		if e.ViewportY < 0 {
			e.ViewportY = 0 // Don't scroll past the beginning of the file
		}
	} else if e.Cursor.Line >= e.ViewportY+e.viewHeight-effectiveScrollOff {
		// Cursor is within scrolloff distance from the bottom edge (or below)
		e.ViewportY = e.Cursor.Line - e.viewHeight + 1 + effectiveScrollOff
		// Optional: Prevent ViewportY from scrolling too far past the end?
		// maxViewportY := e.buffer.LineCount() - e.viewHeight
		// if maxViewportY < 0 { maxViewportY = 0 }
		// if e.ViewportY > maxViewportY { e.ViewportY = maxViewportY }
	}

	// --- Horizontal Scrolling (based on visual column) ---
	lineBytes, err := e.buffer.Line(e.Cursor.Line)
	cursorVisualCol := 0
	if err == nil {
		cursorVisualCol = calculateVisualColumn(lineBytes, e.Cursor.Col)
	} else {
		logger.Debugf("ScrollToCursor: Error getting line %d: %v", e.Cursor.Line, err)
		// Fallback if line fetch fails
	}

	// Simple horizontal scroll (no horizontal ScrollOff for now)
	if cursorVisualCol < e.ViewportX {
		// Cursor's visual position is left of the viewport
		e.ViewportX = cursorVisualCol
	} else if cursorVisualCol >= e.ViewportX+e.viewWidth {
		// Cursor's visual position is right of the viewport edge
		// We want the cursor to be *at* the last visible column,
		// so ViewportX should be cursorVisualCol - viewWidth + 1
		e.ViewportX = cursorVisualCol - e.viewWidth + 1
	}

	// Clamp viewport origins
	if e.ViewportY < 0 {
		e.ViewportY = 0
	}
	if e.ViewportX < 0 {
		e.ViewportX = 0
	}
}

// PageMove moves the cursor and viewport up or down by one page height.
// 'deltaPages' is typically +1 (PageDown) or -1 (PageUp).
func (e *Editor) PageMove(deltaPages int) {
	if e.viewHeight <= 0 {
		return // Cannot page if view has no height
	}

	// Move cursor by viewHeight * deltaPages
	targetLine := e.Cursor.Line + (e.viewHeight * deltaPages)

	// Clamp target line to buffer bounds
	lineCount := e.buffer.LineCount()
	if targetLine < 0 {
		targetLine = 0
	} else if targetLine >= lineCount {
		targetLine = lineCount - 1
	}

	// Set cursor position directly
	e.Cursor.Line = targetLine
	// Try to maintain horizontal position (Col), clamping if necessary
	e.MoveCursor(0, 0) // Use MoveCursor's logic to clamp Col and scroll

	// Note: MoveCursor will update selection if e.selecting is true

	// Explicitly move viewport - ScrollToCursor might not jump a full page
	e.ViewportY += (e.viewHeight * deltaPages)
	// Clamp ViewportY after the jump
	if e.ViewportY < 0 {
		e.ViewportY = 0
	}
	maxViewportY := lineCount - e.viewHeight
	if maxViewportY < 0 {
		maxViewportY = 0
	}
	if e.ViewportY > maxViewportY {
		e.ViewportY = maxViewportY
	}

	// Final scroll check to ensure cursor visibility *after* the jump
	e.ScrollToCursor()
}

// Home moves the cursor to the beginning of the current line (column 0).
func (e *Editor) Home() {
	e.Cursor.Col = 0
	if e.selecting {
		e.selectionEnd = e.Cursor
	}
	e.ScrollToCursor() // Ensure viewport adjusts if needed
}

// End moves the cursor to the end of the current line.
func (e *Editor) End() {
	lineBytes, err := e.buffer.Line(e.Cursor.Line)
	if err != nil {
		logger.Debugf("Error getting line %d for End key: %v", e.Cursor.Line, err)
		e.Cursor.Col = 0 // Fallback to beginning
	} else {
		e.Cursor.Col = utf8.RuneCount(lineBytes) // Move to position *after* last rune
	}
	if e.selecting {
		e.selectionEnd = e.Cursor
	}
	e.ScrollToCursor() // Ensure viewport adjusts
}
