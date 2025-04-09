package find

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/core/history"
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
	GetHistoryManager() *history.Manager
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
// Now returns wrapped status as the third return value.
func (m *Manager) FindNext(forward bool) (types.Position, bool, bool) {
	m.mutex.Lock() // Lock for accessing lastSearchTerm etc.
	term := m.lastSearchTerm
	re := m.lastSearchRegex
	lastPos := m.lastMatchPos
	m.mutex.Unlock() // Unlock after reading shared state

	if term == "" || re == nil {
		return types.Position{}, false, false
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

	foundPos, found, wrapped := m.findInternal(re, startPos, forward)

	if found {
		// Update last match position
		m.mutex.Lock()
		m.lastMatchPos = &foundPos
		m.lastSearchForward = forward
		m.mutex.Unlock()

		return foundPos, true, wrapped
	}

	return types.Position{}, false, false
}

// findInternal performs the actual search using buffer access.
// Returns position, found status, and wrap status.
func (m *Manager) findInternal(re *regexp.Regexp, startPos types.Position, forward bool) (pos types.Position, found bool, wrapped bool) {
	buf := m.editor.GetBuffer()
	lineCount := buf.LineCount()
	if lineCount == 0 {
		return types.Position{}, false, false
	}

	originalStartLine := startPos.Line
	originalStartCol := startPos.Col

	// Normalize start position just in case
	if originalStartLine < 0 {
		originalStartLine = 0
	}
	if originalStartLine >= lineCount {
		originalStartLine = lineCount - 1
	}
	// Note: Col clamping happens within loops

	if forward {
		// --- Phase 1: Search from startPos to end of buffer ---
		currentCol := originalStartCol // Use original col for first line search
		for lineIdx := originalStartLine; lineIdx < lineCount; lineIdx++ {
			lineBytes, err := buf.Line(lineIdx)
			if err != nil {
				continue
			}

			searchStartByteOffset := 0
			if lineIdx == originalStartLine {
				searchStartByteOffset = runeIndexToByteOffset(lineBytes, currentCol)
				if searchStartByteOffset < 0 {
					searchStartByteOffset = 0
				}
			}

			searchBytes := lineBytes[searchStartByteOffset:]
			loc := re.FindIndex(searchBytes)
			if loc != nil {
				matchByteOffset := searchStartByteOffset + loc[0]
				matchRuneCol := byteOffsetToRuneIndex(lineBytes, matchByteOffset)
				return types.Position{Line: lineIdx, Col: matchRuneCol}, true, false // Found, not wrapped
			}
			currentCol = 0 // Reset start col for subsequent lines
		}

		// --- Phase 2: Wrap around - Search from start of buffer to original startPos ---
		logger.DebugTagf("find", "Wrapping forward search to beginning.")
		for lineIdx := 0; lineIdx <= originalStartLine; lineIdx++ { // Include original line
			lineBytes, err := buf.Line(lineIdx)
			if err != nil {
				continue
			}

			searchBytes := lineBytes
			searchEndByteOffset := len(lineBytes) // Default: search whole line

			if lineIdx == originalStartLine {
				// On the original line, only search *up to* the original start column
				searchEndByteOffset = runeIndexToByteOffset(lineBytes, originalStartCol)
				if searchEndByteOffset < 0 {
					searchEndByteOffset = 0
				} // Clamp
				searchBytes = lineBytes[:searchEndByteOffset]
			}

			loc := re.FindIndex(searchBytes) // Find *first* match on wrapped lines
			if loc != nil {
				matchByteOffset := loc[0]
				matchRuneCol := byteOffsetToRuneIndex(lineBytes, matchByteOffset)
				return types.Position{Line: lineIdx, Col: matchRuneCol}, true, true // Found, wrapped
			}
		}

	} else { // Backward search
		// --- Phase 1: Search from startPos to beginning of buffer ---
		currentCol := originalStartCol // Use original col for first line search
		for lineIdx := originalStartLine; lineIdx >= 0; lineIdx-- {
			lineBytes, err := buf.Line(lineIdx)
			if err != nil {
				continue
			}

			searchEndByteOffset := len(lineBytes) // Default: whole line
			if lineIdx == originalStartLine {
				searchEndByteOffset = runeIndexToByteOffset(lineBytes, currentCol)
				if searchEndByteOffset < 0 {
					searchEndByteOffset = 0
				} // Clamp
			}

			searchBytes := lineBytes[:searchEndByteOffset]
			locs := re.FindAllIndex(searchBytes, -1)
			if len(locs) > 0 {
				lastMatch := locs[len(locs)-1] // Get last match before cursor/end offset
				matchByteOffset := lastMatch[0]
				matchRuneCol := byteOffsetToRuneIndex(lineBytes, matchByteOffset)
				return types.Position{Line: lineIdx, Col: matchRuneCol}, true, false // Found, not wrapped
			}
			// No need to reset currentCol for backward line iteration
		}

		// --- Phase 2: Wrap around - Search from end of buffer down to original startPos ---
		logger.DebugTagf("find", "Wrapping backward search to end.")
		for lineIdx := lineCount - 1; lineIdx >= originalStartLine; lineIdx-- { // Include original line
			lineBytes, err := buf.Line(lineIdx)
			if err != nil {
				continue
			}

			searchBytes := lineBytes
			searchStartByteOffset := 0 // Default: Start from beginning

			if lineIdx == originalStartLine {
				// On the original line, only search *from or after* the original start column
				searchStartByteOffset = runeIndexToByteOffset(lineBytes, originalStartCol)
				if searchStartByteOffset < 0 {
					searchStartByteOffset = 0
				}
				searchBytes = lineBytes[searchStartByteOffset:]
			}

			// Find matches on wrapped lines
			var locs [][]int
			if lineIdx == originalStartLine {
				// Find first match after start offset
				loc := re.FindIndex(searchBytes)
				if loc != nil {
					locs = [][]int{{searchStartByteOffset + loc[0], searchStartByteOffset + loc[1]}} // Adjust offset back
				}
			} else {
				// Find all matches on the line (when wrapping from end)
				locs = re.FindAllIndex(searchBytes, -1)
			}

			if len(locs) > 0 {
				// Need the *last* match found during the wrap search from the end
				// If on start line, take first. Otherwise take last.
				var matchToUse []int
				if lineIdx == originalStartLine {
					matchToUse = locs[0]
				} else {
					matchToUse = locs[len(locs)-1]
				}

				matchByteOffset := matchToUse[0]
				matchRuneCol := byteOffsetToRuneIndex(lineBytes, matchByteOffset)
				return types.Position{Line: lineIdx, Col: matchRuneCol}, true, true // Found, wrapped
			}
		}
	}

	return types.Position{}, false, false // Not found, wrap status irrelevant
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
	logger.DebugTagf("core", "FindManager: Added %d search highlights for '%s'", len(m.searchHighlights), term)
	return nil
}

// ClearHighlights removes search highlight regions.
func (m *Manager) ClearHighlights() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if len(m.searchHighlights) > 0 {
		logger.DebugTagf("core", "FindManager: Clearing %d search highlights", len(m.searchHighlights))
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
// Supports global 'g' flag. Undo support is NOT implemented for this operation yet.
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
	histMgr := m.editor.GetHistoryManager() // Get history manager for recording

	originalLineBytes, err := buf.Line(lineIdx)
	if err != nil {
		return 0, fmt.Errorf("cannot get current line %d: %w", lineIdx, err)
	}

	matches := re.FindAllIndex(originalLineBytes, -1)
	if len(matches) == 0 {
		return 0, nil
	} // No matches

	replaceCount := 0
	var finalLine bytes.Buffer // Buffer to build the new line content
	lastIndex := 0             // Tracks end position of last match/start position

	// --- TODO: Add proper Undo support ---
	// For now, global replace isn't easily undoable as one step.
	// We could record a single large Delete+Insert, but figuring out
	// the intermediate EditInfo for highlighting is hard.
	// Let's skip recording Undo for global replace in v1.0.
	canRecordUndo := !global // Only record simple undo for non-global replace

	var firstMatchStartPos types.Position // Cursor position after first replace

	if global {
		// --- Global Replace: Rebuild the line ---
		for _, loc := range matches {
			matchStartByte := loc[0]
			matchEndByte := loc[1]

			// Append text before the current match
			finalLine.Write(originalLineBytes[lastIndex:matchStartByte])
			// Append the replacement text
			finalLine.Write([]byte(replacement))
			// Update lastIndex to point after the current match
			lastIndex = matchEndByte
			replaceCount++
		}
		// Append any remaining text after the last match
		finalLine.Write(originalLineBytes[lastIndex:])

	} else {
		// --- Single Replace (First Occurrence Only) ---
		loc := matches[0]
		matchStartByte := loc[0]
		matchEndByte := loc[1]

		// Build the new line
		finalLine.Write(originalLineBytes[:matchStartByte])
		finalLine.Write([]byte(replacement))
		finalLine.Write(originalLineBytes[matchEndByte:])
		replaceCount = 1

		// Calculate start/end positions for undo/cursor
		firstMatchStartPos = types.Position{Line: lineIdx, Col: byteOffsetToRuneIndex(originalLineBytes, matchStartByte)}
		// Position after replacement (tricky if replacement has different rune count)
		// Let's place cursor at start for now
	}

	newLineBytes := finalLine.Bytes()

	// --- Apply Change to Buffer (Delete original line, Insert new line) ---
	// Calculate start/end for deleting the whole original line
	originalStartPos := types.Position{Line: lineIdx, Col: 0}
	originalEndCol := utf8.RuneCount(originalLineBytes)
	originalEndPos := types.Position{Line: lineIdx, Col: originalEndCol}

	// 1. Delete original line content
	editInfoDel, errDel := buf.Delete(originalStartPos, originalEndPos)
	if errDel != nil {
		return replaceCount, fmt.Errorf("replace failed during line delete: %w", errDel)
	}

	// 2. Insert new line content
	editInfoIns, errIns := buf.Insert(originalStartPos, newLineBytes) // Insert at {line, 0}
	if errIns != nil {
		return replaceCount, fmt.Errorf("replace failed during line insert: %w", errIns)
	}

	// --- Record Undo (Only for non-global for now) ---
	if canRecordUndo && histMgr != nil {
		loc := matches[0]
		matchStartByte := loc[0]
		matchEndByte := loc[1]
		matchStartPos := types.Position{Line: lineIdx, Col: byteOffsetToRuneIndex(originalLineBytes, matchStartByte)}

		// Record Delete
		histMgr.RecordChange(history.Change{
			Type:          history.DeleteAction,
			Text:          originalLineBytes[matchStartByte:matchEndByte],                                             // Original matched text
			StartPosition: matchStartPos,                                                                              // Where the original match started
			EndPosition:   types.Position{Line: lineIdx, Col: byteOffsetToRuneIndex(originalLineBytes, matchEndByte)}, // Where original match ended
			CursorBefore:  cursor,                                                                                     // Cursor before the :s command
		})
		// Record Insert
		histMgr.RecordChange(history.Change{
			Type:          history.InsertAction,
			Text:          []byte(replacement),
			StartPosition: matchStartPos,                                                                               // Where insertion happens
			EndPosition:   types.Position{Line: lineIdx, Col: matchStartPos.Col + utf8.RuneCountInString(replacement)}, // End of inserted text
			CursorBefore:  matchStartPos,                                                                               // Cursor was at match start before this insert
		})
	}

	// --- Dispatch Combined Event for Highlighting ---
	netEditInfo := types.EditInfo{
		StartIndex:     editInfoDel.StartIndex, // 0 (start of line)
		StartPosition:  editInfoDel.StartPosition,
		OldEndIndex:    editInfoDel.OldEndIndex, // Original end of line (bytes)
		OldEndPosition: editInfoDel.OldEndPosition,
		NewEndIndex:    editInfoIns.NewEndIndex, // New end of line (bytes)
		NewEndPosition: editInfoIns.NewEndPosition,
	}
	if eventMgr != nil {
		eventMgr.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: netEditInfo})
	}

	// --- Update Cursor ---
	// Move cursor to the start of the first replacement, or keep original if global?
	// Let's move to start of first replacement for consistency.
	if replaceCount > 0 && !global { // Only move for single replace now
		m.editor.SetCursor(firstMatchStartPos)
		m.editor.ScrollToCursor()
	} else if replaceCount > 0 && global {
		// Keep cursor at original line, column 0 after global replace? Or end of line?
		// Let's keep it at the start of the line.
		m.editor.SetCursor(types.Position{Line: lineIdx, Col: 0})
		m.editor.ScrollToCursor()
	}

	logger.DebugTagf("find", "Replace: Replaced %d occurrence(s). Global: %v", replaceCount, global)
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
