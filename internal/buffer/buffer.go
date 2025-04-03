// internal/buffer/buffer.go
package buffer

import "github.com/bethropolis/tide/internal/types" // Import types instead of core

// Buffer defines the interface for text buffer operations.
type Buffer interface {
	Load(filePath string) error
	Lines() [][]byte
	Line(index int) ([]byte, error)
	LineCount() int
	// Use types.Position in the interface definition
	Insert(pos types.Position, text []byte) error
	Delete(start, end types.Position) error
	Save(filePath string) error
	Bytes() []byte
	FilePath() string
	IsModified() bool
}