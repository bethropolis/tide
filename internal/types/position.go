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

// --- Add Highlight Types ---

// HighlightRegion defines a text range with a specific highlight type.
type HighlightRegion struct {
	Start Position
	End   Position
	Type  HighlightType
}

// HighlightType identifies the reason for highlighting.
type HighlightType string

const (
	HighlightSearch HighlightType = "search"
	// Add HighlightSyntax, HighlightError later
)

// StyledRange represents a segment of text with an associated style name (for theming).
type StyledRange struct {
	StartCol  int    // Rune column index (inclusive)
	EndCol    int    // Rune column index (exclusive)
	StyleName string // Semantic style name (e.g., "keyword", "comment", "string")
}
