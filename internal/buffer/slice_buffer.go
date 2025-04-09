// internal/buffer/slice_buffer.go
package buffer

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types" // Import types instead of core
	sitter "github.com/smacker/go-tree-sitter"   // Import tree-sitter for Point
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

// Save writes the buffer content. Uses provided filePath if not empty, otherwise internal path.
// Updates internal filePath on successful save to a new location.
func (sb *SliceBuffer) Save(filePath string) error {
	savePath := filePath
	if savePath == "" { // If no path provided, use internal path
		savePath = sb.filePath
	}

	if savePath == "" {
		return errors.New("no file path specified for saving")
	}

	content := sb.Bytes()
	// Use TempFile and Rename for atomic save (safer)
	tempFile, err := os.CreateTemp(filepath.Dir(savePath), "."+filepath.Base(savePath)+".tmp")
	if err != nil {
		// Fallback if temp file creation fails (e.g., permissions)
		logger.Warnf("Could not create temp file for saving, attempting direct write: %v", err)
		err = os.WriteFile(savePath, content, 0644)
		if err != nil {
			return fmt.Errorf("failed to write file '%s': %w", savePath, err)
		}
	} else {
		tempPath := tempFile.Name()
		logger.Debugf("Saving to temp file: %s", tempPath)
		_, err = tempFile.Write(content)
		// Close file before renaming
		closeErr := tempFile.Close()
		if err != nil {
			os.Remove(tempPath) // Clean up temp file on write error
			return fmt.Errorf("failed to write to temporary file '%s': %w", tempPath, err)
		}
		if closeErr != nil {
			os.Remove(tempPath) // Clean up temp file on close error
			return fmt.Errorf("failed to close temporary file '%s': %w", tempPath, closeErr)
		}

		// Rename temporary file to the final path
		err = os.Rename(tempPath, savePath)
		if err != nil {
			os.Remove(tempPath) // Clean up temp file on rename error
			return fmt.Errorf("failed to rename temporary file to '%s': %w", savePath, err)
		}
		logger.Debugf("Renamed temp file %s to %s", tempPath, savePath)
	}

	// --- Update internal state ONLY after successful save ---
	sb.filePath = savePath // Update buffer's path to the saved path
	sb.modified = false    // Reset modified status
	logger.Infof("Buffer saved successfully to %s", savePath)
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

// getBufferStateForEdit calculates byte offsets and sitter.Point for a given types.Position
func (sb *SliceBuffer) getBufferStateForEdit(pos types.Position) (byteOffset uint32, point sitter.Point) {
	byteOff := uint32(0)
	// Calculate total bytes up to the start of the target line
	for i := 0; i < pos.Line; i++ {
		if i < len(sb.lines) {
			byteOff += uint32(len(sb.lines[i])) + 1 // +1 for the newline character
		}
	}

	// Calculate byte offset within the target line
	colByteOffset := uint32(0)
	if pos.Line >= 0 && pos.Line < len(sb.lines) {
		currentLine := sb.lines[pos.Line]
		byteOffInLine := 0
		runeCount := 0
		for i := 0; i < len(currentLine); {
			if runeCount == pos.Col {
				break
			}
			_, size := utf8.DecodeRune(currentLine[i:])
			if size == 0 {
				break // Avoid infinite loop
			}
			byteOffInLine += size
			runeCount++
			i += size
		}
		// Handle case where col is at the end of the line
		if runeCount < pos.Col {
			byteOffInLine = len(currentLine)
		}
		colByteOffset = uint32(byteOffInLine)
		byteOff += colByteOffset
	}

	point = sitter.Point{
		Row:    uint32(pos.Line),
		Column: colByteOffset, // Point column is BYTES within the line
	}
	return byteOff, point
}

// Insert inserts text at a given position. Handles single/multiple lines.
func (sb *SliceBuffer) Insert(pos types.Position, text []byte) (types.EditInfo, error) {
	editInfo := types.EditInfo{}
	if len(text) == 0 {
		return editInfo, nil // No change, no edit info
	}

	// 1. Get state *before* the edit
	startIndex, startPoint := sb.getBufferStateForEdit(pos)
	editInfo.StartIndex = startIndex
	editInfo.StartPosition = startPoint
	editInfo.OldEndIndex = startIndex // For insert, old range is zero length
	editInfo.OldEndPosition = startPoint

	// Perform the actual buffer modification
	validPos, byteOffset, err := sb.validatePosition(pos)
	if err != nil {
		return editInfo, fmt.Errorf("invalid insert position: %w", err)
	}

	// Mark buffer as modified
	sb.modified = true

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

		if validPos.Line+1 > len(sb.lines) {
			sb.lines = append(sb.lines, newLines...)
		} else {
			sb.lines = append(sb.lines[:validPos.Line+1], append(newLines, sb.lines[validPos.Line+1:]...)...)
		}
	} else {
		sb.lines[validPos.Line] = append(sb.lines[validPos.Line], tail...)
	}

	// 3. Calculate state *after* the edit
	textLen := uint32(len(text))
	editInfo.NewEndIndex = startIndex + textLen

	// Calculate NewEndPosition based on inserted text content
	numLinesInserted := bytes.Count(text, []byte("\n"))
	newEndLine := startPoint.Row + uint32(numLinesInserted)
	var newEndCol uint32

	if numLinesInserted == 0 {
		// Inserted on the same line
		newEndCol = startPoint.Column + uint32(len(text)) // Column is bytes for sitter.Point
	} else {
		// Inserted multiple lines
		lastNewLineIndex := bytes.LastIndexByte(text, '\n')
		lastLineBytes := text[lastNewLineIndex+1:]
		newEndCol = uint32(len(lastLineBytes)) // Bytes on the last inserted line
	}

	editInfo.NewEndPosition = sitter.Point{Row: newEndLine, Column: newEndCol}

	return editInfo, nil
}

