package core

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/rivo/uniseg"
)

// YankSelection copies the currently selected text into the internal clipboard.
// Returns true if text was copied, false otherwise.
func (e *Editor) YankSelection() (bool, error) {
	start, end, ok := e.GetSelection()
	if !ok {
		// No selection active
		return false, nil // Not an error, just nothing to yank
	}

	// Extract the selected text from the buffer
	// This might involve iterating through lines if selection spans multiple lines.
	var content bytes.Buffer
	lineCount := e.buffer.LineCount()

	for lineIdx := start.Line; lineIdx <= end.Line; lineIdx++ {
		if lineIdx < 0 || lineIdx >= lineCount {
			continue // Skip invalid lines
		}

		lineBytes, err := e.buffer.Line(lineIdx)
		if err != nil {
			// Log error but try to continue? Or return error? Return error for now.
			return false, fmt.Errorf("error getting line %d for yank: %w", lineIdx, err)
		}
		lineStr := string(lineBytes) // Need string for rune indexing
		gr := uniseg.NewGraphemes(lineStr)

		currentRuneIndex := 0
		lineContentStartCol := -1 // Rune index where selection starts on this line
		lineContentEndCol := -1   // Rune index where selection ends on this line

		// Determine start/end columns for *this* line within the selection
		if lineIdx == start.Line {
			lineContentStartCol = start.Col
		} else {
			lineContentStartCol = 0
		} // Starts at beginning if not start line
		if lineIdx == end.Line {
			lineContentEndCol = end.Col
		} else {
			lineContentEndCol = -1
		} // Use -1 to signify "to end of line" if not end line

		startByteOffset := -1
		endByteOffset := -1
		currentByteOffset := 0

		// Find byte offsets corresponding to rune columns
		gr = uniseg.NewGraphemes(lineStr) // Reset iterator
		for gr.Next() {
			runes := gr.Runes()
			numRunes := len(runes)
			numBytes := len(gr.Bytes())

			if lineContentStartCol != -1 && currentRuneIndex == lineContentStartCol {
				startByteOffset = currentByteOffset
			}
			// Check end *before* incrementing index for exclusive end range
			if lineContentEndCol != -1 && currentRuneIndex == lineContentEndCol {
				endByteOffset = currentByteOffset
			}

			currentRuneIndex += numRunes
			currentByteOffset += numBytes
		}

		// Adjust offsets if columns were at the boundaries
		if lineContentStartCol != -1 && startByteOffset == -1 { // If startCol was >= runeCount
			startByteOffset = len(lineBytes)
		}
		if lineContentEndCol != -1 && endByteOffset == -1 { // If endCol was >= runeCount
			endByteOffset = len(lineBytes)
		}

		// Extract the relevant part of the line
		var linePart []byte
		if startByteOffset != -1 { // Selection starts on this line
			if endByteOffset != -1 { // Selection also ends on this line
				if endByteOffset > startByteOffset {
					linePart = lineBytes[startByteOffset:endByteOffset]
				}
			} else { // Selection continues to next line
				linePart = lineBytes[startByteOffset:]
			}
		} else { // Selection started on a previous line
			if endByteOffset != -1 { // Selection ends on this line
				linePart = lineBytes[:endByteOffset]
			} else { // Selection covers the whole line (started before, ends after)
				linePart = lineBytes[:]
			}
		}

		content.Write(linePart)
		// Add newline if this wasn't the last line of the selection
		if lineIdx < end.Line {
			content.WriteByte('\n')
		}
	}

	e.clipboard = content.Bytes()
	logger.Debugf("Editor: Yanked %d bytes", len(e.clipboard)) // Using logger instead of log

	// Optionally clear selection after yank? Common behavior in some editors.
	e.ClearSelection()

	return true, nil
}

// Paste inserts the content of the internal clipboard at the cursor position.
// Returns true if text was pasted, false otherwise.
func (e *Editor) Paste() (bool, error) {
	if e.clipboard == nil || len(e.clipboard) == 0 {
		// Nothing in clipboard
		return false, nil // Not an error, just nothing to paste
	}

	// If there's a selection, delete it first (common behavior)
	if e.HasSelection() {
		start, end, _ := e.GetSelection()
		e.ClearSelection()
		editInfo, err := e.buffer.Delete(start, end) // Capture EditInfo
		if err != nil {
			return false, fmt.Errorf("failed to delete selection before paste: %w", err)
		}
		e.Cursor = start // Move cursor to where selection started
		if e.eventManager != nil {
			e.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
		}
	}

	pastePos := e.Cursor // Paste at current cursor position
	clipboardContent := e.clipboard

	// Use buffer's Insert method
	editInfo, err := e.buffer.Insert(pastePos, clipboardContent) // Capture EditInfo
	if err != nil {
		return false, fmt.Errorf("buffer insert failed during paste: %w", err)
	}

	// --- Update Cursor Position ---
	// Calculate new cursor position based on pasted content
	// Need to count lines and runes in the pasted text
	numLines := bytes.Count(clipboardContent, []byte("\n"))
	lastLine := clipboardContent
	if numLines > 0 {
		lastNewLineIndex := bytes.LastIndexByte(clipboardContent, '\n')
		lastLine = clipboardContent[lastNewLineIndex+1:]
	}
	lastLineRuneCount := utf8.RuneCount(lastLine)

	// Move cursor to the end of the pasted content
	e.Cursor.Line = pastePos.Line + numLines
	if numLines > 0 {
		e.Cursor.Col = lastLineRuneCount // Start of last pasted line + its length
	} else {
		e.Cursor.Col = pastePos.Col + lastLineRuneCount // Same line, advance column
	}

	// Ensure cursor is valid and visible
	e.MoveCursor(0, 0) // Use MoveCursor to clamp and scroll

	logger.Debugf("Editor: Pasted %d bytes", len(clipboardContent)) // Using logger instead of log
	if e.eventManager != nil {
		e.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo}) // Dispatch modify event with EditInfo
	}

	return true, nil
}
