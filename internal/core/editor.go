// internal/core/editor.go
package core

import (
	"regexp" // Needed for regex search
	"sync"   // Add sync package for mutex

	// Needed for Find
	"unicode/utf8" // Needed for rune handling

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/config"
	"github.com/bethropolis/tide/internal/event"
	hl "github.com/bethropolis/tide/internal/highlighter" // Import highlighter
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
	sitter "github.com/smacker/go-tree-sitter" // Import tree-sitter
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

	// Syntax Highlighting State
	highlighter      *hl.Highlighter    // Highlighter service instance
	syntaxHighlights hl.HighlightResult // Store computed syntax highlights
	syntaxTree       *sitter.Tree       // Store the current syntax tree
	highlightMutex   sync.RWMutex       // Mutex to protect syntaxHighlights & syntaxTree
}

// NewEditor creates a new Editor instance with a given buffer.
func NewEditor(buf buffer.Buffer) *Editor {
	return &Editor{
		buffer:           buf,
		Cursor:           types.Position{Line: 0, Col: 0},
		ViewportY:        0,
		ViewportX:        0,
		ScrollOff:        config.DefaultScrollOff,
		selecting:        false,                             // Start with no selection
		selectionStart:   types.Position{Line: -1, Col: -1}, // Invalid position indicates no anchor
		selectionEnd:     types.Position{Line: -1, Col: -1},
		clipboard:        nil,                              // Initialize clipboard as nil
		highlights:       make([]types.HighlightRegion, 0), // Initialize highlights slice
		syntaxHighlights: make(hl.HighlightResult),         // Initialize syntax map
		syntaxTree:       nil,                              // Initialize tree as nil
		highlightMutex:   sync.RWMutex{},                   // Initialize mutex
	}
}

// SetEventManager sets the event manager for dispatching events
func (e *Editor) SetEventManager(mgr *event.Manager) {
	e.eventManager = mgr
}

// SetHighlighter injects the highlighter service.
func (e *Editor) SetHighlighter(h *hl.Highlighter) {
	e.highlighter = h
}

// TriggerSyntaxHighlight is now a placeholder since highlighting is handled asynchronously
func (e *Editor) TriggerSyntaxHighlight() {
	// The actual triggering logic will move to a debounced async mechanism.
	logger.Debugf("Editor: TriggerSyntaxHighlight called (will be handled async now)")
}

// GetSyntaxHighlightsForLine returns the computed syntax styles for a given line number.
func (e *Editor) GetSyntaxHighlightsForLine(lineNum int) []types.StyledRange {
	e.highlightMutex.RLock()
	defer e.highlightMutex.RUnlock()

	if styles, ok := e.syntaxHighlights[lineNum]; ok {
		return styles
	}
	return nil // No highlights for this line
}

// updateSyntaxHighlights safely updates the highlights and tree
func (e *Editor) updateSyntaxHighlights(newHighlights hl.HighlightResult, newTree *sitter.Tree) {
	e.highlightMutex.Lock()
	defer e.highlightMutex.Unlock()

	// Close the old tree before replacing it
	if e.syntaxTree != nil {
		e.syntaxTree.Close()
	}

	e.syntaxHighlights = newHighlights
	e.syntaxTree = newTree // Store the new tree
}

// UpdateSyntaxHighlights is an exported wrapper around updateSyntaxHighlights
// for use by the highlight manager.
func (e *Editor) UpdateSyntaxHighlights(newHighlights hl.HighlightResult, newTree *sitter.Tree) {
	e.updateSyntaxHighlights(newHighlights, newTree)
}

// UpdateVisibleSyntaxHighlights updates only the highlights for visible lines and updates the tree.
// This allows for more responsive UI by prioritizing what the user can see.
func (e *Editor) UpdateVisibleSyntaxHighlights(visibleHighlights hl.HighlightResult, newTree *sitter.Tree) {
	e.highlightMutex.Lock()
	defer e.highlightMutex.Unlock()

	// Merge visible highlights into the main map
	for line, styles := range visibleHighlights {
		e.syntaxHighlights[line] = styles
	}

	// Update the tree
	if e.syntaxTree != nil {
		e.syntaxTree.Close()
	}
	e.syntaxTree = newTree
}

// GetCurrentTree safely gets the current tree (needed for incremental parse)
func (e *Editor) GetCurrentTree() *sitter.Tree {
	e.highlightMutex.RLock()
	defer e.highlightMutex.RUnlock()
	return e.syntaxTree
}

// SetViewSize updates the cached view dimensions. Called on resize or before drawing.
func (e *Editor) SetViewSize(width, height int) {
	e.viewWidth = width
	if height > config.StatusBarHeight {
		e.viewHeight = height - config.StatusBarHeight
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

	// Compile regex - potentially cache compiled regexes later for performance
	re, err := regexp.Compile(term)
	if err != nil {
		logger.Warnf("Find: Invalid regex '%s': %v", term, err)
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

			searchStartByteOffset := 0
			if lineIdx == currentLine {
				// On starting line, only search from current column onward
				searchStartByteOffset = runeIndexToByteOffset(lineBytes, currentCol)
				if searchStartByteOffset < 0 {
					searchStartByteOffset = 0 // Safe fallback
				}
			}

			// Search in the relevant portion of the line
			searchBytes := lineBytes[searchStartByteOffset:]
			loc := re.FindIndex(searchBytes)
			if loc != nil {
				// Found a match, calculate its rune position
				matchByteOffset := searchStartByteOffset + loc[0]
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

			searchEndByteOffset := len(lineBytes)
			if lineIdx == currentLine {
				// On starting line, only search up to current column
				searchEndByteOffset = runeIndexToByteOffset(lineBytes, currentCol)
				if searchEndByteOffset < 0 || searchEndByteOffset > len(lineBytes) {
					searchEndByteOffset = len(lineBytes) // Safe fallback
				}
			}

			// Only search the part before the cursor on starting line
			searchBytes := lineBytes[:searchEndByteOffset]

			// Find all matches, then get the last one
			locs := re.FindAllIndex(searchBytes, -1)
			if len(locs) > 0 {
				lastMatch := locs[len(locs)-1]
				matchByteOffset := lastMatch[0]
				matchRuneCol := byteOffsetToRuneIndex(lineBytes, matchByteOffset)
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

	re, err := regexp.Compile(term)
	if err != nil {
		logger.Warnf("HighlightMatches: Invalid regex '%s': %v", term, err)
		return
	}

	lineCount := e.buffer.LineCount()

	for lineIdx := 0; lineIdx < lineCount; lineIdx++ {
		lineBytes, err := e.buffer.Line(lineIdx)
		if err != nil {
			continue
		}

		// Find all non-overlapping matches on the line
		locs := re.FindAllIndex(lineBytes, -1)

		for _, loc := range locs {
			matchStartByteOffset := loc[0]
			matchEndByteOffset := loc[1] // Regexp gives exclusive end byte offset

			matchStartCol := byteOffsetToRuneIndex(lineBytes, matchStartByteOffset)
			matchEndCol := byteOffsetToRuneIndex(lineBytes, matchEndByteOffset)

			e.highlights = append(e.highlights, types.HighlightRegion{
				Start: types.Position{Line: lineIdx, Col: matchStartCol},
				End:   types.Position{Line: lineIdx, Col: matchEndCol},
				Type:  types.HighlightSearch,
			})
		}
	}
	logger.Debugf("Editor: Added %d regex search highlights for '%s'", len(e.highlights), term)
}

// HasHighlights checks if there are any active highlight regions.
func (e *Editor) HasHighlights() bool {
	return len(e.highlights) > 0
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