// Delete removes text within a given range (start inclusive, end exclusive).
func (sb *SliceBuffer) Delete(start, end types.Position) (types.EditInfo, error) {
	editInfo := types.EditInfo{}
	if start == end {
		return editInfo, nil // Nothing to delete
	}

	// 1. Get state *before* the edit
	startIndexBytes, startPoint := sb.getBufferStateForEdit(start)
	endIndexBytes, endPoint := sb.getBufferStateForEdit(end)

	editInfo.StartIndex = startIndexBytes
	editInfo.StartPosition = startPoint
	editInfo.OldEndIndex = endIndexBytes // End of the text being removed
	editInfo.OldEndPosition = endPoint
	editInfo.NewEndIndex = startIndexBytes // After delete, the new end is where the start was
	editInfo.NewEndPosition = startPoint

	// Validate positions and get byte offsets
	vStart, vEnd, startOffset, endOffset, err := sb.validateAndGetByteOffsets(start, end)
	if err != nil {
		return editInfo, fmt.Errorf("invalid delete range: %w", err)
	}

	// If validation resulted in start == end after clamping, do nothing
	if vStart == vEnd && startOffset == endOffset {
		return editInfo, nil
	}

	// Mark buffer as modified
	sb.modified = true

	startLineBytes := sb.lines[vStart.Line]

	if vStart.Line == vEnd.Line {
		// --- Case 1: Deletion within a single line ---
		if endOffset > len(startLineBytes) {
			endOffset = len(startLineBytes)
		}
		if startOffset > len(startLineBytes) {
			startOffset = len(startLineBytes)
		}
		if startOffset > endOffset {
			startOffset = endOffset
		}

		sb.lines[vStart.Line] = append(startLineBytes[:startOffset], startLineBytes[endOffset:]...)
	} else {
		// --- Case 2: Deletion spans multiple lines ---
		endLineBytes := sb.lines[vEnd.Line]

		startPart := startLineBytes[:startOffset]
		endPart := endLineBytes[endOffset:]

		sb.lines[vStart.Line] = append(startPart, endPart...)

		firstLineToRemove := vStart.Line + 1
		lastLineToRemove := vEnd.Line

		if firstLineToRemove <= lastLineToRemove && lastLineToRemove < len(sb.lines) {
			if lastLineToRemove+1 >= len(sb.lines) {
				sb.lines = sb.lines[:firstLineToRemove]
			} else {
				sb.lines = append(sb.lines[:firstLineToRemove], sb.lines[lastLineToRemove+1:]...)
			}
		}
	}

	// Ensure buffer always has at least one line
	if len(sb.lines) == 0 {
		sb.lines = [][]byte{[]byte("")}
	}

	return editInfo, nil
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
