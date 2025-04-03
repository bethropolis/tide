package highlighter

import (
	"context" // Required by tree-sitter library
	"fmt"

	"github.com/bethropolis/tide/internal/buffer" // Needs buffer content
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types" // For HighlightRegion etc.
	sitter "github.com/smacker/go-tree-sitter"
	gosrc "github.com/smacker/go-tree-sitter/golang"
)

// HighlightResult holds computed highlights for efficient lookup during drawing.
// Maps line number -> slice of styled ranges on that line.
type HighlightResult map[int][]types.StyledRange

// Highlighter service manages parsing and querying syntax trees.
type Highlighter struct {
	parser *sitter.Parser
	// Cache for languages? map[string]*sitter.Language ?
}

// NewHighlighter creates a new highlighter instance.
func NewHighlighter() *Highlighter {
	parser := sitter.NewParser()
	return &Highlighter{
		parser: parser,
	}
}

// GetLanguage returns the Tree-sitter language definition.
// TODO: Detect language based on filename or content. Hardcode Go for now.
func (h *Highlighter) GetLanguage(filePath string) *sitter.Language {
	// Use the imported grammar package directly
	lang := gosrc.GetLanguage()
	// Later, use filePath to determine language and load dynamically or from cache.
	// e.g., lang := h.languageRegistry.GetLanguageForFile(filePath)
	return lang
}

// HighlightBuffer parses the buffer and generates highlight information.
// Returns HighlightResult or error.
// NOTE: This is NON-INCREMENTAL for now. Parses the whole buffer.
func (h *Highlighter) HighlightBuffer(buf buffer.Buffer, lang *sitter.Language) (HighlightResult, error) {
	if lang == nil {
		return nil, fmt.Errorf("no language provided for highlighting")
	}
	h.parser.SetLanguage(lang)

	sourceCode := buf.Bytes() // Get entire buffer content

	// Parse the source code to get the syntax tree
	// Use context.Background() for now
	tree, err := h.parser.ParseCtx(context.Background(), nil, sourceCode)
	if err != nil {
		logger.Errorf("Tree-sitter parsing error: %v", err)
		return nil, fmt.Errorf("parsing failed: %w", err)
	}
	defer tree.Close() // Important to close the tree

	// --- TODO: Query the tree ---
	// 1. Load highlight queries for the language (from .scm files).
	// 2. Execute queries against the tree using sitter.NewQueryCursor.
	// 3. Process captures from the query results.
	// 4. Map capture names (e.g., @keyword, @string) to our StyleName strings.
	// 5. Convert node start/end points (byte offsets, Point{Row, Col}) to our line/rune-column format.
	// 6. Build the HighlightResult map.

	logger.Debugf("HighlightBuffer: Successfully parsed buffer (Tree-sitter). Querying TODO.")

	// --- Placeholder Result (until querying is implemented) ---
	// Return an empty map for now, or maybe basic comment highlighting?
	result := make(HighlightResult)
	// Example: Find comments manually (very basic)
	rootNode := tree.RootNode()
	// We'd typically use a query cursor here. Manual traversal is less efficient/robust.
	// IterateChildren might be needed, or use QueryCursor later.

	// Placeholder: Return empty result until query logic is added.
	return result, nil
}

// --- Helper to convert sitter.Point to our types.Position (if needed) ---
// Note: sitter.Point uses 0-based row and *byte* column. Need conversion.
// func pointToPosition(p sitter.Point, lineContent []byte) types.Position {
//     row := int(p.Row)
//     byteCol := int(p.Column)
//     runeCol := byteOffsetToRuneIndex(lineContent, byteCol) // Need this helper from core
//     return types.Position{Line: row, Col: runeCol}
// }

// TODO: Need byteOffsetToRuneIndex helper accessible here or duplicated/moved.
