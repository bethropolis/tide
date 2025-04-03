// internal/core/editor.go
package core

import (
	"strings"      // Needed for Find
	"unicode/utf8" // Needed for rune handling

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
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

	// --- Selection State ---
	selecting      bool           // True if selection is active
	selectionStart types.Position // Anchor point of the selection
	selectionEnd   types.Position // Other end of the selection (usually Cursor position)

	// Clipboard State
	clipboard    []byte         // Internal register for yank/put
	eventManager *event.Manager // Added for dispatching events on delete etc.

	// Find State
	highlights []types.HighlightRegion // Store regions to highlight
}

// NewEditor creates a new Editor instance with a given buffer.
func NewEditor(buf buffer.Buffer) *Editor {
	return &Editor{
		buffer:         buf,
		Cursor:         types.Position{Line: 0, Col: 0},
		ViewportY:      0,
		ViewportX:      0,
		ScrollOff:      3,                                 // Default scroll-off value (can be configured later)
		selecting:      false,                             // Start with no selection
		selectionStart: types.Position{Line: -1, Col: -1}, // Invalid position indicates no anchor
		selectionEnd:   types.Position{Line: -1, Col: -1},
		clipboard:      nil,                              // Initialize clipboard as nil
		highlights:     make([]types.HighlightRegion, 0), // Initialize highlights slice
	}
}

