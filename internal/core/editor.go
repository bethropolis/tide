// internal/core/editor.go
package core

import (
	"fmt"
	"log"
	"unicode/utf8"

	// Import the buffer INTERFACE definition
	"github.com/bethropolis/tide/internal/buffer"
	// Import the new shared types package
	"github.com/bethropolis/tide/internal/types"
)

type Editor struct {
	buffer     buffer.Buffer
	Cursor     types.Position
	ViewportY  int // Top visible line index (0-based)
	ViewportX  int // Leftmost visible *rune* index (0-based) - Horizontal scroll
	viewWidth  int // Cached terminal width
	viewHeight int // Cached terminal height (excluding status bar)
	ScrollOff  int // Number of lines to keep visible above/below cursor
	// statusMessage string
	// statusTime    time.Time
}

// NewEditor creates a new Editor instance with a given buffer.
func NewEditor(buf buffer.Buffer) *Editor {
	return &Editor{
		buffer:    buf,
		Cursor:    types.Position{Line: 0, Col: 0},
		ViewportY: 0,
		ViewportX: 0,
		ScrollOff: 3, // Default scroll-off value (can be configured later)
	}
}

// SetViewSize updates the cached view dimensions. Called on resize or before drawing.
func (e *Editor) SetViewSize(width, height int) {
	statusBarHeight := 1 // Assuming 1 line for status bar
	e.viewWidth = width
	// Ensure viewHeight is not negative if terminal is too small
	if height > statusBarHeight {
		e.viewHeight = height - statusBarHeight
	} else {
		e.viewHeight = 0 // No space to draw buffer
	}

	// Ensure scrolloff isn't larger than half the view height
	if e.ScrollOff*2 >= e.viewHeight && e.viewHeight > 0 {
		e.ScrollOff = (e.viewHeight - 1) / 2
	} else if e.viewHeight <= 0 {
		e.ScrollOff = 0 // No scrolling if no view height
	}

	// After resize, we might need to adjust viewport/cursor
	e.ScrollToCursor() // Ensure cursor is visible after resize
}

// GetBuffer returns the editor's buffer.
func (e *Editor) GetBuffer() buffer.Buffer {
	return e.buffer
}

// GetCursor returns the current cursor position.
func (e *Editor) GetCursor() types.Position { // Return types.Position
	return e.Cursor
}

// SetCursor sets the current cursor position (add clamping later).
func (e *Editor) SetCursor(pos types.Position) {
	// Clamp cursor position first (using MoveCursor logic is easier)
	e.Cursor = pos     // Set temporarily
	e.MoveCursor(0, 0) // Use MoveCursor to handle clamping
	// MoveCursor already calls ScrollToCursor
}

func (e *Editor) GetViewport() (int, int) {
	return e.ViewportY, e.ViewportX
}

func (e *Editor) InsertRune(r rune) error {
	runeBytes := make([]byte, utf8.RuneLen(r))
	utf8.EncodeRune(runeBytes, r)

	currentPos := e.Cursor
	err := e.buffer.Insert(currentPos, runeBytes)
	if err != nil {
		return err
	}

	// Move cursor forward
	if r == '\n' {
		e.Cursor.Line++
		e.Cursor.Col = 0
	} else {
		e.Cursor.Col++
	}

	// Ensure cursor remains visible after insertion/movement
	e.ScrollToCursor()
	return nil
}

// InsertNewLine inserts a newline and scrolls.
func (e *Editor) InsertNewLine() error {
	// InsertRune handles the scroll now
	return e.InsertRune('\n')
}

func (e *Editor) DeleteBackward() error {
	start := e.Cursor
	end := e.Cursor

	if e.Cursor.Col > 0 {
		start.Col--
	} else if e.Cursor.Line > 0 {
		start.Line--
		prevLineBytes, err := e.buffer.Line(start.Line)
		if err != nil {
			return fmt.Errorf("cannot get previous line %d: %w", start.Line, err)
		}
		start.Col = utf8.RuneCount(prevLineBytes)
	} else {
		return nil
	}

	err := e.buffer.Delete(start, end)
	if err != nil {
		return fmt.Errorf("buffer delete failed: %w", err)
	}

	// Cursor moves to the 'start' position
	e.Cursor = start
	// Ensure cursor is visible after deletion/movement
	e.ScrollToCursor()
	return nil
}

