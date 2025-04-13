// internal/tui/drawing.go
package tui

import (
	"fmt"  // Import fmt
	"math" // Import math for Log10

	// Import config for DefaultTabWidth
	"github.com/bethropolis/tide/internal/config" // Import config for DefaultTabWidth
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/theme" // Import theme package
	"github.com/bethropolis/tide/internal/types" // Needed for Position type and HighlightRegion
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

func calculateVisualColumn(line []byte, runeIndex int, tabWidth int) int {
	if runeIndex <= 0 {
		return 0
	}
	if tabWidth <= 0 {
		tabWidth = config.DefaultTabWidth // Use default if invalid
		if tabWidth <= 0 {
			tabWidth = 8 // Absolute fallback
		}
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

		if len(runes) > 0 && runes[0] == '\t' {
			// Tab calculation - add spaces needed to reach next tab stop
			spacesToNextTabStop := tabWidth - (visualWidth % tabWidth)
			visualWidth += spacesToNextTabStop
		} else {
			// Regular character width
			width := gr.Width() // Use uniseg's width calculation
			visualWidth += width
		}

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
func DrawBuffer(tuiManager *TUI, editor *core.Editor, activeTheme *theme.Theme) {
	// Fallback theme handling (same as before)
	if activeTheme == nil {
		logger.Warnf("DrawBuffer called with nil theme, using package default.")
		activeTheme = &theme.Theme{Styles: map[string]tcell.Style{"Default": tcell.StyleDefault}}
	}

	// Get styles from theme
	defaultStyle := activeTheme.GetStyle("Default")
	lineNumberStyle := activeTheme.GetStyle("LineNumber")
	selectionStyle := activeTheme.GetStyle("Selection")
	searchHighlightStyle := activeTheme.GetStyle("SearchHighlight")

	// Get screen dimensions and viewport position
	width, height := tuiManager.Size()
	logger.DebugTagf("draw", "DrawBuffer Start: Screen Size (%d x %d)", width, height)

	viewY, viewX := editor.GetViewport() // Get both viewY and viewX for horizontal scrolling

	lines := editor.GetBuffer().Lines()
	lineCount := len(lines)
	if lineCount == 0 {
		lineCount = 1
	}

	// Get selection info
	selStart, selEnd, selectionActive := editor.GetSelection()

	// Get search highlights
	searchHighlights := editor.GetFindManager().GetHighlights()

	// Calculate gutter width
	maxDigits := int(math.Log10(float64(lineCount))) + 1
	lineNumberPadding := 1
	initialGutterWidth := maxDigits + lineNumberPadding
	logger.DebugTagf("draw", "DrawBuffer Calc: lineCount=%d, maxDigits=%d, padding=%d -> initialGutterWidth=%d",
		lineCount, maxDigits, lineNumberPadding, initialGutterWidth)

	gutterWidth := initialGutterWidth
	if gutterWidth >= width {
		logger.DebugTagf("draw", "DrawBuffer Gutter Check: gutterWidth (%d) >= width (%d). Setting gutterWidth to 0.",
			gutterWidth, width)
		gutterWidth = 0
	} else {
		logger.DebugTagf("draw", "DrawBuffer Gutter Check: gutterWidth (%d) < width (%d). Keeping gutterWidth.",
			gutterWidth, width)
	}

	// Configure tab width
	tabWidth := config.DefaultTabWidth
	if tabWidth <= 0 {
		tabWidth = 8 // Fallback
	}

	// --- Draw Loop ---
	for screenY := 0; screenY < height; screenY++ {
		bufferLineIdx := screenY + viewY

		// --- Draw Line Number Gutter ---
		if bufferLineIdx >= 0 && bufferLineIdx < len(lines) {
			lineNumStr := fmt.Sprintf("%d", bufferLineIdx+1)
			for i, r := range lineNumStr {
				tuiManager.screen.SetContent(i, screenY, r, nil, lineNumberStyle)
			}
		}

		// --- Draw Buffer Text (if line exists) ---
		if bufferLineIdx < 0 || bufferLineIdx >= len(lines) {
			continue // Skip text drawing for lines outside buffer
		}

		// Get syntax highlights for this line
		syntaxHighlights := editor.GetSyntaxHighlightsForLine(bufferLineIdx)

		// Create a map for fast lookup of search highlights on this line
		lineSearchHighlights := make(map[int]bool)
		if searchHighlights != nil {
			for _, highlight := range searchHighlights {
				if highlight.Start.Line <= bufferLineIdx && bufferLineIdx <= highlight.End.Line {
					// If highlight spans multiple lines, we need special handling
					if highlight.Start.Line < bufferLineIdx && bufferLineIdx < highlight.End.Line {
						// Middle of multi-line highlight - entire line is highlighted
						for i := 0; i < len(lines[bufferLineIdx]); i++ {
							lineSearchHighlights[i] = true
						}
					} else if highlight.Start.Line == bufferLineIdx && highlight.End.Line > bufferLineIdx {
						// Start of multi-line highlight
						for i := highlight.Start.Col; i < len(lines[bufferLineIdx]); i++ {
							lineSearchHighlights[i] = true
						}
					} else if highlight.Start.Line < bufferLineIdx && highlight.End.Line == bufferLineIdx {
						// End of multi-line highlight
						for i := 0; i < highlight.End.Col; i++ {
							lineSearchHighlights[i] = true
						}
					} else if highlight.Start.Line == bufferLineIdx && highlight.End.Line == bufferLineIdx {
						// Single line highlight
						for i := highlight.Start.Col; i < highlight.End.Col; i++ {
							lineSearchHighlights[i] = true
						}
					}
				}
			}
		}

		// Draw text with syntax highlighting, accounting for horizontal scrolling
		lineStr := string(lines[bufferLineIdx])
		gr := uniseg.NewGraphemes(lineStr)
		screenX := gutterWidth // Start position after gutter
		currentRuneIndex := 0
		currentVisualX := 0 // Track visual position in the line

		for gr.Next() {
			runes := gr.Runes()
			if len(runes) == 0 {
				continue
			}

			// Get visual width of this grapheme cluster
			clusterWidth := gr.Width()

			// Current position for style and selection checks
			currentPos := types.Position{Line: bufferLineIdx, Col: currentRuneIndex}

			// Determine the style to use for this character
			currentStyle := defaultStyle // Start with default style

			// Apply syntax highlights if available
			styleName := ""
			for _, syntaxHL := range syntaxHighlights {
				if currentRuneIndex >= syntaxHL.StartCol && currentRuneIndex < syntaxHL.EndCol {
					styleName = syntaxHL.StyleName
					currentStyle = activeTheme.GetStyle(styleName)
					break
				}
			}

			// Apply search highlight (takes precedence over syntax)
			if _, isHighlighted := lineSearchHighlights[currentRuneIndex]; isHighlighted {
				currentStyle = searchHighlightStyle
			}

			// Apply selection style (takes precedence over both syntax and search)
			if selectionActive && isPositionWithin(currentPos, selStart, selEnd) {
				currentStyle = selectionStyle
			}

			// Get the main rune
			mainRune := runes[0]

			// Handle tabs specially
			if mainRune == '\t' {
				// Calculate tab stops based on current visual position
				spacesToNextTabStop := tabWidth - (currentVisualX % tabWidth)

				// Only draw visible portion (after viewX)
				if currentVisualX+spacesToNextTabStop > viewX {
					// How many spaces to draw?
					visibleSpaces := currentVisualX + spacesToNextTabStop - viewX
					if currentVisualX < viewX {
						visibleSpaces = spacesToNextTabStop - (viewX - currentVisualX)
					}

					// Draw spaces for the tab if they're visible
					tabScreenX := screenX
					if currentVisualX < viewX {
						tabScreenX = gutterWidth // Start at gutter if partially visible
					}

					for i := 0; i < visibleSpaces && tabScreenX+i < width; i++ {
						tuiManager.screen.SetContent(tabScreenX+i, screenY, ' ', nil, currentStyle)
					}

					// Adjust screen position if tab is visible
					if currentVisualX+spacesToNextTabStop > viewX {
						screenX = tabScreenX + visibleSpaces
					}
				}

				currentVisualX += spacesToNextTabStop
			} else {
				// Regular character
				// Only draw if it's in the visible area (after viewX)
				if currentVisualX >= viewX {
					// Debug logging for styles with screen positions
					fg, bg, attr := currentStyle.Decompose()
					logger.DebugTagf("draw", "SetContent Line %d: screenX=%d, screenY=%d, rune='%c'(%d), Style=%s (FG:%v, BG:%v, Attr:%v)",
						bufferLineIdx, screenX, screenY, mainRune, mainRune, styleName, fg, bg, attr)

					// Only draw if within screen width
					if screenX >= gutterWidth && screenX < width {
						// Draw character with style
						tuiManager.screen.SetContent(screenX, screenY, mainRune, runes[1:], currentStyle)

						// For wide characters (like CJK), fill the extra cells
						for i := 1; i < clusterWidth && screenX+i < width; i++ {
							tuiManager.screen.SetContent(screenX+i, screenY, ' ', nil, currentStyle)
						}
					}
					screenX += clusterWidth
				}

				currentVisualX += clusterWidth
			}

			// Advance the rune index
			currentRuneIndex += len(runes)

			// Optimization: stop processing if we're past the visible area
			if currentVisualX >= viewX+width-gutterWidth {
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

	// Configurable Tab Width
	tabWidth := config.DefaultTabWidth
	if tabWidth <= 0 {
		tabWidth = 8 // Fallback
	}

	// Get current line to calculate visual offset
	lineBytes, err := editor.GetBuffer().Line(cursor.Line)
	cursorVisualCol := 0
	if err == nil {
		cursorVisualCol = calculateVisualColumn(lineBytes, cursor.Col, tabWidth)
	} else {
		logger.DebugTagf("tui", "DrawCursor: Error getting line %d: %v", cursor.Line, err)
	}

	// Calculate screen position based on viewport and visual column
	screenX := (cursorVisualCol - viewX) + gutterWidth
	screenY := cursor.Line - viewY

	// Hide cursor if it's outside the drawable area
	statusBarHeight := config.Get().Editor.StatusBarHeight // Use config value instead of hardcoding
	viewHeight := height - statusBarHeight
	textAreaWidth := width - gutterWidth

	// --- Add Debug Logging ---
	logger.DebugTagf("draw", "DrawCursor: Screen Size (%d x %d), StatusBarHeight: %d, ViewHeight: %d, CursorScreen: (%d, %d)",
		width, height, statusBarHeight, viewHeight, screenX, screenY)
	// --- End Debug Logging ---

	// Check against screen boundaries AND ensure it's not within the gutter itself
	if screenX < gutterWidth || screenX >= width || screenY < 0 || screenY >= viewHeight || viewHeight <= 0 || textAreaWidth <= 0 {
		tuiManager.screen.HideCursor()
	} else {
		tuiManager.screen.ShowCursor(screenX, screenY)
	}
}
