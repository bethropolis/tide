package buffer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// PieceTable is a more efficient buffer implementation for large files
type PieceTable struct {
	original []byte
	add      []byte
	pieces   []piece

	filePath string
	modified bool

	// Cached flat byte content and line-start offsets.
	// lineOffsetsDirty is set to true after every mutation; the caches are
	// rebuilt lazily by ensureCache() on the next read.
	cachedBytes []byte
	lineOffsets []int // lineOffsets[i] = byte offset of line i in cachedBytes
	cacheValid  bool
}

type piece struct {
	buffer source
	start  int
	length int
}

type source int

const (
	originalBuffer source = iota
	addBuffer
)

func NewPieceTable() *PieceTable {
	return &PieceTable{
		original: []byte{},
		add:      []byte{},
		pieces:   []piece{{buffer: originalBuffer, start: 0, length: 0}},
	}
}

// Implement the Buffer interface methods...
func (pt *PieceTable) Load(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			pt.filePath = filePath
			return err
		}
		return err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	pt.original = content
	pt.add = []byte{}
	pt.pieces = []piece{{buffer: originalBuffer, start: 0, length: len(content)}}
	pt.filePath = filePath
	pt.modified = false
	pt.invalidateCache()
	return nil
}

// Convert absolute position to piece
func (pt *PieceTable) findPiece(offset int) (int, int) {
	currentOffset := 0
	for i, p := range pt.pieces {
		if currentOffset+p.length > offset {
			return i, offset - currentOffset
		}
		currentOffset += p.length
	}
	if len(pt.pieces) == 0 {
		return 0, 0
	}
	return len(pt.pieces) - 1, pt.pieces[len(pt.pieces)-1].length
}

func (pt *PieceTable) GetText(start, end types.Position) string {
	startOff := pt.positionToOffset(start)
	endOff := pt.positionToOffset(end)

	if startOff > endOff {
		startOff, endOff = endOff, startOff
	}

	if startOff == endOff {
		return ""
	}

	bytes := pt.Bytes()
	if startOff < 0 {
		startOff = 0
	}
	if endOff > len(bytes) {
		endOff = len(bytes)
	}

	return string(bytes[startOff:endOff])
}

