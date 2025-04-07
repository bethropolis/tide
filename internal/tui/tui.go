// internal/tui/tui.go
package tui

import (
	"fmt" // Keep fmt if needed for error formatting

	"github.com/bethropolis/tide/internal/theme" // Import theme package
	"github.com/gdamore/tcell/v2"
)

// TUI manages the terminal screen using tcell.
type TUI struct {
	screen tcell.Screen
}

// New creates and initializes a new TUI instance.
func New() (*TUI, error) {
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, fmt.Errorf("failed to create tcell screen: %w", err)
	}
	if err := s.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize tcell screen: %w", err)
	}

	// Use the theme's default style for the screen background
	currentTheme := theme.GetCurrentTheme()
	defStyle := currentTheme.GetStyle("Default")
	s.SetStyle(defStyle)

	return &TUI{screen: s}, nil
}

// Close finalizes the tcell screen.
func (t *TUI) Close() {
	if t.screen != nil {
		t.screen.Fini()
	}
}

// PollEvent retrieves the next event.
func (t *TUI) PollEvent() tcell.Event {
	return t.screen.PollEvent()
}

// Clear clears the entire screen.
func (t *TUI) Clear() {
	t.screen.Clear()
}

// Show makes the changes visible.
func (t *TUI) Show() {
	t.screen.Show()
}

// Size returns the width and height of the terminal screen.
func (t *TUI) Size() (int, int) {
	return t.screen.Size()
}

// GetScreen provides direct access (use with caution).
func (t *TUI) GetScreen() tcell.Screen {
	return t.screen
}
