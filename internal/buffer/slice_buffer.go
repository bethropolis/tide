// internal/buffer/slice_buffer.go
package buffer

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/types" // Import types instead of core
)

// SliceBuffer implementation (content mostly unchanged, just imports and method signatures)
type SliceBuffer struct {
	lines    [][]byte
	filePath string
	modified bool // Track if buffer has unsaved changes
}

// NewSliceBuffer creates an empty SliceBuffer.
func NewSliceBuffer() *SliceBuffer {
	return &SliceBuffer{
		// Start with a single empty line, common for new files
		lines:    [][]byte{[]byte("")},
		modified: false, // Initially not modified
	}
}

// Load reads a file into the buffer. Replaces existing content.
func (sb *SliceBuffer) Load(filePath string) error {
	// Reset modified status on load
	sb.modified = false

	file, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			sb.lines = [][]byte{[]byte("")}
			sb.filePath = filePath
			sb.modified = false // New buffer isn't modified yet
			return nil
		}
		return fmt.Errorf("failed to open file '%s': %w", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	newLines := [][]byte{}
	for scanner.Scan() {
		line := scanner.Bytes()
		lineCopy := make([]byte, len(line))
		copy(lineCopy, line)
		newLines = append(newLines, lineCopy)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file '%s': %w", filePath, err)
	}
	if len(newLines) == 0 {
		newLines = append(newLines, []byte(""))
	}
	sb.lines = newLines
	sb.filePath = filePath
	return nil
}



// Lines implementation (no changes)
func (sb *SliceBuffer) Lines() [][]byte {
	return sb.lines
}

// LineCount implementation (no changes)
func (sb *SliceBuffer) LineCount() int {
	return len(sb.lines)
}

// Line implementation (no changes)
func (sb *SliceBuffer) Line(index int) ([]byte, error) {
	if index < 0 || index >= len(sb.lines) {
		return nil, fmt.Errorf("line index %d out of bounds (0-%d)", index, len(sb.lines)-1)
	}
	return sb.lines[index], nil
}

// Bytes implementation (no changes)
func (sb *SliceBuffer) Bytes() []byte {
	var buffer bytes.Buffer
	for i, line := range sb.lines {
		buffer.Write(line)
		if i < len(sb.lines)-1 {
			buffer.WriteByte('\n')
		}
	}
	return buffer.Bytes()
}

// Save writes the buffer content to the stored filePath.
func (sb *SliceBuffer) Save(filePath string) error {
	path := sb.filePath
	if filePath != "" { // Allow overriding path during save
		path = filePath
	}
	if path == "" {
		return errors.New("no file path specified for saving")
	}

	content := sb.Bytes()
	err := os.WriteFile(path, content, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file '%s': %w", path, err)
	}

	// Update internal path if saved to a new location
	sb.filePath = path
	// Reset modified status after successful save
	sb.modified = false
	return nil
}

// IsModified returns true if the buffer has unsaved changes.
func (sb *SliceBuffer) IsModified() bool {
	return sb.modified
}

func (sb *SliceBuffer) FilePath() string {
	return sb.filePath
}


// --- Buffer Modification Methods ---

// validateAndGetByteOffsets validates start and end positions and returns their byte offsets.
// It ensures start <= end.
func (sb *SliceBuffer) validateAndGetByteOffsets(start, end types.Position) (vStart types.Position, vEnd types.Position, startOffset int, endOffset int, err error) {
	// Ensure start <= end (lexicographically)
	if start.Line > end.Line || (start.Line == end.Line && start.Col > end.Col) {
		start, end = end, start // Swap if start is after end
	}

	var startErr, endErr error
	vStart, startOffset, startErr = sb.validatePosition(start) // Use existing validation
	vEnd, endOffset, endErr = sb.validatePosition(end)

	if startErr != nil || endErr != nil {
		// Consolidate potential errors (though validatePosition doesn't currently return them)
		return vStart, vEnd, 0, 0, fmt.Errorf("invalid range: startErr=%v, endErr=%v", startErr, endErr)
	}

	// Ensure byte offsets are consistent if positions were clamped differently
	// (e.g., if both point past end of same line, byte offsets should be same)
	if vStart.Line == vEnd.Line {
		// Re-calculate endOffset based on validated start line and original end column
		// to handle clamping correctly within the same line.
		_, endOffset, _ = sb.validatePositionOnLine(vEnd.Col, vStart.Line)
		// Ensure startOffset <= endOffset after validation/clamping
		if startOffset > endOffset {
			startOffset = endOffset
		}
	}


	return vStart, vEnd, startOffset, endOffset, nil
}

// validatePositionOnLine is a helper to get byte offset for a column on a specific line.
func (sb *SliceBuffer) validatePositionOnLine(col int, lineIndex int) (validCol int, byteOffset int, err error) {
    if lineIndex < 0 || lineIndex >= len(sb.lines) {
        return 0, 0, fmt.Errorf("line index %d out of bounds", lineIndex)
    }
    currentLine := sb.lines[lineIndex]
    byteOff := 0
    runeCount := 0
    for i := 0; i < len(currentLine); {
        if runeCount == col {
            break
        }
        _, size := utf8.DecodeRune(currentLine[i:])
        byteOff += size
        runeCount++
        i += size
    }
    if runeCount < col {
        col = runeCount
        byteOff = len(currentLine)
    }
    return col, byteOff, nil
}


