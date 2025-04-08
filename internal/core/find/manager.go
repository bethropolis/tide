package find

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
)

// EditorInterface defines methods the find manager needs from the editor.
type EditorInterface interface {
	GetBuffer() buffer.Buffer
	GetCursor() types.Position
	SetCursor(types.Position)
	GetEventManager() *event.Manager
	ScrollToCursor()
}

// Manager handles find, replace, and search highlighting logic.
type Manager struct {
	editor            EditorInterface
	mutex             sync.RWMutex // Protects internal state
	searchHighlights  []types.HighlightRegion
	lastSearchTerm    string
	lastSearchRegex   *regexp.Regexp // Cache compiled regex
	lastMatchPos      *types.Position
	lastSearchForward bool
}

// NewManager creates a find manager.
func NewManager(editor EditorInterface) *Manager {
	return &Manager{
		editor:            editor,
		searchHighlights:  make([]types.HighlightRegion, 0),
		lastSearchForward: true, // Default search direction
	}
}

// FindNext finds the next occurrence and moves cursor to it.
func (m *Manager) FindNext(forward bool) (types.Position, bool) {
	m.mutex.Lock() // Lock for accessing lastSearchTerm etc.
	term := m.lastSearchTerm
	re := m.lastSearchRegex
	lastPos := m.lastMatchPos
	m.mutex.Unlock() // Unlock after reading shared state

	if term == "" || re == nil {
		return types.Position{}, false
	}

	startPos := m.editor.GetCursor()
	// If navigating and we have a last match, start search *after* it
	if lastPos != nil {
		startPos = *lastPos
		if forward {
			// Move one character forward to avoid finding the same match
			startPos.Col++
		}
		// For backward search, we'll find matches before the current match
	}

	foundPos, found := m.findInternal(re, startPos, forward)

	if found {
		// Update last match position
		m.mutex.Lock()
		m.lastMatchPos = &foundPos
		m.lastSearchForward = forward
		m.mutex.Unlock()

		return foundPos, true
	}

	return types.Position{}, false
}

// findInternal performs the actual search using buffer access.
func (m *Manager) findInternal(re *regexp.Regexp, startPos types.Position, forward bool) (types.Position, bool) {
	buf := m.editor.GetBuffer()
	lineCount := buf.LineCount()
	currentLine := startPos.Line
	currentCol := startPos.Col

	if forward {
		// Forward search (current line to end, then wrap to beginning)
		for lineIdx := currentLine; lineIdx < lineCount; lineIdx++ {
			lineBytes, err := buf.Line(lineIdx)
			if err != nil {
				continue
			}

			// Calculate byte offset for starting the search
			searchStartByteOffset := 0
			if lineIdx == currentLine {
				searchStartByteOffset = runeIndexToByteOffset(lineBytes, currentCol)
				if searchStartByteOffset < 0 {
					searchStartByteOffset = 0
				}
			}

			// Search from the current position to the end of line
			searchBytes := lineBytes[searchStartByteOffset:]
			loc := re.FindIndex(searchBytes)
			if loc != nil {
				// Found a match - convert byte offset back to rune column
				matchByteOffset := searchStartByteOffset + loc[0]
				matchRuneCol := byteOffsetToRuneIndex(lineBytes, matchByteOffset)
				return types.Position{Line: lineIdx, Col: matchRuneCol}, true
			}

			// Reset column position for next line
			currentCol = 0
		}

		// No match found - could implement wrap-around search here
	} else {
		// Backward search (current line to beginning, then wrap to end)
		for lineIdx := currentLine; lineIdx >= 0; lineIdx-- {
			lineBytes, err := buf.Line(lineIdx)
			if err != nil {
				continue
			}

			// For backward search on current line, we search up to the current position
			searchEndByteOffset := len(lineBytes)
			if lineIdx == currentLine {
				searchEndByteOffset = runeIndexToByteOffset(lineBytes, currentCol)
				if searchEndByteOffset < 0 || searchEndByteOffset > len(lineBytes) {
					searchEndByteOffset = len(lineBytes)
				}
			}

			// Search the line up to the current position
			searchBytes := lineBytes[:searchEndByteOffset]

			// Find all matches before the current position and take the last one
			locs := re.FindAllIndex(searchBytes, -1)
			if len(locs) > 0 {
				// Take the last match on the line (closest to the current position)
				lastMatch := locs[len(locs)-1]
				matchByteOffset := lastMatch[0]
				matchRuneCol := byteOffsetToRuneIndex(lineBytes, matchByteOffset)
				return types.Position{Line: lineIdx, Col: matchRuneCol}, true
			}
		}

		// No match found - could implement wrap-around search here
	}

	return types.Position{}, false
}

