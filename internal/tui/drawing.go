// internal/tui/drawing.go
package tui

import (
	"fmt"  // Import fmt
	"math" // Import math for Log10

	// Import strconv
	// Import strings
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/theme" // Import theme package
	"github.com/bethropolis/tide/internal/types" // Needed for Position type and HighlightRegion
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

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

// DrawBuffer draws the *visible* portion using the provided theme.
// It now takes an activeTheme argument.
func DrawBuffer(tuiManager *TUI, editor *core.Editor, activeTheme *theme.Theme) {

	if activeTheme == nil {
		logger.Warnf("DrawBuffer called with nil theme, using package default.")
		defaultTheme := &theme.DevComfortDark // Assuming DevComfortDark is accessible
		// Check if the default theme itself has issues
		if defaultTheme == nil || len(defaultTheme.Styles) == 0 {
			activeTheme = &theme.Theme{Styles: map[string]tcell.Style{"Default": tcell.StyleDefault}}
		} else {
			activeTheme = defaultTheme
		}
	}

	// Get styles from theme
	defaultStyle := activeTheme.GetStyle("Default")       // <<< Get Default style (now has BG)
	lineNumberStyle := activeTheme.GetStyle("LineNumber") // Get LineNumber style
	selectionStyle := activeTheme.GetStyle("Selection")
	searchHighlightStyle := activeTheme.GetStyle("SearchHighlight")

	width, height := tuiManager.Size()
	viewY, viewX := editor.GetViewport()
	selStart, selEnd, selectionActive := editor.GetSelection()
	highlights := editor.GetHighlights()
	statusBarHeight := 1
	viewHeight := height - statusBarHeight

	if viewHeight <= 0 || width <= 0 {
		return
	}

	lines := editor.GetBuffer().Lines()
	lineCount := len(lines)
	if lineCount == 0 {
		lineCount = 1
	} // Avoid Log10(0)

	// --- Calculate Gutter Width ---
	maxDigits := int(math.Log10(float64(lineCount))) + 1
	lineNumberPadding := 1 // Space between number and text
	gutterWidth := maxDigits + lineNumberPadding
	if gutterWidth >= width { // Not enough space for gutter and text
		gutterWidth = 0 // Disable gutter if screen too narrow
	}
	textAreaWidth := width - gutterWidth

	visibleSearchHighlights := make(map[int][]types.HighlightRegion) // Renamed for clarity
	for _, h := range highlights {
		// Iterate over all lines in the highlight range
		startLine := h.Start.Line
		endLine := h.End.Line

		// For each line in the highlight range that's visible
		for lineIdx := startLine; lineIdx <= endLine; lineIdx++ {
			if lineIdx >= viewY && lineIdx < viewY+viewHeight {
				visibleSearchHighlights[lineIdx] = append(visibleSearchHighlights[lineIdx], h)
			}
		}
	}

	// --- Draw Loop ---
	for screenY := 0; screenY < viewHeight; screenY++ {
		bufferLineIdx := screenY + viewY

		// --- A: Fill the entire line with the theme's default style ---
		for fillX := 0; fillX < width; fillX++ {
			tuiManager.screen.SetContent(fillX, screenY, ' ', nil, defaultStyle)
		}

		// --- B: Draw Line Number Gutter ---
		if gutterWidth > 0 {
			lineNumStr := ""
			currentLineStyle := lineNumberStyle // Default gutter style
			if bufferLineIdx >= 0 && bufferLineIdx < len(lines) {
				// Format line number, right-aligned
				lineNumStr = fmt.Sprintf("%*d", maxDigits, bufferLineIdx+1)

				// Optional: Highlight current line number differently
				if editor.GetCursor().Line == bufferLineIdx {
					currentLineStyle = lineNumberStyle.Bold(true)
				}
			}

			// Draw the formatted string into the gutter area
			lineNumRunes := []rune(lineNumStr)
			for i, r := range lineNumRunes {
				drawX := i                                 // Draw starting from column 0
				if drawX < gutterWidth-lineNumberPadding { // Ensure within number area
					tuiManager.screen.SetContent(drawX, screenY, r, nil, currentLineStyle)
				}
			}
		}

		// Check if buffer line exists
		if bufferLineIdx < 0 || bufferLineIdx >= len(lines) {
			// Line is below buffer content, already filled with defaultStyle background.
			continue // Nothing more to draw on this line
		}

		// --- C: Draw Buffer Text ---
		lineBytes := lines[bufferLineIdx]
		lineStr := string(lineBytes)
		gr := uniseg.NewGraphemes(lineStr)
		lineSearchHighlights := visibleSearchHighlights[bufferLineIdx]
		lineSyntaxHighlights := editor.GetSyntaxHighlightsForLine(bufferLineIdx)

		currentVisualX := 0
		currentRuneIndex := 0

		for gr.Next() { // Iterate through grapheme clusters
			clusterRunes := gr.Runes()
			clusterWidth := gr.Width()
			clusterVisualStart := currentVisualX
			clusterVisualEnd := currentVisualX + clusterWidth

			// Calculate screen X relative to text area, accounting for horizontal scroll
			// and offset by the gutter width
			screenX := (clusterVisualStart - viewX) + gutterWidth

			if clusterVisualEnd > viewX && clusterVisualStart < viewX+textAreaWidth {
				// --- Determine Style (Syntax > Search > Selection) ---
				currentStyle := defaultStyle // Start with default (important!)
				currentPos := types.Position{Line: bufferLineIdx, Col: currentRuneIndex}

				// Apply Syntax
				for _, synHL := range lineSyntaxHighlights {
					if currentRuneIndex >= synHL.StartCol && currentRuneIndex < synHL.EndCol {
						currentStyle = activeTheme.GetStyle(synHL.StyleName)
						break
					}
				}
				// Apply Search Highlight
				for _, h := range lineSearchHighlights {
					if h.Type == types.HighlightSearch && isPositionWithin(currentPos, h.Start, h.End) {
						currentStyle = searchHighlightStyle
						break
					}
				}
				// Apply Selection Highlight
				if selectionActive && isPositionWithin(currentPos, selStart, selEnd) {
					currentStyle = selectionStyle
				}

				// --- Draw the Cluster ---
				if screenX >= gutterWidth && screenX < width { // Check if the start of the cluster is visible
					mainRune := clusterRunes[0]
					combining := clusterRunes[1:]

					if mainRune == '\t' {
						// Basic tab expansion (replace with spaces using currentStyle BG/FG)
						tabSpaces := 4 // Or get from config
						// Calculate visual width to next tab stop correctly
						visualScreenX := currentVisualX - viewX + gutterWidth
						spacesToDraw := tabSpaces - (visualScreenX % tabSpaces)
						for i := 0; i < spacesToDraw && screenX+i < width; i++ {
							tuiManager.screen.SetContent(screenX+i, screenY, ' ', nil, currentStyle)
						}
					} else {
						// Draw the rune cluster using the determined style
						tuiManager.screen.SetContent(screenX, screenY, mainRune, combining, currentStyle)
						// Fill remaining cells for wide characters using the determined style
						for cw := 1; cw < clusterWidth; cw++ {
							fillX := screenX + cw
							if fillX < width {
								tuiManager.screen.SetContent(fillX, screenY, ' ', nil, currentStyle)
							}
						}
					}
				}
			}

			currentVisualX += clusterWidth
			currentRuneIndex += len(clusterRunes)
			// Optimization: Stop drawing if we go past the visible text area edge
			if currentVisualX >= viewX+textAreaWidth {
				break
			}
		}
	}
}

// DrawCursor positions the terminal cursor using visual width calculations.
func DrawCursor(tuiManager *TUI, editor *core.Editor) {
	cursor := editor.GetCursor()
	viewY, viewX := editor.GetViewport()

	// Calculate gutter width
	lineCount := editor.GetBuffer().LineCount()
	if lineCount == 0 {
		lineCount = 1
	}
	maxDigits := int(math.Log10(float64(lineCount))) + 1
	lineNumberPadding := 1
	gutterWidth := maxDigits + lineNumberPadding
	width, height := tuiManager.Size() // Get screen width
	if gutterWidth >= width {
		gutterWidth = 0
	} // Disable gutter if too narrow

	// Get current line to calculate visual offset
	lineBytes, err := editor.GetBuffer().Line(cursor.Line)
	cursorVisualCol := 0
	if err == nil {
		cursorVisualCol = calculateVisualColumn(lineBytes, cursor.Col)
	} else {
		logger.Debugf("DrawCursor: Error getting line %d: %v", cursor.Line, err)
	}

	// Calculate screen position based on viewport and visual column
	screenX := (cursorVisualCol - viewX) + gutterWidth
	screenY := cursor.Line - viewY

	// Hide cursor if it's outside the drawable area
	statusBarHeight := 1 // Assuming status bar height is 1
	viewHeight := height - statusBarHeight
	textAreaWidth := width - gutterWidth

	// Check against screen boundaries AND ensure it's not within the gutter itself
	if screenX < gutterWidth || screenX >= width || screenY < 0 || screenY >= viewHeight || viewHeight <= 0 || textAreaWidth <= 0 {
		tuiManager.screen.HideCursor()
	} else {
		tuiManager.screen.ShowCursor(screenX, screenY)
	}
}