// Insert inserts text at a given position. Handles single/multiple lines.
func (sb *SliceBuffer) Insert(pos types.Position, text []byte) error {
	if len(text) == 0 {
		return nil
	}

	validPos, byteOffset, err := sb.validatePosition(pos)
	if err != nil {
		return fmt.Errorf("invalid insert position: %w", err)
	}

	// Mark buffer as modified
	sb.modified = true // Set modified flag

	currentLine := sb.lines[validPos.Line]
	insertLines := bytes.Split(text, []byte("\n"))

	tail := make([]byte, len(currentLine[byteOffset:]))
	copy(tail, currentLine[byteOffset:])

	sb.lines[validPos.Line] = append(currentLine[:byteOffset], insertLines[0]...)

	if len(insertLines) > 1 {
		newLines := make([][]byte, len(insertLines)-1)
		for i := 1; i < len(insertLines); i++ {
			lineCopy := make([]byte, len(insertLines[i]))
			copy(lineCopy, insertLines[i])
			newLines[i-1] = lineCopy
		}
		newLines[len(newLines)-1] = append(newLines[len(newLines)-1], tail...)
		// Insert the new lines
        // Ensure we handle insertion at the very end correctly
        if validPos.Line+1 > len(sb.lines) {
             sb.lines = append(sb.lines, newLines...)
        } else {
		    sb.lines = append(sb.lines[:validPos.Line+1], append(newLines, sb.lines[validPos.Line+1:]...)...)
        }

	} else {
		sb.lines[validPos.Line] = append(sb.lines[validPos.Line], tail...)
	}

	return nil
}


// Delete removes text within a given range (start inclusive, end exclusive).
func (sb *SliceBuffer) Delete(start, end types.Position) error {
	if start == end {
		return nil // Nothing to delete
	}

	// Validate positions and get byte offsets
	vStart, vEnd, startOffset, endOffset, err := sb.validateAndGetByteOffsets(start, end)
	if err != nil {
		return fmt.Errorf("invalid delete range: %w", err)
	}

	// If validation resulted in start == end after clamping, do nothing
	if vStart == vEnd && startOffset == endOffset {
		return nil
	}

	// Mark buffer as modified
	sb.modified = true

	startLineBytes := sb.lines[vStart.Line]

	if vStart.Line == vEnd.Line {
		// --- Case 1: Deletion within a single line ---
		// Ensure endOffset is not out of bounds after potential clamping
		if endOffset > len(startLineBytes) {
			endOffset = len(startLineBytes)
		}
		// Ensure startOffset is valid
		if startOffset > len(startLineBytes) {
		    startOffset = len(startLineBytes)
		}
        // Ensure start <= end after clamping
        if startOffset > endOffset {
            startOffset = endOffset
        }

		// Reconstruct the line by combining the part before start and the part after end
		sb.lines[vStart.Line] = append(startLineBytes[:startOffset], startLineBytes[endOffset:]...)

	} else {
		// --- Case 2: Deletion spans multiple lines ---
		endLineBytes := sb.lines[vEnd.Line]

		// Keep the beginning of the start line and the end of the end line
		startPart := startLineBytes[:startOffset]
		endPart := endLineBytes[endOffset:]

		// Merge the start part and end part onto the start line
		sb.lines[vStart.Line] = append(startPart, endPart...)

		// Remove the lines between start and end (exclusive start, inclusive end)
		// Calculate the indices of lines to remove
		firstLineToRemove := vStart.Line + 1
		lastLineToRemove := vEnd.Line

		// Ensure indices are valid before slicing
		if firstLineToRemove <= lastLineToRemove && lastLineToRemove < len(sb.lines) {
            // Check if we are deleting up to the *last* line
            if lastLineToRemove + 1 >= len(sb.lines) {
                // If deleting includes the last line, just truncate the slice
                sb.lines = sb.lines[:firstLineToRemove]
            } else {
                // Otherwise, append the lines *after* the deleted range
			    sb.lines = append(sb.lines[:firstLineToRemove], sb.lines[lastLineToRemove+1:]...)
            }
		} else if firstLineToRemove > lastLineToRemove {
            // This means vStart.Line + 1 > vEnd.Line, only possible if vEnd.Line == vStart.Line
            // This case is handled above (single line deletion). Should not happen here.
            log.Printf("Warning: Unexpected state in multi-line delete: startLine=%d, endLine=%d", vStart.Line, vEnd.Line)
        }
        // If lastLineToRemove is out of bounds, it means we deleted *up to* the end, handled by the first condition.
	}

    // Ensure buffer always has at least one line (convention)
    if len(sb.lines) == 0 {
        sb.lines = [][]byte{[]byte("")}
        // If buffer became empty, it's arguably not modified from a 'new file' state
        // However, if it previously had content, it *is* modified. Keep sb.modified = true.
    }

	return nil
}

// validatePosition (keep the implementation from the previous step)
func (sb *SliceBuffer) validatePosition(pos types.Position) (validPos types.Position, byteOffset int, err error) {
	if pos.Line < 0 {
		pos.Line = 0
	}
	// Clamp line index
	if pos.Line >= len(sb.lines) {
		pos.Line = len(sb.lines) - 1
		if pos.Line < 0 { // Buffer was empty?
			sb.lines = append(sb.lines, []byte("")) // Ensure at least one line
			pos.Line = 0
		}
	}

    validLine := pos.Line // Use the potentially clamped line index
    var validCol int
    validCol, byteOffset, err = sb.validatePositionOnLine(pos.Col, validLine)
    if err != nil {
        // Should not happen if line index is clamped correctly
        return types.Position{}, 0, err
    }

	return types.Position{Line: validLine, Col: validCol}, byteOffset, nil
}


// Ensure SliceBuffer satisfies the Buffer interface
var _ Buffer = (*SliceBuffer)(nil)