// HighlightMatches finds and stores all occurrences for highlighting.
func (m *Manager) HighlightMatches(term string) error {
	m.ClearHighlights() // Clear previous search highlights

	if term == "" {
		m.mutex.Lock()
		m.lastSearchTerm = ""
		m.lastSearchRegex = nil
		m.lastMatchPos = nil
		m.mutex.Unlock()
		return nil // Nothing to highlight
	}

	re, err := regexp.Compile(term)
	if err != nil {
		m.mutex.Lock()
		m.lastSearchTerm = term
		m.lastSearchRegex = nil // Mark regex as invalid
		m.lastMatchPos = nil
		m.mutex.Unlock()
		logger.Warnf("HighlightMatches: Invalid regex '%s': %v", term, err)
		return fmt.Errorf("invalid search pattern: %w", err)
	}

	m.mutex.Lock()
	m.lastSearchTerm = term
	m.lastSearchRegex = re
	m.lastMatchPos = nil // Reset last match position on new highlight
	m.mutex.Unlock()

	m.mutex.Lock() // Lock highlights for writing
	defer m.mutex.Unlock()

	buf := m.editor.GetBuffer()
	lineCount := buf.LineCount()
	newHighlights := make([]types.HighlightRegion, 0)

	for lineIdx := 0; lineIdx < lineCount; lineIdx++ {
		lineBytes, err := buf.Line(lineIdx)
		if err != nil {
			continue
		}

		locs := re.FindAllIndex(lineBytes, -1)
		for _, loc := range locs {
			matchStartByteOffset := loc[0]
			matchEndByteOffset := loc[1]
			matchStartCol := byteOffsetToRuneIndex(lineBytes, matchStartByteOffset)
			matchEndCol := byteOffsetToRuneIndex(lineBytes, matchEndByteOffset)

			newHighlights = append(newHighlights, types.HighlightRegion{
				Start: types.Position{Line: lineIdx, Col: matchStartCol},
				End:   types.Position{Line: lineIdx, Col: matchEndCol},
				Type:  types.HighlightSearch,
			})
		}
	}
	m.searchHighlights = newHighlights // Assign new highlights
	logger.Debugf("FindManager: Added %d search highlights for '%s'", len(m.searchHighlights), term)
	return nil
}

// ClearHighlights removes search highlight regions.
func (m *Manager) ClearHighlights() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if len(m.searchHighlights) > 0 {
		logger.Debugf("FindManager: Clearing %d search highlights", len(m.searchHighlights))
		m.searchHighlights = make([]types.HighlightRegion, 0) // Clear slice
	}
}

// HasHighlights checks if there are any search highlights.
func (m *Manager) HasHighlights() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.searchHighlights) > 0
}

// GetHighlights returns the current search highlight regions.
func (m *Manager) GetHighlights() []types.HighlightRegion {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Return a copy to avoid race conditions
	highlights := make([]types.HighlightRegion, len(m.searchHighlights))
	copy(highlights, m.searchHighlights)
	return highlights
}

// --- Replace Logic ---

// ParseSubstituteCommand parses the :s/pattern/replacement/[g] command string.
func ParseSubstituteCommand(cmdStr string) (pattern, replacement string, global bool, err error) {
	// Simple parsing, doesn't handle escaped delimiters yet
	parts := strings.SplitN(cmdStr, "/", 4) // Use '/' as delimiter
	if len(parts) < 3 || parts[0] != "" {   // Must start with '/'
		err = fmt.Errorf("invalid format: use /pattern/replacement/[g]")
		return
	}

	pattern = parts[1]
	replacement = parts[2]

	if pattern == "" {
		err = fmt.Errorf("search pattern cannot be empty")
		return
	}

	if len(parts) > 3 && strings.Contains(parts[3], "g") { // Check for 'g' flag
		global = true
	}
	// TODO: Add other flags like 'i' for case-insensitive?

	return // Return parsed values and nil error
}

