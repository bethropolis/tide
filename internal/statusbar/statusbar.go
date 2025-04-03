// internal/statusbar/statusbar.go
package statusbar

import (
	"fmt"
	"sync"
	"time"

	// "github.com/bethropolis/tide/internal/core" // Might need editor state later
	"github.com/bethropolis/tide/internal/types" // For cursor position etc.
	"github.com/gdamore/tcell/v2"
	// Consider "github.com/rivo/uniseg" for accurate width calculation later
)

// Config defines the appearance and behavior of the status bar.
type Config struct {
	StyleDefault   tcell.Style // Default background/foreground
	StyleModified  tcell.Style // Style for the modified indicator
	StyleMessage   tcell.Style // Style for temporary messages
	MessageTimeout time.Duration
}

// DefaultConfig provides sensible defaults.
func DefaultConfig() Config {
	return Config{
		StyleDefault:   tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorBlue),
		StyleModified:  tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorBlue).Bold(true),
		StyleMessage:   tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlue).Bold(true),
		MessageTimeout: 4 * time.Second,
	}
}

// StatusBar represents the UI component for the status line.
type StatusBar struct {
	config Config
	mu     sync.RWMutex // Protect access to text fields

	// Content fields (will be updated externally)
	filePath       string
	cursorPos      types.Position
	isModified     bool
	editorMode     string // Placeholder for future modes (NORMAL, INSERT, etc.)

	// Temporary message state
	tempMessage     string
	tempMessageTime time.Time
}

// New creates a new StatusBar with the given configuration.
func New(config Config) *StatusBar {
	return &StatusBar{
		config: config,
	}
}

// SetFileInfo updates the file path shown in the status bar.
func (sb *StatusBar) SetFileInfo(path string, modified bool) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.filePath = path
	sb.isModified = modified
}

// SetCursorInfo updates the cursor position shown.
func (sb *StatusBar) SetCursorInfo(pos types.Position) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.cursorPos = pos
}

// SetEditorMode updates the displayed editor mode.
func (sb *StatusBar) SetEditorMode(mode string) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.editorMode = mode
}

// SetTemporaryMessage displays a message for a configured duration.
func (sb *StatusBar) SetTemporaryMessage(format string, args ...interface{}) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.tempMessage = fmt.Sprintf(format, args...)
	sb.tempMessageTime = time.Now()
}

// getDisplayText determines the text to render based on current state.
func (sb *StatusBar) getDisplayText() string {
	sb.mu.RLock() // Use read lock for accessing state

	// Check if temporary message is active and not expired
	isTempMsgActive := !sb.tempMessageTime.IsZero() && time.Since(sb.tempMessageTime) <= sb.config.MessageTimeout
	if isTempMsgActive {
		// Clear expired message *after* check but before returning
        // This feels slightly wrong place, maybe clear in Draw?
        // Let's clear it here for simplicity now.
		// No, clearing should happen based on time passing, Draw is better.
		msg := sb.tempMessage
		sb.mu.RUnlock() // Unlock before returning
		return msg
	}

    // Clear expired message if checked above and it *was* expired
    // Moved clearing logic to Draw

	// Build the default status line if no active message
	fPath := sb.filePath
	if fPath == "" { fPath = "[No Name]" }

	modifiedIndicator := ""
	if sb.isModified { modifiedIndicator = " [Modified]" }

	modeIndicator := ""
	if sb.editorMode != "" { modeIndicator = fmt.Sprintf(" [%s]", sb.editorMode) } // Example mode

	// Using RLocked values
	cursor := sb.cursorPos
	text := fmt.Sprintf("%s%s%s -- Line: %d, Col: %d",
		fPath, modifiedIndicator, modeIndicator, cursor.Line+1, cursor.Col+1)

	sb.mu.RUnlock()
	return text
}

// Draw renders the status bar onto the screen.
func (sb *StatusBar) Draw(screen tcell.Screen, width, height int) {
	if height <= 0 || width <= 0 { return }
	y := height - 1 // Status bar is always the last line

	sb.mu.Lock() // Lock for potential modification of tempMessageTime
	// Clear expired temporary message *before* getting display text
	isTempMsgActive := !sb.tempMessageTime.IsZero() && time.Since(sb.tempMessageTime) <= sb.config.MessageTimeout
	if !sb.tempMessageTime.IsZero() && !isTempMsgActive {
		sb.tempMessage = ""
		sb.tempMessageTime = time.Time{}
	}
	// Determine style and text based on whether a temporary message is active
	var style tcell.Style
	var text string
	if isTempMsgActive {
		style = sb.config.StyleMessage
		text = sb.tempMessage // Use the message stored when SetTemporaryMessage was called
	} else {
		// Build default status text (could call getDisplayText but need lock anyway)
		fPath := sb.filePath
		if fPath == "" { fPath = "[No Name]" }
		modifiedIndicator := ""
		if sb.isModified { modifiedIndicator = " [Modified]" } // TODO: Apply StyleModified to this part?
		modeIndicator := ""
		if sb.editorMode != "" { modeIndicator = fmt.Sprintf(" [%s]", sb.editorMode) }
		cursor := sb.cursorPos
		text = fmt.Sprintf("%s%s%s -- Line: %d, Col: %d", fPath, modifiedIndicator, modeIndicator, cursor.Line+1, cursor.Col+1)
		style = sb.config.StyleDefault // Use default style
	}
	sb.mu.Unlock() // Unlock after accessing/modifying state

	// --- Actual Drawing ---
	// Fill background first
	for x := 0; x < width; x++ {
		screen.SetContent(x, y, ' ', nil, style) // Use determined style
	}

	// Draw the text runes
	// TODO: Enhance to handle different styles for parts (e.g., modified indicator)
	currentX := 0
	for _, r := range text {
		if currentX >= width { break }
		// runeWidth := uniseg.Width(r) // Use later
		runeWidth := 1
		if currentX+runeWidth <= width {
			screen.SetContent(currentX, y, r, nil, style)
		}
		currentX += runeWidth
	}
}