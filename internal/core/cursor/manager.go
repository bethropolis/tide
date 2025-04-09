package cursor

import (
	"math" // Adding math import for Log10

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/config"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
)

// Editor is the interface cursor manager expects from the editor
type Editor interface {
	GetBuffer() buffer.Buffer
	ScrollOff() int
}

// Manager handles cursor positioning and viewport management
type Manager struct {
	editor       Editor
	position     types.Position
	viewportTop  int
	viewportLeft int
	viewWidth    int
	viewHeight   int
}

// NewManager creates a new cursor manager
func NewManager(editor Editor) *Manager {
	return &Manager{
		editor:       editor,
		position:     types.Position{Line: 0, Col: 0},
		viewportTop:  0,
		viewportLeft: 0,
	}
}

// SetViewSize updates the view dimensions
func (m *Manager) SetViewSize(width, height int) {
	m.viewWidth = width
	m.viewHeight = height
}

// GetViewport returns the current viewport top line and left column
func (m *Manager) GetViewport() (int, int) {
	return m.viewportTop, m.viewportLeft
}

// GetPosition returns the current cursor position
func (m *Manager) GetPosition() types.Position {
	return m.position
}

// SetPosition sets the cursor position
func (m *Manager) SetPosition(pos types.Position) {
	buf := m.editor.GetBuffer()
	if buf == nil {
		logger.Warnf("CursorManager.SetPosition: Buffer is nil")
		return
	}

	// Clamp to valid line range
	lineCount := buf.LineCount()
	if pos.Line < 0 {
		pos.Line = 0
	}
	if pos.Line >= lineCount {
		pos.Line = lineCount - 1
	}

	// Clamp to valid column range
	if pos.Col < 0 {
		pos.Col = 0
	}

	lineBytes, err := buf.Line(pos.Line)
	if err != nil {
		logger.Warnf("CursorManager.SetPosition: Failed to get line %d: %v", pos.Line, err)
		return
	}

	// Convert []byte to string for processing
	line := string(lineBytes)

	// Get visual line length (considering tabs)
	visualLen := GetVisualLineLength(line, config.Get().Editor.TabWidth)
	if pos.Col > visualLen {
		pos.Col = visualLen
	}

	m.position = pos
	m.ScrollToCursor()
}

// MoveCursor moves the cursor by the given delta
func (m *Manager) MoveCursor(deltaLine, deltaCol int) {
	newPos := types.Position{
		Line: m.position.Line + deltaLine,
		Col:  m.position.Col + deltaCol,
	}
	m.SetPosition(newPos)
}

// Move moves the cursor by the given delta
// This is an alias for MoveCursor to maintain API compatibility
func (m *Manager) Move(deltaLine, deltaCol int) {
	m.MoveCursor(deltaLine, deltaCol)
}

// PageMove moves the cursor by the given number of pages
func (m *Manager) PageMove(deltaPages int) {
	if m.viewHeight <= 0 {
		return // View not initialized
	}

	// Move cursor by viewHeight * deltaPages
	linesToMove := deltaPages * m.viewHeight
	m.MoveCursor(linesToMove, 0)
}

// MoveToStartOfLine moves the cursor to the first non-whitespace character
func (m *Manager) MoveToStartOfLine() {
	buf := m.editor.GetBuffer()
	if buf == nil {
		return
	}

	lineBytes, err := buf.Line(m.position.Line)
	if err != nil {
		return
	}

	// Convert []byte to string for processing
	line := string(lineBytes)

	// Find first non-whitespace character
	firstNonWS := 0
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			firstNonWS = i
			break
		}
	}

	m.SetPosition(types.Position{Line: m.position.Line, Col: firstNonWS})
}

// MoveToLineStart moves the cursor to the start of the current line
// This is an alias for MoveToStartOfLine to maintain API compatibility
func (m *Manager) MoveToLineStart() {
	m.MoveToStartOfLine()
}

// MoveToEndOfLine moves the cursor to the end of the current line
func (m *Manager) MoveToEndOfLine() {
	buf := m.editor.GetBuffer()
	if buf == nil {
		return
	}

	lineBytes, err := buf.Line(m.position.Line)
	if err != nil {
		return
	}

	// Convert []byte to string for processing
	line := string(lineBytes)

	visualLen := GetVisualLineLength(line, config.Get().Editor.TabWidth)
	m.SetPosition(types.Position{Line: m.position.Line, Col: visualLen})
}

// MoveToLineEnd moves the cursor to the end of the current line
// This is an alias for MoveToEndOfLine to maintain API compatibility
func (m *Manager) MoveToLineEnd() {
	m.MoveToEndOfLine()
}

// GetVisualCol translates a buffer column position to a visual column position,
// accounting for tab characters.
func (m *Manager) GetVisualCol(line string, col int) int {
	return GetVisualCol(line, col, config.Get().Editor.TabWidth)
}

