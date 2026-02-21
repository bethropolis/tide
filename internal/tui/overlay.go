package tui

import (
	"github.com/bethropolis/tide/internal/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// Overlay represents a floating UI element
type Overlay interface {
	Draw(screen tcell.Screen, t *theme.Theme, screenW, screenH int)
	HandleKeyEvent(ev *tcell.EventKey) bool // returns true if event was consumed
	IsActive() bool
}

// DrawBox is a utility to draw a bordered box on the screen
func DrawBox(screen tcell.Screen, x, y, width, height int, style tcell.Style) {
	// Draw corners
	screen.SetContent(x, y, '┌', nil, style)
	screen.SetContent(x+width-1, y, '┐', nil, style)
	screen.SetContent(x, y+height-1, '└', nil, style)
	screen.SetContent(x+width-1, y+height-1, '┘', nil, style)

	// Draw horizontal borders
	for col := x + 1; col < x+width-1; col++ {
		screen.SetContent(col, y, '─', nil, style)
		screen.SetContent(col, y+height-1, '─', nil, style)
	}

	// Draw vertical borders
	for row := y + 1; row < y+height-1; row++ {
		screen.SetContent(x, row, '│', nil, style)
		screen.SetContent(x+width-1, row, '│', nil, style)
	}

	// Fill background
	for row := y + 1; row < y+height-1; row++ {
		for col := x + 1; col < x+width-1; col++ {
			screen.SetContent(col, row, ' ', nil, style)
		}
	}
}

// DrawText is a utility to draw text at a given position, truncated to a max width
func DrawText(screen tcell.Screen, x, y, maxWidth int, text string, style tcell.Style) {
	col := x
	for _, r := range text {
		w := runewidth.RuneWidth(r)
		if col-x+w > maxWidth {
			break
		}
		screen.SetContent(col, y, r, nil, style)
		col += w
	}
}
