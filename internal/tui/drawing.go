// internal/tui/drawing.go
package tui

import (
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/theme" // Import theme package
	"github.com/bethropolis/tide/internal/types" // Needed for Position type and HighlightRegion
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

// DrawBuffer draws the *visible* portion using the provided theme.
// It now takes an activeTheme argument.
func DrawBuffer(tuiManager *TUI, editor *core.Editor, activeTheme *theme.Theme) {

	// Use the provided theme. If nil, log warning and use a basic default.
	if activeTheme == nil {
		// This case should ideally not happen if wired correctly in App.
		// If it does, use the package default or a very basic tcell style.
		logger.Warnf("DrawBuffer called with nil theme, using package default.")
		activeTheme = &theme.DevComfortDark // Assuming DefaultDark is accessible
		if activeTheme == nil { // Absolute fallback
			activeTheme = &theme.Theme{ Styles: map[string]tcell.Style{"Default": tcell.StyleDefault} }
		}
	}

	// Get base styles *from the theme*
	style := activeTheme.GetStyle("Default")
	selectionStyle := activeTheme.GetStyle("Selection")
	searchHighlightStyle := activeTheme.GetStyle("SearchHighlight")

	// NOTE: The hardcoded `syntaxStyles` map is REMOVED. We look up from activeTheme.

	width, height := tuiManager.Size()
	viewY, viewX := editor.GetViewport()

	// Get normalized selection range and active status
	// FIX 1: Use the correct method name 'GetSelection'
	selStart, selEnd, selectionActive := editor.GetSelection()

	// Get search highlights
	highlights := editor.GetHighlights()

	statusBarHeight := 1
	viewHeight := height - statusBarHeight
	if viewHeight <= 0 || width <= 0 {
		return // Nothing to draw
	}

	lines := editor.GetBuffer().Lines()

	// --- Pre-calculate search highlights for visible lines ---
	visibleHighlights := make(map[int][]types.HighlightRegion)
	for _, h := range highlights {
		// Basic visibility check (could be refined for multi-line highlights)
		if h.Start.Line >= viewY && h.Start.Line < viewY+viewHeight {
			visibleHighlights[h.Start.Line] = append(visibleHighlights[h.Start.Line], h)
		} else if h.End.Line >= viewY && h.End.Line < viewY+viewHeight {
             // Also include if it ends in the viewport
             visibleHighlights[h.Start.Line] = append(visibleHighlights[h.Start.Line], h) // Use Start.Line as key for simplicity
        } else if h.Start.Line < viewY && h.End.Line >= viewY+viewHeight {
             // Also include if it spans the entire viewport
             visibleHighlights[h.Start.Line] = append(visibleHighlights[h.Start.Line], h)
        }
	}


	// --- Draw Loop ---
	for screenY := 0; screenY < viewHeight; screenY++ {
		bufferLineIdx := screenY + viewY
		if bufferLineIdx < 0 || bufferLineIdx >= len(lines) {
			// Draw empty lines or tildes for lines beyond the buffer? Optional.
			// For now, just skip.
			continue
		}

		lineBytes := lines[bufferLineIdx]
		lineStr := string(lineBytes)
		gr := uniseg.NewGraphemes(lineStr)

		// Get search highlights specific to this line
		lineSearchHighlights := visibleHighlights[bufferLineIdx] // Check map for search highlights

		// Get syntax highlights for this specific line (uses internal mutex)
		lineSyntaxHighlights := editor.GetSyntaxHighlightsForLine(bufferLineIdx)

		currentVisualX := 0
		currentRuneIndex := 0 // Track rune index on the line

		for gr.Next() { // Iterate through grapheme clusters
			clusterRunes := gr.Runes()
			clusterWidth := gr.Width()
			clusterVisualStart := currentVisualX
			clusterVisualEnd := currentVisualX + clusterWidth

			// Check if cluster is horizontally visible
			if clusterVisualEnd > viewX && clusterVisualStart < viewX+width {
				// Determine style for this cluster
				currentStyle := style // Start with theme's default style
				currentPos := types.Position{Line: bufferLineIdx, Col: currentRuneIndex}

				// --- Apply Syntax Highlighting ---
				// Find the syntax style for the current position
				for _, synHL := range lineSyntaxHighlights {
					// Check if current rune index is within this syntax range
					if currentRuneIndex >= synHL.StartCol && currentRuneIndex < synHL.EndCol {
						// FIX 2: Use theme.GetStyle for syntax lookup
						currentStyle = activeTheme.GetStyle(synHL.StyleName)
						break // Apply first syntax highlight found for this position
					}
				}

				// --- Apply Search Highlight Style (Overrides Syntax) ---
				for _, h := range lineSearchHighlights { // Iterate search highlights for this line
					if h.Type == types.HighlightSearch && isPositionWithin(currentPos, h.Start, h.End) {
						currentStyle = searchHighlightStyle // Already got this from theme
						break
					}
				}

				// --- Apply Selection Highlight (Overrides Everything) ---
				if selectionActive && isPositionWithin(currentPos, selStart, selEnd) {
					currentStyle = selectionStyle // Already got this from theme
				}

				// --- Draw the Cluster ---
				screenX := clusterVisualStart - viewX
				visualOffsetInCluster := 0
				for i, r := range clusterRunes {
					calculatedScreenX := screenX + visualOffsetInCluster
					if calculatedScreenX >= 0 && calculatedScreenX < width {
						// Use first rune of cluster, others are combining chars for tcell
						mainRune := r
						var combining []rune
						if i > 0 { // Should not happen if we draw cluster at once? Let tcell handle it.
							// continue // Skip drawing combining chars directly? tcell handles this.
						}
						if i == 0 {
                            combining = clusterRunes[1:]
                        }


						if mainRune == '\t' {
							// TODO: Tab expansion needs more work with themes/widths
							// For now, draw a space using the current style
                            for tabX := 0; tabX < 4; tabX++ { // Assuming tab width 4 for now
                                 drawX := calculatedScreenX + tabX
                                 if drawX < width {
                                     tuiManager.screen.SetContent(drawX, screenY, ' ', nil, currentStyle)
                                 }
                            }

						} else if i == 0 { // Draw only the first rune + combining chars
							tuiManager.screen.SetContent(calculatedScreenX, screenY, mainRune, combining, currentStyle)
						}
					}

                    // Advance offset within the *current* screen cell for wide chars
                    // Only advance if it's the *first* rune of the cluster being drawn
                    if i == 0 {
                         runeWidth := clusterWidth // Use cluster width
                         // Fill remaining cells of a wide character cluster
                         for cw := 1; cw < runeWidth; cw++ {
                              fillX := calculatedScreenX + cw
                              if fillX >= 0 && fillX < width {
                                   tuiManager.screen.SetContent(fillX, screenY, ' ', nil, currentStyle)
                              }
                         }
                         visualOffsetInCluster += runeWidth
                    }
				}
			}

			// --- Update positions for next cluster ---
			currentVisualX += clusterWidth
			currentRuneIndex += len(clusterRunes)

			// Optimization: Stop drawing line if we've gone past the edge
			if currentVisualX >= viewX+width {
				break
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
		logger.Debugf("DrawCursor: Error getting line %d: %v", cursor.Line, err)
	}

	// Calculate screen position based on viewport and visual column
	screenX := cursorVisualCol - viewX
	screenY := cursor.Line - viewY

	// Hide cursor if it's outside the drawable buffer area
	width, height := tuiManager.Size()
	statusBarHeight := 1 // Assuming status bar height is 1
	viewHeight := height - statusBarHeight
	if screenX < 0 || screenX >= width || screenY < 0 || screenY >= viewHeight || viewHeight <= 0 || width <= 0 {
		tuiManager.screen.HideCursor()
	} else {
		tuiManager.screen.ShowCursor(screenX, screenY)
	}
}