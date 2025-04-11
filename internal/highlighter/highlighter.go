package highlighter

import (
	"bytes"
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/highlighter/lang"
	"github.com/bethropolis/tide/internal/highlighter/utils"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// HighlightResult maps line numbers to styled ranges
type HighlightResult map[int][]types.StyledRange

// Highlighter handles syntax highlighting using tree-sitter
type Highlighter struct {
	parser *sitter.Parser
}

// NewHighlighter creates a new highlighter
func NewHighlighter() *Highlighter {
	// Initialize language registry
	lang.Initialize()

	// Register all supported languages
	RegisterLanguages()

	// Create the tree-sitter parser
	parser := sitter.NewParser()

	return &Highlighter{
		parser: parser,
	}
}

// GetLanguage retrieves language info for a file
func (h *Highlighter) GetLanguage(filePath string) (*sitter.Language, []byte) {
	// Find the language for this file extension
	language := lang.GetForFile(filePath)
	if language == nil {
		logger.Debugf("No language found for file: %s", filePath)
		return nil, nil
	}

	// Return the tree-sitter language and query
	return language.TreeSitterLang, language.GetQuery()
}

// HighlightBuffer performs syntax highlighting on buffer *content bytes*.
// Accepts sourceCode []byte instead of buffer.Buffer.
func (h *Highlighter) HighlightBuffer(ctx context.Context, sourceCode []byte, lang *sitter.Language, queryBytes []byte, oldTree *sitter.Tree) (HighlightResult, *sitter.Tree, error) {
	if lang == nil {
		return make(HighlightResult), oldTree, fmt.Errorf("no language provided for highlighting")
	}

	// --- Parse the source code ---
	h.parser.SetLanguage(lang)
	tree, err := h.parser.ParseCtx(ctx, oldTree, sourceCode)
	if err != nil {
		// If context was cancelled, return the cancellation error directly
		if ctx.Err() != nil {
			if oldTree != nil {
				oldTree.Close()
			} // Ensure old tree is closed on error too
			return nil, nil, ctx.Err() // Propagate cancellation
		}
		logger.Errorf("Tree-sitter parsing error: %v", err)
		if oldTree != nil {
			oldTree.Close()
		}
		return make(HighlightResult), nil, fmt.Errorf("parsing failed: %w", err) // Return empty result and new nil tree on parse error
	}

	// If no query is available, just return the parsed tree and empty highlights
	if queryBytes == nil {
		logger.Debugf("No query available for language, skipping highlighting")
		// Return the new tree, but empty highlights
		return make(HighlightResult), tree, nil
	}

	// --- Execute Query ---
	query, err := sitter.NewQuery(queryBytes, lang)
	if err != nil {
		logger.Errorf("Failed to parse highlight query: %v", err)
		tree.Close()                                                                 // Close the tree we just parsed
		return make(HighlightResult), nil, fmt.Errorf("query parse failed: %w", err) // Return empty result, nil tree
	}
	defer query.Close()

	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(query, tree.RootNode())

	// --- Pre-split source code into lines for efficient processing ---
	// This avoids repeatedly searching for newlines inside processLine/processMultiLine
	lines := bytes.Split(sourceCode, []byte("\n"))

	// --- Process Matches ---
	highlights := make(HighlightResult)
	matchCount := 0
	captureCount := 0

	for {
		match, exists := qc.NextMatch()
		if !exists {
			break // No more matches
		}
		// Check for cancellation *inside* the loop
		if ctx.Err() != nil {
			logger.Debugf("Context cancelled during query processing")
			tree.Close()
			return nil, nil, ctx.Err() // Return cancellation error
		}
		matchCount++

		for _, capture := range match.Captures {
			captureCount++
			captureName := query.CaptureNameForId(capture.Index)
			node := capture.Node
			styleName := utils.CaptureNameToStyleName(captureName)

			startPoint := node.StartPoint()
			endPoint := node.EndPoint()
			startLine := int(startPoint.Row)
			endLine := int(endPoint.Row)

			// Process capture using the pre-split lines
			if startLine == endLine {
				processLine(lines, highlights, startLine, styleName,
					int(startPoint.Column), int(endPoint.Column))
			} else {
				processMultiLine(lines, highlights, startLine, endLine, styleName,
					int(startPoint.Column), int(endPoint.Column))
			}
		}
	}

	logger.Debugf("Processed %d matches with %d captures, found highlights on %d lines",
		matchCount, captureCount, len(highlights))

	return highlights, tree, nil
}

// processLine processes a single-line capture using pre-split lines.
func processLine(lines [][]byte, highlights HighlightResult, lineIdx int, styleName string, startByteCol, endByteCol int) {
	// Bounds check for the line index
	if lineIdx < 0 || lineIdx >= len(lines) {
		logger.Warnf("processLine: Invalid line index %d (total lines %d)", lineIdx, len(lines))
		return
	}
	lineBytes := lines[lineIdx]

	// Clamp byte columns to line boundaries
	if startByteCol < 0 {
		startByteCol = 0
	}
	if endByteCol > len(lineBytes) {
		endByteCol = len(lineBytes)
	}
	if startByteCol > len(lineBytes) {
		startByteCol = len(lineBytes)
	} // Clamp startByteCol as well
	if startByteCol >= endByteCol {
		return
	} // Skip if range is invalid or empty

	startRuneCol := utils.ByteOffsetToRuneIndex(lineBytes, startByteCol)
	endRuneCol := utils.ByteOffsetToRuneIndex(lineBytes, endByteCol)

	// Only add if rune range is valid (start < end)
	if endRuneCol > startRuneCol {
		styledRange := types.StyledRange{StartCol: startRuneCol, EndCol: endRuneCol, StyleName: styleName}
		highlights[lineIdx] = append(highlights[lineIdx], styledRange)
	}
}

// processMultiLine processes a multi-line capture using pre-split lines.
func processMultiLine(lines [][]byte, highlights HighlightResult, startLine, endLine int, styleName string, startByteCol, endByteCol int) {

	// --- 1. Start Line ---
	if startLine >= 0 && startLine < len(lines) {
		lineBytes := lines[startLine]
		startBC := startByteCol // Local var for byte column
		if startBC < 0 {
			startBC = 0
		}
		if startBC > len(lineBytes) {
			startBC = len(lineBytes)
		} // Clamp start

		startRuneCol := utils.ByteOffsetToRuneIndex(lineBytes, startBC)
		endRuneCol := utf8.RuneCount(lineBytes) // Highlight to end of the start line

		if endRuneCol > startRuneCol {
			styledRange := types.StyledRange{StartCol: startRuneCol, EndCol: endRuneCol, StyleName: styleName}
			highlights[startLine] = append(highlights[startLine], styledRange)
		}
	} else {
		logger.Warnf("processMultiLine: Invalid start line index %d", startLine)
	}

	// --- 2. Middle Lines (Full line highlight) ---
	for lineIdx := startLine + 1; lineIdx < endLine; lineIdx++ {
		if lineIdx >= 0 && lineIdx < len(lines) {
			lineBytes := lines[lineIdx]
			endRuneCol := utf8.RuneCount(lineBytes)
			if endRuneCol > 0 { // Only add if line is not empty
				styledRange := types.StyledRange{StartCol: 0, EndCol: endRuneCol, StyleName: styleName}
				highlights[lineIdx] = append(highlights[lineIdx], styledRange)
			}
		} else {
			logger.Warnf("processMultiLine: Invalid middle line index %d", lineIdx)
		}
	}

	// --- 3. End Line ---
	if endLine >= 0 && endLine < len(lines) {
		lineBytes := lines[endLine]
		endBC := endByteCol // Local var for byte column
		if endBC < 0 {
			endBC = 0
		} // Should not happen but safety clamp
		if endBC > len(lineBytes) {
			endBC = len(lineBytes)
		} // Clamp end

		endRuneCol := utils.ByteOffsetToRuneIndex(lineBytes, endBC)

		if endRuneCol > 0 { // Highlight from start up to endRuneCol
			styledRange := types.StyledRange{StartCol: 0, EndCol: endRuneCol, StyleName: styleName}
			highlights[endLine] = append(highlights[endLine], styledRange)
		}
	} else {
		logger.Warnf("processMultiLine: Invalid end line index %d", endLine)
	}
}
