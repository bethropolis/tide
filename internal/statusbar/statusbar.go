// internal/statusbar/statusbar.go
package statusbar

import (
	"fmt"
	"sync"
	"time"

	// "github.com/bethropolis/tide/internal/core" // Might need editor state later
	"github.com/bethropolis/tide/internal/types" // For cursor position etc.
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg" // For proper Unicode width calculation
)

// Config defines the appearance and behavior of the status bar.
type Config struct {
	StyleDefault   tcell.Style // Default background/foreground
	StyleModified  tcell.Style // Style for the modified indicator
	StyleMessage   tcell.Style // Style for temporary messages
	StyleFindInput tcell.Style // Style for find mode input
	MessageTimeout time.Duration
}

// DefaultConfig provides sensible defaults.
func DefaultConfig() Config {
	return Config{
		StyleDefault:   tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorBlue),
		StyleModified:  tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorBlue).Bold(true),
		StyleMessage:   tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlue).Bold(true),
		StyleFindInput: tcell.StyleDefault.Foreground(tcell.ColorGreen).Background(tcell.ColorBlue).Bold(true), // Green for find input
		MessageTimeout: 4 * time.Second,
	}
}

// StatusBar represents the UI component for the status line.
type StatusBar struct {
	config Config
	mu     sync.RWMutex // Protect access to text fields

	// Content fields (will be updated externally)
	filePath   string
	cursorPos  types.Position
	isModified bool
	editorMode string // Placeholder for future modes (NORMAL, INSERT, etc.)

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

// ResetTemporaryMessage clears any temporary message being displayed
func (sb *StatusBar) ResetTemporaryMessage() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.tempMessage = ""
	sb.tempMessageTime = time.Time{}
}

// getDefaultDisplayText builds the default status line text.
func (sb *StatusBar) getDefaultDisplayText() string {
	// Assumes read lock is held or not needed if called from Draw where write lock is held
	fPath := sb.filePath
	if fPath == "" {
		fPath = "[No Name]"
	}
	modifiedIndicator := ""
	if sb.isModified {
		modifiedIndicator = " [Modified]"
	}

	// --- Format mode indicator ---
	modeIndicator := ""
	if sb.editorMode != "" {
		modeIndicator = fmt.Sprintf(" -- %s", sb.editorMode)
	}

	cursor := sb.cursorPos
	return fmt.Sprintf("%s%s -- Line: %d, Col: %d%s",
		fPath, modifiedIndicator, cursor.Line+1, cursor.Col+1, modeIndicator)
}

// Draw renders the status bar onto the screen using visual widths.
func (sb *StatusBar) Draw(screen tcell.Screen, width, height int) {
	if height <= 0 || width <= 0 {
		return
	}
	y := height - 1 // Status bar is always the last line

	sb.mu.Lock() // Lock for potential modification of tempMessageTime
	// Clear expired temporary message *before* getting display text
	isTempMsgActive := !sb.tempMessageTime.IsZero() && time.Since(sb.tempMessageTime) <= sb.config.MessageTimeout
	isFindInput := isTempMsgActive && len(sb.tempMessage) > 0 && sb.tempMessage[0] == '/' // Check for find input

	if !sb.tempMessageTime.IsZero() && !isTempMsgActive {
		sb.tempMessage = ""
		sb.tempMessageTime = time.Time{}
	}

	// Determine style and text based on whether a temporary message is active
	var style tcell.Style
	var text string

	if isTempMsgActive {
		text = sb.tempMessage
		if isFindInput {
			style = sb.config.StyleFindInput // Use find input style
		} else {
			style = sb.config.StyleMessage // Use regular message style
		}
	} else {
		text = sb.getDefaultDisplayText()
		style = sb.config.StyleDefault // Use default style
	}

	sb.mu.Unlock() // Unlock after accessing/modifying state

	// --- Actual Drawing ---
	// Fill background first
	for x := 0; x < width; x++ {
		screen.SetContent(x, y, ' ', nil, style) // Use determined style
	}

	// Draw text using uniseg for width calculation
	gr := uniseg.NewGraphemes(text)
	currentX := 0
	for gr.Next() {
		clusterWidth := gr.Width()
		if currentX+clusterWidth > width {
			break // Stop if cluster doesn't fit
		}

		runes := gr.Runes() // Get runes in the cluster

		// Draw the first rune of the cluster
		if len(runes) > 0 {
			mainRune := runes[0]
			var combiningRunes []rune
			if len(runes) > 1 {
				combiningRunes = runes[1:]
			}

			// Let tcell handle the rendering of the cluster
			screen.SetContent(currentX, y, mainRune, combiningRunes, style)
		}

		currentX += clusterWidth // Advance by the calculated visual width
	}
}
