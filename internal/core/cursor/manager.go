package cursor

import (
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/buffer" // Import main buffer package
	"github.com/bethropolis/tide/internal/config"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
	"github.com/rivo/uniseg"
)

// Manager handles cursor movement and viewport scrolling
type Manager struct {
	editor     EditorInterface
	position   types.Position
	viewportY  int // Top visible line
	viewportX  int // Leftmost visible rune index
	viewWidth  int // Terminal width
	viewHeight int // Terminal height (minus status bar)
	scrollOff  int // Lines to keep visible around cursor
}

// EditorInterface defines methods the cursor manager needs from the editor
type EditorInterface interface {
	GetBuffer() buffer.Buffer // Changed return type to concrete buffer.Buffer
	GetCursor() types.Position
}

// NewManager creates a new cursor manager
func NewManager(editor EditorInterface) *Manager {
	return &Manager{
		editor:    editor,
		position:  types.Position{Line: 0, Col: 0},
		viewportY: 0,
		viewportX: 0,
		scrollOff: config.DefaultScrollOff,
	}
}

// GetPosition returns the current cursor position
func (m *Manager) GetPosition() types.Position {
	return m.position
}

// SetPosition sets the cursor position with bounds checking
func (m *Manager) SetPosition(pos types.Position) {
	m.position = pos
	m.Move(0, 0) // Use Move to handle clamping
}

// GetViewport returns the viewport position
func (m *Manager) GetViewport() (int, int) {
	return m.viewportY, m.viewportX
}

// SetViewSize updates the viewport dimensions
func (m *Manager) SetViewSize(width, height int) {
	m.viewWidth = width
	if height > config.StatusBarHeight {
		m.viewHeight = height - config.StatusBarHeight
	} else {
		m.viewHeight = 0
	}

	// Adjust scrolloff if needed
	if m.scrollOff*2 >= m.viewHeight && m.viewHeight > 0 {
		m.scrollOff = (m.viewHeight - 1) / 2
	} else if m.viewHeight <= 0 {
		m.scrollOff = 0
	}

	m.ScrollToCursor()
}

