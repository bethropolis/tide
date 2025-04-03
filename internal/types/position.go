// internal/types/position.go
package types

// Position represents a cursor or text position within the buffer.
// Line is the 0-based line index.
// Col is the 0-based column (rune) index within the line.
// Using Rune index is important for future Unicode handling.
type Position struct {
	Line int
	Col  int // Rune index
}