// SetEventManager sets the event manager for dispatching events
func (e *Editor) SetEventManager(mgr *event.Manager) {
	e.eventManager = mgr
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
func (e *Editor) GetCursor() types.Position {
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

// SaveBuffer saves the buffer to disk
func (e *Editor) SaveBuffer() error {
	filePath := ""
	if bufWithFP, ok := e.buffer.(interface{ FilePath() string }); ok {
		filePath = bufWithFP.FilePath()
	}

	err := e.buffer.Save(filePath) // Pass buffer's path or let buffer handle it
	if err != nil {
		return err // Propagate error
	}
	return nil // Return nil on success
}

// --- Find Logic ---

// Find searches for a term starting from a position.
// For forward search: starts at or after startPos
// For backward search: searches strictly before startPos
func (e *Editor) Find(term string, startPos types.Position, forward bool) (types.Position, bool) {
	if term == "" {
		return types.Position{}, false
	}

	lineCount := e.buffer.LineCount()
	currentLine := startPos.Line
	currentCol := startPos.Col // This is rune index

	if forward {
		// Search from startPos to end of buffer
		for lineIdx := currentLine; lineIdx < lineCount; lineIdx++ {
			lineBytes, err := e.buffer.Line(lineIdx)
			if err != nil {
				continue
			} // Skip lines we can't read
			lineStr := string(lineBytes)

			searchStartCol := 0
			if lineIdx == currentLine {
				searchStartCol = currentCol // Start search from currentCol on the first line
			}

			// Convert rune column index to byte offset for strings.Index
			startByteOffset := runeIndexToByteOffset(lineBytes, searchStartCol)
			if startByteOffset < 0 {
				startByteOffset = 0 // Safe fallback if conversion fails
			}

			byteIndex := strings.Index(lineStr[startByteOffset:], term)
			if byteIndex != -1 {
				// Found a match, calculate its rune position
				matchByteOffset := startByteOffset + byteIndex
				matchRuneCol := byteOffsetToRuneIndex(lineBytes, matchByteOffset)
				return types.Position{Line: lineIdx, Col: matchRuneCol}, true
			}
		}
		// TODO: Add wrap-around search?
	} else {
		// Search from startPos backward to beginning of buffer
		for lineIdx := currentLine; lineIdx >= 0; lineIdx-- {
			lineBytes, err := e.buffer.Line(lineIdx)
			if err != nil {
				continue
			}
			lineStr := string(lineBytes)

			// For backward search on starting line, only search up to (but not including) the startPos.Col
			searchEndByteOffset := len(lineBytes) // Default to end of line
			if lineIdx == currentLine {
				// On starting line, convert startPos.Col to byte offset as the end limit
				searchEndByteOffset = runeIndexToByteOffset(lineBytes, currentCol)
				if searchEndByteOffset < 0 || searchEndByteOffset > len(lineBytes) {
					searchEndByteOffset = len(lineBytes) // Safe fallback
				}
			}

			// Only search the part before the cursor on starting line
			searchStr := lineStr[:searchEndByteOffset]
			byteIndex := strings.LastIndex(searchStr, term) // Find last occurrence

			if byteIndex != -1 {
				// Found, calculate rune position
				matchRuneCol := byteOffsetToRuneIndex(lineBytes, byteIndex)
				return types.Position{Line: lineIdx, Col: matchRuneCol}, true
			}
		}
		// TODO: Add wrap-around search?
	}

	return types.Position{}, false // Not found
}

// HighlightMatches finds all occurrences of a term and stores them for highlighting.
func (e *Editor) HighlightMatches(term string) {
	e.ClearHighlights() // Clear previous highlights first
	if term == "" {
		return
	}

	lineCount := e.buffer.LineCount()
	termRuneLen := utf8.RuneCountInString(term)

	for lineIdx := 0; lineIdx < lineCount; lineIdx++ {
		lineBytes, err := e.buffer.Line(lineIdx)
		if err != nil {
			continue
		}
		lineStr := string(lineBytes)

		currentByteOffset := 0
		for {
			byteIndex := strings.Index(lineStr[currentByteOffset:], term)
			if byteIndex == -1 {
				break
			} // No more matches on this line

			matchStartByteOffset := currentByteOffset + byteIndex
			matchStartCol := byteOffsetToRuneIndex(lineBytes, matchStartByteOffset)
			matchEndCol := matchStartCol + termRuneLen // End is exclusive

			e.highlights = append(e.highlights, types.HighlightRegion{
				Start: types.Position{Line: lineIdx, Col: matchStartCol},
				End:   types.Position{Line: lineIdx, Col: matchEndCol},
				Type:  types.HighlightSearch,
			})

			// Advance search position past the current match
			// Important: advance by bytes in the original string
			matchBytes := []byte(term) // Get bytes of term for len
			currentByteOffset = matchStartByteOffset + len(matchBytes)
			if currentByteOffset >= len(lineBytes) {
				break
			} // Avoid infinite loop if match is at end
		}
	}
	logger.Debugf("Editor: Added %d search highlights for '%s'", len(e.highlights), term)
}

// ClearHighlights removes all highlight regions.
func (e *Editor) ClearHighlights() {
	if len(e.highlights) > 0 {
		logger.Debugf("Editor: Clearing %d highlights", len(e.highlights))
		e.highlights = make([]types.HighlightRegion, 0)
	}
}

// GetHighlights returns the current highlight regions (for drawing).
func (e *Editor) GetHighlights() []types.HighlightRegion {
	return e.highlights
}

// --- Helper functions for rune/byte offset conversion ---

// runeIndexToByteOffset converts a rune index to a byte offset in a byte slice.
// Returns -1 if runeIndex is out of bounds.
func runeIndexToByteOffset(line []byte, runeIndex int) int {
	if runeIndex <= 0 {
		return 0
	}
	byteOffset := 0
	currentRune := 0
	for byteOffset < len(line) {
		if currentRune == runeIndex {
			return byteOffset
		}
		_, size := utf8.DecodeRune(line[byteOffset:])
		byteOffset += size
		currentRune++
	}
	if currentRune == runeIndex {
		return len(line)
	} // Allow index at the very end
	return -1 // Index out of bounds
}

// byteOffsetToRuneIndex converts a byte offset to a rune index in a byte slice.
func byteOffsetToRuneIndex(line []byte, byteOffset int) int {
	if byteOffset <= 0 {
		return 0
	}
	if byteOffset > len(line) {
		byteOffset = len(line)
	} // Clamp offset
	runeIndex := 0
	currentOffset := 0
	for currentOffset < byteOffset {
		_, size := utf8.DecodeRune(line[currentOffset:])
		if currentOffset+size > byteOffset {
			break
		} // Don't count rune if offset is within it
		currentOffset += size
		runeIndex++
	}
	return runeIndex
}