// GetBufferCol translates a visual column position to a buffer column position,
// accounting for tab characters.
func (m *Manager) GetBufferCol(line string, visualCol int) int {
	return GetBufferCol(line, visualCol, config.Get().Editor.TabWidth)
}

// ScrollToCursor ensures the cursor is visible in the viewport
func (m *Manager) ScrollToCursor() {
	if m.viewHeight <= 0 || m.viewWidth <= 0 {
		// View not initialized yet
		return
	}

	// --- Calculate Gutter Width (needed for textAreaWidth) ---
	buffer := m.editor.GetBuffer()
	lineCount := buffer.LineCount()
	if lineCount == 0 {
		lineCount = 1
	}
	maxDigits := int(math.Log10(float64(lineCount))) + 1
	lineNumberPadding := 1
	gutterWidth := maxDigits + lineNumberPadding
	if gutterWidth >= m.viewWidth {
		gutterWidth = 0 // Disable if no space
	}
	// --- End Gutter Width ---

	effectiveScrollOff := m.editor.ScrollOff()
	if effectiveScrollOff < 0 {
		effectiveScrollOff = 0
	}

	oldViewportY, oldViewportX := m.viewportTop, m.viewportLeft

	// --- Vertical Scrolling ---
	if m.position.Line < m.viewportTop+effectiveScrollOff {
		// Cursor is above the viewport plus scroll-off
		m.viewportTop = m.position.Line - effectiveScrollOff
		if m.viewportTop < 0 {
			m.viewportTop = 0
		}
	} else if m.position.Line >= m.viewportTop+m.viewHeight-effectiveScrollOff {
		// Cursor is below the viewport minus scroll-off
		m.viewportTop = m.position.Line - m.viewHeight + effectiveScrollOff + 1
		if m.viewportTop < 0 {
			m.viewportTop = 0
		}
	}

	// --- Horizontal Scrolling (Refined) ---
	lineBytes, err := buffer.Line(m.position.Line)
	cursorVisualCol := 0 // Visual position relative to start of the line (col 0)
	if err == nil {
		tabWidth := config.Get().Editor.TabWidth // Get current tab width
		cursorVisualCol = GetVisualCol(string(lineBytes), m.position.Col, tabWidth)
	} else {
		logger.Warnf("ScrollToCursor: Failed to get line %d: %v", m.position.Line, err)
	}

	textAreaWidth := m.viewWidth - gutterWidth // Actual width available for text
	if textAreaWidth < 1 {
		textAreaWidth = 1 // Avoid division by zero or weirdness
	}

	newViewportX := m.viewportLeft // Start with current value

	if cursorVisualCol < m.viewportLeft {
		// Cursor went left of the visible area, scroll left to show it
		newViewportX = cursorVisualCol
	} else if cursorVisualCol >= m.viewportLeft+textAreaWidth {
		// Cursor went right of the visible area, scroll right to show it
		// Place cursor at the last column by setting viewportX appropriately
		newViewportX = cursorVisualCol - textAreaWidth + 1
	}

	// Clamp viewportX
	if newViewportX < 0 {
		newViewportX = 0
	}
	m.viewportLeft = newViewportX // Update the manager's state

	// --- Logging ---
	if m.viewportTop != oldViewportY || m.viewportLeft != oldViewportX {
		logger.DebugTagf("cursor",
			"ScrollToCursor: Cursor(L:%d, C:%d Vis:%d) Viewport(Y:%d->%d, X:%d->%d) TW:%d GH:%d TVW:%d",
			m.position.Line, m.position.Col, cursorVisualCol,
			oldViewportY, m.viewportTop, oldViewportX, m.viewportLeft,
			m.viewWidth, m.viewHeight, textAreaWidth)
	}
}

// GetVisualCol translates a buffer column to a visual column
func GetVisualCol(line string, col int, tabWidth int) int {
	visualCol := 0
	for i, ch := range line {
		if i >= col {
			break
		}
		if ch == '\t' {
			// Move to next tab stop
			visualCol = (visualCol/tabWidth + 1) * tabWidth
		} else {
			visualCol++
		}
	}
	return visualCol
}

// GetBufferCol translates a visual column to a buffer column
func GetBufferCol(line string, visualCol int, tabWidth int) int {
	currentVisual := 0
	for i, ch := range line {
		if currentVisual >= visualCol {
			return i
		}
		if ch == '\t' {
			// Move to next tab stop
			currentVisual = (currentVisual/tabWidth + 1) * tabWidth
		} else {
			currentVisual++
		}
	}
	return len(line) // Return end of line if visualCol is beyond line length
}

// GetVisualLineLength computes the visual length of a line
func GetVisualLineLength(line string, tabWidth int) int {
	return GetVisualCol(line, len(line), tabWidth)
}