// DeleteForward deletes and scrolls if needed.
func (e *Editor) DeleteForward() error {
	start := e.Cursor
	end := e.Cursor

	lineBytes, err := e.buffer.Line(e.Cursor.Line)
	if err != nil {
		return fmt.Errorf("cannot get current line %d: %w", e.Cursor.Line, err)
	}
	lineRuneCount := utf8.RuneCount(lineBytes)

	if e.Cursor.Col < lineRuneCount {
		end.Col++
	} else if e.Cursor.Line < e.buffer.LineCount()-1 {
		end.Line++
		end.Col = 0
	} else {
		return nil
	}

	err = e.buffer.Delete(start, end)
	if err != nil {
		return fmt.Errorf("buffer delete failed: %w", err)
	}

	// Cursor position remains at 'start'
	e.Cursor = start
	// Ensure cursor is visible (though it didn't move, viewport might need adjustment if lines were merged?)
	// Let's be safe and scroll anyway.
	e.ScrollToCursor()
	return nil
}

// SaveBuffer (remains the same)
func (e *Editor) SaveBuffer() error {
	filePath := ""
	if bufWithFP, ok := e.buffer.(interface{ FilePath() string }); ok {
		filePath = bufWithFP.FilePath()
	}

	err := e.buffer.Save(filePath) // Pass buffer's path or let buffer handle it
	if err != nil {
		// log.Printf("Error saving buffer to %s: %v", filePath, err) // REMOVE LOG
		return err // Propagate error
	}
	// log.Printf("Buffer saved successfully to %s.", filePath) // REMOVE LOG
	return nil // Return nil on success
}

