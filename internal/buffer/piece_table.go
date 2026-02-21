package buffer

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/bethropolis/tide/internal/types"
)

// PieceTable is a more efficient buffer implementation for large files
type PieceTable struct {
	original []byte
	add      []byte
	pieces   []piece

	filePath string
	modified bool
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

func (pt *PieceTable) Bytes() []byte {
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
	return result.Bytes()
}

func (pt *PieceTable) Lines() [][]byte {
	return bytes.Split(pt.Bytes(), []byte{'\n'})
}

func (pt *PieceTable) Line(index int) ([]byte, error) {
	lines := pt.Lines()
	if index < 0 || index >= len(lines) {
		return nil, fmt.Errorf("line index out of bounds")
	}
	return lines[index], nil
}

func (pt *PieceTable) LineCount() int {
	return len(pt.Lines())
}

// Calculate absolute byte offset from a Position
func (pt *PieceTable) positionToOffset(pos types.Position) int {
	lines := pt.Lines()
	if pos.Line >= len(lines) {
		return len(pt.Bytes())
	}

	offset := 0
	for i := 0; i < pos.Line; i++ {
		offset += len(lines[i]) + 1 // +1 for '\n'
	}

	col := pos.Col
	if col > len(lines[pos.Line]) {
		col = len(lines[pos.Line])
	}
	return offset + col
}

func (pt *PieceTable) Insert(pos types.Position, text []byte) (types.EditInfo, error) {
	offset := pt.positionToOffset(pos)

	// Fast path for empty pieces
	if len(pt.pieces) == 1 && pt.pieces[0].length == 0 {
		pt.add = append(pt.add, text...)
		pt.pieces[0] = piece{buffer: addBuffer, start: 0, length: len(text)}
		pt.modified = true
		return types.EditInfo{}, nil
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

	return types.EditInfo{}, nil
}

func (pt *PieceTable) Delete(start, end types.Position) (types.EditInfo, error) {
	startOff := pt.positionToOffset(start)
	endOff := pt.positionToOffset(end)

	if startOff >= endOff {
		return types.EditInfo{}, nil
	}

	// A simpler (but less optimal) approach for now is to rebuild from Bytes()
	// to ensure correctness before optimizing
	content := pt.Bytes()

	// Ensure bounds are safe
	if startOff > len(content) {
		startOff = len(content)
	}
	if endOff > len(content) {
		endOff = len(content)
	}

	// Perform deletion by keeping parts outside the range
	var newContent []byte
	newContent = append(newContent, content[:startOff]...)
	newContent = append(newContent, content[endOff:]...)

	pt.original = newContent
	pt.add = []byte{}
	pt.pieces = []piece{{buffer: originalBuffer, start: 0, length: len(newContent)}}

	pt.modified = true

	return types.EditInfo{}, nil
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