// Replace replaces occurrences on the current line.
// For v1.0: Replaces only the FIRST occurrence on the CURRENT line by default.
// TODO: Add global flag support.
// TODO: Add range support.
// TODO: Add undo support.
func (m *Manager) Replace(patternStr, replacement string, global bool) (int, error) {
	if patternStr == "" {
		return 0, fmt.Errorf("search pattern cannot be empty")
	}

	re, err := regexp.Compile(patternStr)
	if err != nil {
		return 0, fmt.Errorf("invalid search pattern: %w", err)
	}

	buf := m.editor.GetBuffer()
	cursor := m.editor.GetCursor()
	lineIdx := cursor.Line
	eventMgr := m.editor.GetEventManager()

	lineBytes, err := buf.Line(lineIdx)
	if err != nil {
		return 0, fmt.Errorf("cannot get current line %d: %w", lineIdx, err)
	}

	matches := re.FindAllIndex(lineBytes, -1)
	if len(matches) == 0 {
		return 0, nil
	} // No matches on this line

	replaceCount := 0

	// For now, only replace the first match on the line
	// TODO: Implement 'g' flag and potentially start search from cursor col?
	loc := matches[0] // Get first match
	matchStartByte := loc[0]
	matchEndByte := loc[1]
	deletedText := lineBytes[matchStartByte:matchEndByte] // Text being replaced

	// Convert byte offsets to rune positions for buffer operations
	matchStartPos := types.Position{Line: lineIdx, Col: byteOffsetToRuneIndex(lineBytes, matchStartByte)}
	matchEndPos := types.Position{Line: lineIdx, Col: byteOffsetToRuneIndex(lineBytes, matchEndByte)}

	logger.Debugf("Replace: Found match '%s' at Line %d, Col %d-%d (Bytes %d-%d)",
		string(deletedText), lineIdx, matchStartPos.Col, matchEndPos.Col, matchStartByte, matchEndByte)

	// --- Perform Replace (Delete + Insert) ---
	// 1. Delete the matched text
	editInfoDel, errDel := buf.Delete(matchStartPos, matchEndPos)
	if errDel != nil {
		return 0, fmt.Errorf("replace failed during delete: %w", errDel)
	}

	// 2. Insert the replacement text
	replacementBytes := []byte(replacement)
	// Insert at the start position where the deletion happened
	editInfoIns, errIns := buf.Insert(matchStartPos, replacementBytes)
	if errIns != nil {
		return 0, fmt.Errorf("replace failed during insert: %w", errIns)
	}

	replaceCount = 1 // Only replaced one for now

	// --- Dispatch Events ---
	// We need to dispatch a single event representing the NET change for highlighting.
	// Calculate net EditInfo:
	netEditInfo := types.EditInfo{
		StartIndex:     editInfoDel.StartIndex, // Start index is the same
		StartPosition:  editInfoDel.StartPosition,
		OldEndIndex:    editInfoDel.OldEndIndex, // Original end of deleted text
		OldEndPosition: editInfoDel.OldEndPosition,
		NewEndIndex:    editInfoDel.StartIndex + uint32(len(replacementBytes)), // New end after insertion
		NewEndPosition: editInfoIns.NewEndPosition,                             // End position comes from the insert operation
	}
	if eventMgr != nil {
		eventMgr.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: netEditInfo})
	}

	// --- Update Cursor ---
	// Move cursor to the start of the replacement for now
	m.editor.SetCursor(matchStartPos)
	m.editor.ScrollToCursor()

	logger.Debugf("Replace: Replaced 1 occurrence.")
	return replaceCount, nil
}

// Utilities for byte offset / rune index conversion
func runeIndexToByteOffset(line []byte, runeIndex int) int {
	if runeIndex <= 0 {
		return 0
	}

	runeCount := 0
	for byteOffset := 0; byteOffset < len(line); {
		r, size := utf8.DecodeRune(line[byteOffset:])
		if r == utf8.RuneError {
			// If we encounter an error, just increment byte offset and continue
			byteOffset++
			continue
		}

		runeCount++
		if runeCount > runeIndex {
			return byteOffset
		}
		byteOffset += size
	}

	// If runeIndex is past the end of the line, return the line length
	return len(line)
}

func byteOffsetToRuneIndex(line []byte, byteOffset int) int {
	if byteOffset <= 0 {
		return 0
	}
	if byteOffset >= len(line) {
		return utf8.RuneCount(line)
	}

	return utf8.RuneCount(line[:byteOffset])
}