// MoveCursor moves the cursor AND adjusts the viewport, handling line wraps.
func (e *Editor) MoveCursor(deltaLine, deltaCol int) {
	currentLine := e.Cursor.Line
	currentCol := e.Cursor.Col
	lineCount := e.buffer.LineCount()

	// --- Handle Horizontal Wrap-Around FIRST ---
	// If deltaLine is non-zero, we are moving vertically, wrap doesn't apply here.
	if deltaLine == 0 && lineCount > 0 {
		if deltaCol > 0 { // Attempting to move Right
			lineBytes, err := e.buffer.Line(currentLine)
			// Only wrap if we successfully read the current line
			if err == nil {
				maxCol := utf8.RuneCount(lineBytes)
				if currentCol >= maxCol && currentLine < lineCount-1 { // At or past EOL and not on the last line
					e.Cursor.Line++  // Move to next line
					e.Cursor.Col = 0 // Move to beginning of that line
					e.ScrollToCursor()
					return // Wrap handled
				}
			}
		} else if deltaCol < 0 { // Attempting to move Left
			// No need to read line content for BOL check
			if currentCol <= 0 && currentLine > 0 { // At or before BOL and not on the first line
				e.Cursor.Line-- // Move to previous line
				// Now read the previous line to find its end
				prevLineBytes, err := e.buffer.Line(e.Cursor.Line)
				if err == nil {
					e.Cursor.Col = utf8.RuneCount(prevLineBytes) // Move to end of that line
				} else {
					e.Cursor.Col = 0 // Fallback if error reading prev line
				}
				e.ScrollToCursor()
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
		targetLineBytes, err := e.buffer.Line(targetLine)
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
	e.Cursor.Line = targetLine
	e.Cursor.Col = targetCol

	// Ensure cursor is visible after move/clamp
	e.ScrollToCursor()
}

func (e *Editor) ScrollToCursor() {
	if e.viewHeight <= 0 || e.viewWidth <= 0 {
		return // Cannot scroll if view has no dimensions
	}

	// Effective scrolloff (cannot be larger than half the view height)
	effectiveScrollOff := e.ScrollOff
	if effectiveScrollOff*2 >= e.viewHeight {
		if e.viewHeight > 0 {
			effectiveScrollOff = (e.viewHeight - 1) / 2 // Max half the height
		} else {
			effectiveScrollOff = 0
		}
	}

	// Vertical scrolling with scrolloff
	if e.Cursor.Line < e.ViewportY+effectiveScrollOff {
		// Cursor is within scrolloff distance from the top edge (or above)
		e.ViewportY = e.Cursor.Line - effectiveScrollOff
		if e.ViewportY < 0 {
			e.ViewportY = 0 // Don't scroll past the beginning of the file
		}
	} else if e.Cursor.Line >= e.ViewportY+e.viewHeight-effectiveScrollOff {
		// Cursor is within scrolloff distance from the bottom edge (or below)
		e.ViewportY = e.Cursor.Line - e.viewHeight + 1 + effectiveScrollOff
		// Optional: Prevent ViewportY from scrolling too far past the end?
		// maxViewportY := e.buffer.LineCount() - e.viewHeight
		// if maxViewportY < 0 { maxViewportY = 0 }
		// if e.ViewportY > maxViewportY { e.ViewportY = maxViewportY }
	}

	// Horizontal scrolling (simple version, no scrolloff for now)
	// Note: Horizontal scrolloff is less common and more complex with variable width chars
	if e.Cursor.Col < e.ViewportX {
		// Cursor is left of the viewport
		e.ViewportX = e.Cursor.Col
	} else if e.Cursor.Col >= e.ViewportX+e.viewWidth {
		// Cursor is right of the viewport
		e.ViewportX = e.Cursor.Col - e.viewWidth + 1
	}

	// Clamp viewport origins just in case
	if e.ViewportY < 0 {
		e.ViewportY = 0
	}
	if e.ViewportX < 0 {
		e.ViewportX = 0
	}
}

// --- New Navigation Methods ---

// PageMove moves the cursor and viewport up or down by one page height.
// 'deltaPages' is typically +1 (PageDown) or -1 (PageUp).
func (e *Editor) PageMove(deltaPages int) {
	if e.viewHeight <= 0 {
		return // Cannot page if view has no height
	}

	// Move cursor by viewHeight * deltaPages
	targetLine := e.Cursor.Line + (e.viewHeight * deltaPages)

	// Clamp target line to buffer bounds
	lineCount := e.buffer.LineCount()
	if targetLine < 0 {
		targetLine = 0
	} else if targetLine >= lineCount {
		targetLine = lineCount - 1
	}

	// Set cursor position directly (MoveCursor would clamp differently)
	e.Cursor.Line = targetLine
	// Try to maintain horizontal position (Col), clamping if necessary
	e.MoveCursor(0, 0) // Use MoveCursor's logic to clamp Col and scroll

	// Explicitly move viewport - ScrollToCursor might not jump a full page
	e.ViewportY += (e.viewHeight * deltaPages)
	// Clamp ViewportY after the jump
	if e.ViewportY < 0 {
		e.ViewportY = 0
	}
	maxViewportY := lineCount - e.viewHeight
	if maxViewportY < 0 {
		maxViewportY = 0
	}
	if e.ViewportY > maxViewportY {
		e.ViewportY = maxViewportY
	}

	// Final scroll check to ensure cursor visibility *after* the jump
	e.ScrollToCursor()
}

// Home moves the cursor to the beginning of the current line (column 0).
func (e *Editor) Home() {
	e.Cursor.Col = 0
	e.ScrollToCursor() // Ensure viewport adjusts if needed
}

// End moves the cursor to the end of the current line.
func (e *Editor) End() {
	lineBytes, err := e.buffer.Line(e.Cursor.Line)
	if err != nil {
		log.Printf("Error getting line %d for End key: %v", e.Cursor.Line, err)
		e.Cursor.Col = 0 // Fallback to beginning
	} else {
		e.Cursor.Col = utf8.RuneCount(lineBytes) // Move to position *after* last rune
	}
	e.ScrollToCursor() // Ensure viewport adjusts
}
