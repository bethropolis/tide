// internal/statusbar/statusbar.go
package statusbar

import (
	"fmt" // Import strings for string operations
	"strings"
	"sync"
	"time"

	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/theme" // Import theme package
	"github.com/bethropolis/tide/internal/types" // For cursor position etc.
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg" // For proper Unicode width calculation
)

// Config defines the appearance and behavior of the status bar.
type Config struct {
	MessageTimeout time.Duration
}

// DefaultConfig provides sensible defaults.
func DefaultConfig() Config {
	return Config{
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

// Draw renders the status bar onto the screen using segmented styles.
func (sb *StatusBar) Draw(screen tcell.Screen, width, height int, activeTheme *theme.Theme) {
	if height <= 0 || width <= 0 {
		return
	}
	y := height - 1 // Status bar is always the last line

	logger.DebugTagf("draw", "statusbar.Draw: Drawing on Y=%d (Screen Height=%d)", y, height)

	// Ensure theme is valid
	if activeTheme == nil {
		activeTheme = theme.GetCurrentTheme() // Fallback to current theme
	}

	// Get base style for filling and separators
	baseStyle := activeTheme.GetStyle("StatusBar")

	// --- Fill Background ---
	// Fill the entire status bar row with the base style first.
	for x := 0; x < width; x++ {
		screen.SetContent(x, y, ' ', nil, baseStyle)
	}

	sb.mu.RLock() // Use RLock for reading state
	tempMsg := sb.tempMessage
	tempMsgTime := sb.tempMessageTime
	fPath := sb.filePath
	isMod := sb.isModified
	cursor := sb.cursorPos
	mode := sb.editorMode
	sb.mu.RUnlock() // Unlock after reading

	isTempMsgActive := !tempMsgTime.IsZero() && time.Since(tempMsgTime) <= sb.config.MessageTimeout
	currentX := 0 // Tracks horizontal drawing position

	if isTempMsgActive {
		// --- Draw Temporary Message ---
		var msgStyle tcell.Style
		isCommandInput := len(tempMsg) > 0 && tempMsg[0] == ':'
		isFindInput := len(tempMsg) > 0 && tempMsg[0] == '/'

		if isCommandInput {
			msgStyle = activeTheme.GetStyle("StatusBar.CommandInput")
		} else if isFindInput {
			msgStyle = activeTheme.GetStyle("StatusBar.FindInput")
		} else {
			msgStyle = activeTheme.GetStyle("StatusBar.Message")
		}
		// Draw the message using the helper, allowing it to fill available width
		drawSegment(screen, currentX, y, tempMsg, msgStyle, width)

	} else {
		// --- Draw Default Segments ---
		padding := " " // Space between segments

		// 1. Filename
		filenameStyle := activeTheme.GetStyle("StatusBar.Filename")
		displayPath := fPath
		if displayPath == "" {
			displayPath = "[No Name]"
		}
		currentX = drawSegment(screen, currentX, y, displayPath, filenameStyle, width)

		// 2. Modified Indicator
		if isMod {
			modifiedStyle := activeTheme.GetStyle("StatusBar.Modified")
			currentX = drawSegment(screen, currentX, y, padding+"[Modified]", modifiedStyle, width)
		}

		// Prepare right-aligned segments (calculate their total width first)
		cursorStr := fmt.Sprintf("Line: %d, Col: %d", cursor.Line+1, cursor.Col+1)
		modeStr := strings.ToUpper(mode)
		separator := " -- "

		cursorWidth := uniseg.StringWidth(cursorStr)
		modeWidth := uniseg.StringWidth(modeStr)
		separatorWidth := uniseg.StringWidth(separator)

		// Calculate start position for right-aligned block
		rightBlockWidth := cursorWidth + separatorWidth + modeWidth
		rightStartX := width - rightBlockWidth

		// Only draw right block if it doesn't overlap with left block (filename + modified)
		if rightStartX > currentX+uniseg.StringWidth(padding) { // Ensure space for padding
			// 3. Cursor Info (Right Aligned)
			cursorStyle := activeTheme.GetStyle("StatusBar.CursorInfo")
			currentX = drawSegment(screen, rightStartX, y, cursorStr, cursorStyle, width)

			// 4. Separator
			currentX = drawSegment(screen, currentX, y, separator, baseStyle, width)

			// 5. Mode
			modeStyle := activeTheme.GetStyle("StatusBar.Mode")
			drawSegment(screen, currentX, y, modeStr, modeStyle, width)
		} else {
			// Not enough space, maybe just draw cursor info if possible
			rightStartX = width - cursorWidth
			if rightStartX > currentX+uniseg.StringWidth(padding) {
				cursorStyle := activeTheme.GetStyle("StatusBar.CursorInfo")
				drawSegment(screen, rightStartX, y, cursorStr, cursorStyle, width)
			}
			logger.DebugTagf("draw", "Status bar segments overlap, omitting mode info.")
		}
	}
}

// drawSegment draws text at a given position with a specific style,
// handling clipping and returning the next available X coordinate.
func drawSegment(screen tcell.Screen, x, y int, text string, style tcell.Style, maxWidth int) int {
	if x >= maxWidth {
		return x // Already past the edge
	}

	gr := uniseg.NewGraphemes(text)
	startX := x // Remember start for clipping check

	for gr.Next() {
		clusterWidth := gr.Width()
		// Check if the *entire cluster* fits before drawing
		if x+clusterWidth > maxWidth {
			// Stop drawing at the screen edge
			return maxWidth
		}

		runes := gr.Runes()
		if len(runes) > 0 {
			screen.SetContent(x, y, runes[0], runes[1:], style)
			// Fill for wide characters
			for wc := 1; wc < clusterWidth; wc++ {
				screen.SetContent(x+wc, y, ' ', nil, style) // Fill with spaces using same style
			}
		}
		x += clusterWidth
	}
	// Log if nothing was drawn but text wasn't empty
	if x == startX && len(text) > 0 {
		logger.DebugTagf("draw", "drawSegment: Text '%s' completely clipped at x=%d, maxWidth=%d", text, startX, maxWidth)
	}
	return x // Return position after the drawn segment
}
