package highlighter

import (
	"context" // Required by tree-sitter library
	_ "embed" // Import embed
	"fmt"
	"strings"
	"unicode/utf8" // Import utf8 for rune handling

	"github.com/bethropolis/tide/internal/buffer" // Needs buffer content
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types" // For HighlightRegion etc.
	sitter "github.com/smacker/go-tree-sitter"
	gosrc "github.com/smacker/go-tree-sitter/golang"
)

//go:embed queries/go/highlights.scm
var goHighlightsQuery []byte // Embed the query file content

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

	// --- Query the Tree ---
	// TODO: Cache queries per language
	query, err := sitter.NewQuery(goHighlightsQuery, lang) // Use embedded query bytes
	if err != nil {
		logger.Errorf("Failed to parse highlight query: %v", err)
		return nil, fmt.Errorf("query parse failed: %w", err)
	}

	qc := sitter.NewQueryCursor()   // Create a query cursor
	qc.Exec(query, tree.RootNode()) // Execute query on the root node

	highlights := make(HighlightResult)

	// Iterate through query matches
	for {
		match, exists := qc.NextMatch()
		if !exists {
			break // No more matches
		}

		// A match can have multiple captures (e.g., function call might capture function name and arguments)
		// We iterate through the captures for the *current* match.
		for _, capture := range match.Captures {
			// 'capture.Node' is the specific node in the syntax tree that matched
			// 'capture.Index' is the index of the capture name in query.CaptureNameForId()

			captureName := query.CaptureNameForId(capture.Index) // Get the capture name (e.g., "keyword", "string")
			node := capture.Node

			// Get node boundaries (byte offsets and Point)
			startPoint := node.StartPoint() // Point{Row, Column (bytes)}
			endPoint := node.EndPoint()

			// --- Convert to Line/Rune-Column ---
			// We need the content of the lines involved to do the conversion accurately.
			// This is inefficient if captures span many lines.
			// TODO: Optimize by getting line content only once per line needed.

			startLine := int(startPoint.Row)
			endLine := int(endPoint.Row) // Might span lines

			currentLineNum := startLine
			currentLineBytes, lineErr := buf.Line(currentLineNum) // Get content of start line

			if lineErr != nil {
				logger.Warnf("HighlightBuffer: Cannot get line %d for highlight: %v", currentLineNum, lineErr)
				continue // Skip this capture if line not found
			}

			// Calculate start rune column on the start line
			highlightStartCol := byteOffsetToRuneIndex(currentLineBytes, int(startPoint.Column))

			// Calculate end rune column - tricky for multi-line captures
			highlightEndCol := 0
			if startLine == endLine {
				// Capture ends on the same line
				highlightEndCol = byteOffsetToRuneIndex(currentLineBytes, int(endPoint.Column))
			} else {
				// Capture spans multiple lines. Highlight to end of start line.
				// TODO: Handle multi-line highlights properly (create ranges for intermediate lines and end line)
				highlightEndCol = utf8.RuneCount(currentLineBytes) // Highlight to end of this line for now
			}

			// Ensure EndCol is strictly greater than StartCol for a valid range
			if highlightEndCol <= highlightStartCol {
				// This can happen for zero-width nodes or errors. Skip.
				continue
			}

			// --- Store Highlight ---
			styleName := captureNameToStyleName(captureName) // Map tree-sitter capture name to our style name

			// Add the styled range to the results for the specific line
			styledRange := types.StyledRange{
				StartCol:  highlightStartCol,
				EndCol:    highlightEndCol,
				StyleName: styleName,
			}
			highlights[startLine] = append(highlights[startLine], styledRange)

			// TODO: If capture spanned multiple lines, add ranges for intermediate lines (full line highlight)
			// and the portion on the end line. This requires more complex logic.
		}
	}

	logger.Debugf("HighlightBuffer: Querying complete. Found highlights on %d lines.", len(highlights))
	return highlights, nil
}

// captureNameToStyleName maps Tree-sitter capture names (like @keyword.control)
// to the style names used by our theme system.
func captureNameToStyleName(captureName string) string {
	// Simple mapping for now. Can be made more sophisticated (e.g., using '.' hierarchy).
	// Strip the leading '@' if present
	if len(captureName) > 0 && captureName[0] == '@' {
		captureName = captureName[1:]
	}
	// Use the first part before a dot as the general style for now
	// e.g., "keyword.control" -> "keyword"
	if dotIndex := strings.Index(captureName, "."); dotIndex != -1 {
		return captureName[:dotIndex]
	}
	return captureName // Return full name if no dot
}

// --- Helpers ---

// byteOffsetToRuneIndex converts a byte offset to a rune index in a byte slice.
// (Copied from core/editor.go - TODO: Move to a shared util package?)
func byteOffsetToRuneIndex(line []byte, byteOffset int) int {
	if byteOffset <= 0 {
		return 0
	}
	if byteOffset > len(line) {
		byteOffset = len(line)
	}
	runeIndex := 0
	currentOffset := 0
	for currentOffset < byteOffset {
		_, size := utf8.DecodeRune(line[currentOffset:])
		if currentOffset+size > byteOffset {
			break
		}
		currentOffset += size
		runeIndex++
	}
	return runeIndex
}