// Move moves the cursor with boundary checks
func (m *Manager) Move(deltaLine, deltaCol int) {
	currentLine := m.position.Line
	currentCol := m.position.Col
	buffer := m.editor.GetBuffer()
	lineCount := buffer.LineCount()

	// --- Handle Horizontal Wrap-Around FIRST ---
	// If deltaLine is non-zero, we are moving vertically, wrap doesn't apply here.
	if deltaLine == 0 && lineCount > 0 {
		if deltaCol > 0 { // Attempting to move Right
			lineBytes, err := buffer.Line(currentLine)
			// Only wrap if we successfully read the current line
			if err == nil {
				maxCol := utf8.RuneCount(lineBytes)
				if currentCol >= maxCol && currentLine < lineCount-1 { // At or past EOL and not on the last line
					m.position.Line++  // Move to next line
					m.position.Col = 0 // Move to beginning of that line
					m.ScrollToCursor()
					return // Wrap handled
				}
			}
		} else if deltaCol < 0 { // Attempting to move Left
			// No need to read line content for BOL check
			if currentCol <= 0 && currentLine > 0 { // At or before BOL and not on the first line
				m.position.Line-- // Move to previous line
				// Now read the previous line to find its end
				prevLineBytes, err := buffer.Line(m.position.Line)
				if err == nil {
					m.position.Col = utf8.RuneCount(prevLineBytes) // Move to end of that line
				} else {
					m.position.Col = 0 // Fallback if error reading prev line
				}
				m.ScrollToCursor()
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
		targetLineBytes, err := buffer.Line(targetLine)
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
	m.position.Line = targetLine
	m.position.Col = targetCol

	// Ensure cursor is visible after move/clamp
	m.ScrollToCursor()
}

// ScrollToCursor ensures the cursor is visible in the viewport
func (m *Manager) ScrollToCursor() {
	if m.viewHeight <= 0 || m.viewWidth <= 0 {
		logger.Debugf("ScrollToCursor: View has no dimensions (%dx%d), skipping", m.viewWidth, m.viewHeight)
		return // Cannot scroll if view has no dimensions
	}

	// Effective scrolloff (cannot be larger than half the view height)
	effectiveScrollOff := m.scrollOff
	if effectiveScrollOff*2 >= m.viewHeight {
		if m.viewHeight > 0 {
			effectiveScrollOff = (m.viewHeight - 1) / 2 // Max half the height
		} else {
			effectiveScrollOff = 0
		}
	}

	// Store original viewport position for logging
	oldViewportY, oldViewportX := m.viewportY, m.viewportX

	// Vertical scrolling with scrolloff
	if m.position.Line < m.viewportY+effectiveScrollOff {
		// Cursor is within scrolloff distance from the top edge (or above)
		m.viewportY = m.position.Line - effectiveScrollOff
		if m.viewportY < 0 {
			m.viewportY = 0 // Don't scroll past the beginning of the file
		}
	} else if m.position.Line >= m.viewportY+m.viewHeight-effectiveScrollOff {
		// Cursor is within scrolloff distance from the bottom edge (or below)
		m.viewportY = m.position.Line - m.viewHeight + 1 + effectiveScrollOff
	}

	// --- Horizontal Scrolling (based on visual column) ---
	buffer := m.editor.GetBuffer()
	lineBytes, err := buffer.Line(m.position.Line)
	cursorVisualCol := 0
	if err == nil {
		cursorVisualCol = CalculateVisualColumn(lineBytes, m.position.Col)
	} else {
		logger.Debugf("ScrollToCursor: Error getting line %d: %v", m.position.Line, err)
		// Fallback if line fetch fails
	}

	// Simple horizontal scroll (no horizontal ScrollOff for now)
	if cursorVisualCol < m.viewportX {
		// Cursor's visual position is left of the viewport
		m.viewportX = cursorVisualCol
	} else if cursorVisualCol >= m.viewportX+m.viewWidth {
		// Cursor's visual position is right of the viewport edge
		// We want the cursor to be *at* the last visible column,
		// so ViewportX should be cursorVisualCol - viewWidth + 1
		m.viewportX = cursorVisualCol - m.viewWidth + 1
	}

	// Clamp viewport origins
	if m.viewportY < 0 {
		m.viewportY = 0
	}
	if m.viewportX < 0 {
		m.viewportX = 0
	}

	// Only log if viewport position changed
	if m.viewportY != oldViewportY || m.viewportX != oldViewportX {
		logger.Debugf("ScrollToCursor: Cursor(%d,%d) Viewport(%d,%d)→(%d,%d) ViewSize(%dx%d) ScrollOff(%d)",
			m.position.Line, m.position.Col, oldViewportY, oldViewportX,
			m.viewportY, m.viewportX, m.viewWidth, m.viewHeight, effectiveScrollOff)
	}
}

// PageMove moves the cursor by page increments
func (m *Manager) PageMove(deltaPages int) {
	if m.viewHeight <= 0 {
		return // Cannot page if view has no height
	}

	buffer := m.editor.GetBuffer()

	// Move cursor by viewHeight * deltaPages
	targetLine := m.position.Line + (m.viewHeight * deltaPages)

	// Clamp target line to buffer bounds
	lineCount := buffer.LineCount()
	if targetLine < 0 {
		targetLine = 0
	} else if targetLine >= lineCount {
		targetLine = lineCount - 1
	}

	// Set cursor position directly
	m.position.Line = targetLine
	// Try to maintain horizontal position (Col), clamping if necessary
	m.Move(0, 0) // Use Move to handle clamping Col and scroll

	// Explicitly move viewport - ScrollToCursor might not jump a full page
	m.viewportY += (m.viewHeight * deltaPages)
	// Clamp ViewportY after the jump
	if m.viewportY < 0 {
		m.viewportY = 0
	}
	maxViewportY := lineCount - m.viewHeight
	if maxViewportY < 0 {
		maxViewportY = 0
	}
	if m.viewportY > maxViewportY {
		m.viewportY = maxViewportY
	}

	// Final scroll check to ensure cursor visibility *after* the jump
	m.ScrollToCursor()
}

// MoveToLineStart moves cursor to beginning of line
func (m *Manager) MoveToLineStart() {
	m.position.Col = 0
	m.ScrollToCursor() // Ensure viewport adjusts if needed
}

// MoveToLineEnd moves cursor to end of line
func (m *Manager) MoveToLineEnd() {
	buffer := m.editor.GetBuffer()
	lineBytes, err := buffer.Line(m.position.Line)
	if err != nil {
		logger.Debugf("Error getting line %d for MoveToLineEnd: %v", m.position.Line, err)
		m.position.Col = 0 // Fallback to beginning
	} else {
		m.position.Col = utf8.RuneCount(lineBytes) // Move to position *after* last rune
	}
	m.ScrollToCursor() // Ensure viewport adjusts
}

// CalculateVisualColumn computes screen column width
func CalculateVisualColumn(line []byte, runeIndex int) int {
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
