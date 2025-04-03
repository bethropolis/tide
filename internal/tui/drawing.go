// internal/tui/drawing.go
package tui

import (
	// "fmt" // No longer needed here directly
	//"unicode/utf8"

	"github.com/bethropolis/tide/internal/core" // Need editor state
	// "github.com/bethropolis/tide/internal/types" // Included via core
	"github.com/gdamore/tcell/v2"
)

// DrawBuffer draws the *visible* portion based on editor's viewport.
func DrawBuffer(tuiManager *TUI, editor *core.Editor) {
	// Use default style for now, could be passed or configured
	style := tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)

	width, height := tuiManager.Size()
	viewY, viewX := editor.GetViewport()
	statusBarHeight := 1
	viewHeight := height - statusBarHeight
	if viewHeight <= 0 || width <= 0 { return }

	lines := editor.GetBuffer().Lines()

	for screenY := 0; screenY < viewHeight; screenY++ {
		bufferLineIdx := screenY + viewY
		if bufferLineIdx >= 0 && bufferLineIdx < len(lines) {
			lineBytes := lines[bufferLineIdx]
			lineRunes := []rune(string(lineBytes))

			startRuneIdx := viewX
			endRuneIdx := viewX + width

			currentScreenX := 0
			for runeIdx := startRuneIdx; runeIdx < endRuneIdx && runeIdx < len(lineRunes); runeIdx++ {
				if runeIdx < 0 { continue }
				r := lineRunes[runeIdx]
				if r == '\t' {
					tabWidth := 4
					spacesToDraw := tabWidth - (currentScreenX % tabWidth)
					for i := 0; i < spacesToDraw && currentScreenX < width; i++ {
						tuiManager.screen.SetContent(currentScreenX, screenY, ' ', nil, style)
						currentScreenX++
					}
				} else {
					// Handle potential zero-width characters or double-width characters?
					// For now, assume fixed width. Libraries like `rivo/uniseg` help here.
					// runeWidth := uniseg.StringWidth(string(r)) // Example with uniseg
                    runeWidth := 1 // Assume 1 for now
					if currentScreenX < width {
						tuiManager.screen.SetContent(currentScreenX, screenY, r, nil, style)
					}
					currentScreenX += runeWidth
				}
				if currentScreenX >= width { break }
			}
		}
	}
}

// DrawStatusBar draws the status line with the provided text.
func DrawStatusBar(tuiManager *TUI, editor *core.Editor, text string) {
	// editor *core.Editor might not be needed if text is pre-formatted
	// but could be used for mode indicators etc. later.

	_, height := tuiManager.Size()
	if height <= 0 { return }
	y := height - 1

	style := tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorBlue)
	width, _ := tuiManager.Size()

	// Fill background
	for x := 0; x < width; x++ {
		tuiManager.screen.SetContent(x, y, ' ', nil, style)
	}

	// Draw text runes (handles unicode better than DrawText)
	runes := []rune(text)
	for x := 0; x < width && x < len(runes); x++ {
		// runeWidth := uniseg.StringWidth(string(runes[x])) // Use later if needed
        runeWidth := 1
		if x+runeWidth <= width {
			tuiManager.screen.SetContent(x, y, runes[x], nil, style)
		}
		x += runeWidth -1 // Adjust loop counter for multi-width runes
	}
}

// DrawCursor positions the terminal cursor relative to the viewport.
func DrawCursor(tuiManager *TUI, editor *core.Editor) {
	cursor := editor.GetCursor()
	viewY, viewX := editor.GetViewport()

	screenX := cursor.Col - viewX
	screenY := cursor.Line - viewY

	width, height := tuiManager.Size()
	statusBarHeight := 1
	viewHeight := height - statusBarHeight
	if screenX < 0 || screenX >= width || screenY < 0 || screenY >= viewHeight || viewHeight <= 0 || width <= 0 {
		tuiManager.screen.HideCursor()
	} else {
		// TODO: Calculate screenX more accurately if using variable width runes
		// Need to sum widths of runes from viewX up to cursor.Col
		// For now, screenX assumes fixed width.
		tuiManager.screen.ShowCursor(screenX, screenY)
	}
}