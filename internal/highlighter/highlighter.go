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
// Now accepts the previous tree for incremental parsing and returns the new tree.
func (h *Highlighter) HighlightBuffer(ctx context.Context, buf buffer.Buffer, lang *sitter.Language, oldTree *sitter.Tree) (HighlightResult, *sitter.Tree, error) {
	logger.Debugf("HighlightBuffer: Starting for language %v", lang)
	logger.Debugf("HighlightBuffer: Embedded query size: %d bytes", len(goHighlightsQuery))

	if lang == nil {
		return nil, oldTree, fmt.Errorf("no language provided for highlighting")
	}
	h.parser.SetLanguage(lang)

	sourceCode := buf.Bytes() // Get entire buffer content

	// Parse the source code to get the syntax tree
	// Use provided context and oldTree for incremental parsing
	tree, err := h.parser.ParseCtx(ctx, oldTree, sourceCode)
	if err != nil {
		logger.Errorf("Tree-sitter parsing error: %v", err)
		return nil, oldTree, fmt.Errorf("parsing failed: %w", err)
	}
	logger.Debugf("HighlightBuffer: Parsing successful: %t", err == nil)
	// Note: Don't close the tree here anymore, as the Editor will manage its lifecycle

	// --- Query the Tree ---
	// TODO: Cache queries per language
	query, err := sitter.NewQuery(goHighlightsQuery, lang) // Use embedded query bytes
	if err != nil {
		logger.Errorf("Failed to parse highlight query: %v", err)
		tree.Close() // Close the newly parsed tree if query fails
		return nil, oldTree, fmt.Errorf("query parse failed: %w", err)
	}
	defer query.Close() // Close query when done

	qc := sitter.NewQueryCursor()   // Create a query cursor
	defer qc.Close()                // Close cursor when done
	qc.Exec(query, tree.RootNode()) // Execute query on the root node

	highlights := make(HighlightResult)
	captureCount := 0
	matchCount := 0

	// Iterate through query matches
	for {
		match, exists := qc.NextMatch()
		if !exists || ctx.Err() != nil { // Check for context cancellation
			break // No more matches or context canceled
		}
		matchCount++

		// A match can have multiple captures
		for _, capture := range match.Captures {
			captureCount++

			captureName := query.CaptureNameForId(capture.Index)
			node := capture.Node

			// Get node boundaries (byte offsets and Point)
			startPoint := node.StartPoint()
			endPoint := node.EndPoint()

			// --- Convert to Line/Rune-Column ---
			startLine := int(startPoint.Row)
			endLine := int(endPoint.Row)
			styleName := captureNameToStyleName(captureName)

			// Log the first 10 captures for debugging
			if captureCount <= 10 {
				logger.Debugf("HighlightBuffer: Found capture '%s', mapped to style '%s' at line %d", captureName, styleName, startLine)
			}

			if startLine == endLine {
				// --- Single Line Capture ---
				lineBytes, lineErr := buf.Line(startLine)
				if lineErr != nil {
					logger.Warnf("HighlightBuffer: Cannot get line %d for highlight: %v", startLine, lineErr)
					continue // Skip this capture if line not found
				}
				startCol := byteOffsetToRuneIndex(lineBytes, int(startPoint.Column))
				endCol := byteOffsetToRuneIndex(lineBytes, int(endPoint.Column))
				if endCol > startCol { // Ensure valid range
					styledRange := types.StyledRange{StartCol: startCol, EndCol: endCol, StyleName: styleName}
					highlights[startLine] = append(highlights[startLine], styledRange)
				}
			} else {
				// --- Multi-Line Capture ---
				// 1. Highlight part on the start line
				startLineBytes, startLineErr := buf.Line(startLine)
				if startLineErr == nil {
					startCol := byteOffsetToRuneIndex(startLineBytes, int(startPoint.Column))
					endCol := utf8.RuneCount(startLineBytes) // To end of line
					if endCol > startCol {
						styledRange := types.StyledRange{StartCol: startCol, EndCol: endCol, StyleName: styleName}
						highlights[startLine] = append(highlights[startLine], styledRange)
					}
				} else {
					logger.Warnf("HighlightBuffer: Cannot get start line %d for highlight: %v", startLine, startLineErr)
				}

				// 2. Highlight intermediate full lines
				for line := startLine + 1; line < endLine; line++ {
					lineBytes, lineErr := buf.Line(line)
					if lineErr == nil {
						endCol := utf8.RuneCount(lineBytes)
						if endCol > 0 {
							styledRange := types.StyledRange{StartCol: 0, EndCol: endCol, StyleName: styleName}
							highlights[line] = append(highlights[line], styledRange)
						}
					} else {
						logger.Warnf("HighlightBuffer: Cannot get intermediate line %d for highlight: %v", line, lineErr)
					}
				}

				// 3. Highlight part on the end line
				endLineBytes, endLineErr := buf.Line(endLine)
				if endLineErr == nil {
					startCol := 0 // From start of line
					endCol := byteOffsetToRuneIndex(endLineBytes, int(endPoint.Column))
					if endCol > startCol {
						styledRange := types.StyledRange{StartCol: startCol, EndCol: endCol, StyleName: styleName}
						highlights[endLine] = append(highlights[endLine], styledRange)
					}
				} else {
					logger.Warnf("HighlightBuffer: Cannot get end line %d for highlight: %v", endLine, endLineErr)
				}
			}
		}
	}

	if ctx.Err() != nil {
		logger.Debugf("HighlightBuffer: Context cancelled during query processing.")
		tree.Close()                   // Clean up the tree if we were cancelled
		return nil, oldTree, ctx.Err() // Return old tree and context error
	}

	logger.Debugf("HighlightBuffer: Processed %d matches with %d captures", matchCount, captureCount)
	logger.Debugf("HighlightBuffer: Querying complete. Found highlights on %d lines.", len(highlights))

	// Return highlights and the new tree for future incremental parsing
	return highlights, tree, nil
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
		// Log potential issues with rune decoding for suspicious values
		if size == 0 && currentOffset < len(line) {
			logger.Warnf("byteOffsetToRuneIndex: Zero rune size at offset %d of %d", currentOffset, len(line))
			break
		}
		if currentOffset+size > byteOffset {
			break
		}
		currentOffset += size
		runeIndex++
	}
	return runeIndex
}