// ensureCache rebuilds cachedBytes and lineOffsets if the cache is invalid.
// lineOffsets[i] is the byte offset of the start of line i in cachedBytes.
// The final entry lineOffsets[n] points one past the last byte (== len(cachedBytes))
// so that the length of line i is lineOffsets[i+1] - lineOffsets[i] - 1 (excluding '\n').
func (pt *PieceTable) ensureCache() {
	if pt.cacheValid {
		return
	}

	// Rebuild flat byte slice
	var result bytes.Buffer
	for _, p := range pt.pieces {
		if p.buffer == originalBuffer {
			if p.start+p.length <= len(pt.original) {
				result.Write(pt.original[p.start : p.start+p.length])
			}
		} else {
			if p.start+p.length <= len(pt.add) {
				result.Write(pt.add[p.start : p.start+p.length])
			}
		}
	}
	pt.cachedBytes = result.Bytes()

	// Build line-start offset table
	offsets := make([]int, 0, 64)
	offsets = append(offsets, 0)
	for i, b := range pt.cachedBytes {
		if b == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	pt.lineOffsets = offsets
	pt.cacheValid = true
}

// invalidateCache marks the cache as stale. Call after every mutation.
func (pt *PieceTable) invalidateCache() {
	pt.cacheValid = false
	pt.cachedBytes = nil
	pt.lineOffsets = nil
}

func (pt *PieceTable) Bytes() []byte {
	pt.ensureCache()
	// Return a copy to prevent callers from mutating our cache.
	out := make([]byte, len(pt.cachedBytes))
	copy(out, pt.cachedBytes)
	return out
}

func (pt *PieceTable) Lines() [][]byte {
	pt.ensureCache()
	return bytes.Split(pt.cachedBytes, []byte{'\n'})
}

func (pt *PieceTable) Line(index int) ([]byte, error) {
	pt.ensureCache()
	offsets := pt.lineOffsets
	lineCount := len(offsets)
	if index < 0 || index >= lineCount {
		return nil, fmt.Errorf("line index out of bounds")
	}
	start := offsets[index]
	var end int
	if index+1 < len(offsets) {
		end = offsets[index+1] - 1 // exclude the '\n'
	} else {
		end = len(pt.cachedBytes)
	}
	if end < start {
		end = start
	}
	// Return a copy so callers cannot corrupt the cache.
	out := make([]byte, end-start)
	copy(out, pt.cachedBytes[start:end])
	return out, nil
}

func (pt *PieceTable) LineCount() int {
	pt.ensureCache()
	return len(pt.lineOffsets)
}

// Calculate absolute byte offset from a Position.
// pos.Col is treated as a rune index.
func (pt *PieceTable) positionToOffset(pos types.Position) int {
	pt.ensureCache()
	offsets := pt.lineOffsets
	if pos.Line >= len(offsets) {
		return len(pt.cachedBytes)
	}

	lineStart := offsets[pos.Line]
	var lineEnd int
	if pos.Line+1 < len(offsets) {
		lineEnd = offsets[pos.Line+1] - 1 // exclude '\n'
	} else {
		lineEnd = len(pt.cachedBytes)
	}
	lineBytes := pt.cachedBytes[lineStart:lineEnd]

	byteOffInLine := 0
	runeCount := 0
	for i := 0; i < len(lineBytes); {
		if runeCount == pos.Col {
			break
		}
		_, size := utf8.DecodeRune(lineBytes[i:])
		if size == 0 {
			break
		}
		byteOffInLine += size
		runeCount++
		i += size
	}
	if runeCount < pos.Col {
		byteOffInLine = len(lineBytes)
	}

	return lineStart + byteOffInLine
}

// getBufferStateForEdit calculates byte offsets and sitter.Point for a given types.Position.
// pos.Col is treated as a rune index and converted to byte offset for tree-sitter.
func (pt *PieceTable) getBufferStateForEdit(pos types.Position) (byteOffset uint32, point sitter.Point) {
	pt.ensureCache()
	offsets := pt.lineOffsets

	var lineStart int
	if pos.Line < len(offsets) {
		lineStart = offsets[pos.Line]
	} else {
		lineStart = len(pt.cachedBytes)
	}

	// Calculate byte offset within the target line (converting rune col to bytes)
	colByteOffset := uint32(0)
	if pos.Line >= 0 && pos.Line < len(offsets) {
		var lineEnd int
		if pos.Line+1 < len(offsets) {
			lineEnd = offsets[pos.Line+1] - 1 // exclude '\n'
		} else {
			lineEnd = len(pt.cachedBytes)
		}
		currentLine := pt.cachedBytes[lineStart:lineEnd]
		byteOffInLine := 0
		runeCount := 0
		for i := 0; i < len(currentLine); {
			if runeCount == pos.Col {
				break
			}
			_, size := utf8.DecodeRune(currentLine[i:])
			if size == 0 {
				break
			}
			byteOffInLine += size
			runeCount++
			i += size
		}
		if runeCount < pos.Col {
			byteOffInLine = len(currentLine)
		}
		colByteOffset = uint32(byteOffInLine)
	}

	byteOff := uint32(lineStart) + colByteOffset
	point = sitter.Point{
		Row:    uint32(pos.Line),
		Column: colByteOffset, // Point column is BYTES within the line
	}
	return byteOff, point
}

func (pt *PieceTable) Insert(pos types.Position, text []byte) (types.EditInfo, error) {
	editInfo := types.EditInfo{}
	if len(text) == 0 {
		return editInfo, nil
	}

	// 1. Get state *before* the edit
	startIndex, startPoint := pt.getBufferStateForEdit(pos)
	editInfo.StartIndex = startIndex
	editInfo.StartPosition = startPoint
	editInfo.OldEndIndex = startIndex // For insert, old range is zero length
	editInfo.OldEndPosition = startPoint

	offset := pt.positionToOffset(pos)

	// Fast path for empty pieces
	if len(pt.pieces) == 1 && pt.pieces[0].length == 0 {
		pt.add = append(pt.add, text...)
		pt.pieces[0] = piece{buffer: addBuffer, start: 0, length: len(text)}
		pt.modified = true
		pt.invalidateCache()
		textLen := uint32(len(text))
		editInfo.NewEndIndex = startIndex + textLen
		numLinesInserted := bytes.Count(text, []byte("\n"))
		newEndLine := startPoint.Row + uint32(numLinesInserted)
		var newEndCol uint32
		if numLinesInserted == 0 {
			newEndCol = startPoint.Column + textLen
		} else {
			lastNewLineIndex := bytes.LastIndexByte(text, '\n')
			lastLineBytes := text[lastNewLineIndex+1:]
			newEndCol = uint32(len(lastLineBytes))
		}
		editInfo.NewEndPosition = sitter.Point{Row: newEndLine, Column: newEndCol}

		return editInfo, nil
	}

	addStart := len(pt.add)
	pt.add = append(pt.add, text...)
	newPiece := piece{buffer: addBuffer, start: addStart, length: len(text)}

	pieceIdx, pieceOffset := pt.findPiece(offset)

	var newPieces []piece

	if pieceOffset == 0 {
		newPieces = make([]piece, 0, len(pt.pieces)+1)
		newPieces = append(newPieces, pt.pieces[:pieceIdx]...)
		newPieces = append(newPieces, newPiece)
		newPieces = append(newPieces, pt.pieces[pieceIdx:]...)
	} else if pieceOffset == pt.pieces[pieceIdx].length {
		newPieces = make([]piece, 0, len(pt.pieces)+1)
		newPieces = append(newPieces, pt.pieces[:pieceIdx+1]...)
		newPieces = append(newPieces, newPiece)
		if pieceIdx+1 < len(pt.pieces) {
			newPieces = append(newPieces, pt.pieces[pieceIdx+1:]...)
		}
	} else {
		oldPiece := pt.pieces[pieceIdx]
		leftPiece := piece{buffer: oldPiece.buffer, start: oldPiece.start, length: pieceOffset}
		rightPiece := piece{buffer: oldPiece.buffer, start: oldPiece.start + pieceOffset, length: oldPiece.length - pieceOffset}

		newPieces = make([]piece, 0, len(pt.pieces)+2)
		newPieces = append(newPieces, pt.pieces[:pieceIdx]...)
		newPieces = append(newPieces, leftPiece, newPiece, rightPiece)
		if pieceIdx+1 < len(pt.pieces) {
			newPieces = append(newPieces, pt.pieces[pieceIdx+1:]...)
		}
	}

	pt.pieces = newPieces
	pt.modified = true
	pt.invalidateCache()

	// Calculate NewEndPosition
	textLen := uint32(len(text))
	editInfo.NewEndIndex = startIndex + textLen
	numLinesInserted := bytes.Count(text, []byte("\n"))
	newEndLine := startPoint.Row + uint32(numLinesInserted)
	var newEndCol uint32
	if numLinesInserted == 0 {
		newEndCol = startPoint.Column + textLen
	} else {
		lastNewLineIndex := bytes.LastIndexByte(text, '\n')
		lastLineBytes := text[lastNewLineIndex+1:]
		newEndCol = uint32(len(lastLineBytes))
	}
	editInfo.NewEndPosition = sitter.Point{Row: newEndLine, Column: newEndCol}

	return editInfo, nil
}

func (pt *PieceTable) Delete(start, end types.Position) (types.EditInfo, error) {
	editInfo := types.EditInfo{}

	// Get state *before* the edit
	startIndexBytes, startPoint := pt.getBufferStateForEdit(start)
	endIndexBytes, endPoint := pt.getBufferStateForEdit(end)

	editInfo.StartIndex = startIndexBytes
	editInfo.StartPosition = startPoint
	editInfo.OldEndIndex = endIndexBytes
	editInfo.OldEndPosition = endPoint
	editInfo.NewEndIndex = startIndexBytes // After delete, new end is where start was
	editInfo.NewEndPosition = startPoint

	startOff := pt.positionToOffset(start)
	endOff := pt.positionToOffset(end)

	if startOff >= endOff {
		return editInfo, nil
	}

	// A more optimal approach using piece manipulation

	// Use findPiece to locate starting and ending pieces
	startPieceIdx, startPieceOffset := pt.findPiece(startOff)
	endPieceIdx, endPieceOffset := pt.findPiece(endOff)

	var newPieces []piece

	// Prefix pieces (before the deleted region)
	newPieces = append(newPieces, pt.pieces[:startPieceIdx]...)

	// Handle the start piece (if we keep part of it)
	if startPieceOffset > 0 {
		startPiece := pt.pieces[startPieceIdx]
		newPieces = append(newPieces, piece{
			buffer: startPiece.buffer,
			start:  startPiece.start,
			length: startPieceOffset,
		})
	}

	// Handle the end piece (if we keep part of it)
	if endPieceOffset < pt.pieces[endPieceIdx].length {
		endPiece := pt.pieces[endPieceIdx]
		newPieces = append(newPieces, piece{
			buffer: endPiece.buffer,
			start:  endPiece.start + endPieceOffset,
			length: endPiece.length - endPieceOffset,
		})
	}

	// Suffix pieces (after the deleted region)
	if endPieceIdx+1 < len(pt.pieces) {
		newPieces = append(newPieces, pt.pieces[endPieceIdx+1:]...)
	}

	pt.pieces = newPieces
	pt.modified = true
	pt.invalidateCache()

	return editInfo, nil
}

func (pt *PieceTable) Save(filePath string) error {
	path := filePath
	if path == "" {
		path = pt.filePath
	}
	if path == "" {
		return fmt.Errorf("no file path specified")
	}

	err := os.WriteFile(path, pt.Bytes(), 0644)
	if err != nil {
		return err
	}

	pt.filePath = path
	pt.modified = false
	return nil
}

func (pt *PieceTable) FilePath() string {
	return pt.filePath
}

func (pt *PieceTable) IsModified() bool {
	return pt.modified
}
