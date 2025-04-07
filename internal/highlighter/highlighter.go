package highlighter

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/buffer"
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

// HighlightBuffer performs syntax highlighting on a buffer
func (h *Highlighter) HighlightBuffer(ctx context.Context, buf buffer.Buffer, lang *sitter.Language, queryBytes []byte, oldTree *sitter.Tree) (HighlightResult, *sitter.Tree, error) {
	if lang == nil {
		return make(HighlightResult), oldTree, fmt.Errorf("no language provided for highlighting")
	}

	// No query available? Just parse without highlighting
	if queryBytes == nil {
		logger.Debugf("No query available for language, skipping highlighting")
		h.parser.SetLanguage(lang)
		tree, err := h.parser.ParseCtx(ctx, oldTree, buf.Bytes())
		if err != nil {
			logger.Errorf("Tree-sitter parsing error: %v", err)
			return make(HighlightResult), oldTree, fmt.Errorf("parsing failed: %w", err)
		}
		return make(HighlightResult), tree, nil
	}

	// Set language and parse
	h.parser.SetLanguage(lang)
	sourceCode := buf.Bytes()
	tree, err := h.parser.ParseCtx(ctx, oldTree, sourceCode)
	if err != nil {
		logger.Errorf("Tree-sitter parsing error: %v", err)
		return nil, oldTree, fmt.Errorf("parsing failed: %w", err)
	}

	// Create and execute the query
	query, err := sitter.NewQuery(queryBytes, lang)
	if err != nil {
		logger.Errorf("Failed to parse highlight query: %v", err)
		tree.Close()
		return nil, oldTree, fmt.Errorf("query parse failed: %w", err)
	}
	defer query.Close()

	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(query, tree.RootNode())

	// Store highlighting results
	highlights := make(HighlightResult)
	matchCount := 0
	captureCount := 0

	// Process query matches
	for {
		match, exists := qc.NextMatch()
		if !exists || ctx.Err() != nil {
			break
		}
		matchCount++

		// Process each capture in the match
		for _, capture := range match.Captures {
			captureCount++

			captureName := query.CaptureNameForId(capture.Index)
			node := capture.Node

			// Get node positions
			startPoint := node.StartPoint()
			endPoint := node.EndPoint()
			startLine := int(startPoint.Row)
			endLine := int(endPoint.Row)
			styleName := utils.CaptureNameToStyleName(captureName)

			// Debug the first few captures
			if captureCount <= 10 {
				logger.Debugf("Found capture '%s', mapped to style '%s' at line %d",
					captureName, styleName, startLine)
			}

			// Process based on whether it's a single-line or multi-line capture
			if startLine == endLine {
				processLine(buf, highlights, startLine, styleName,
					int(startPoint.Column), int(endPoint.Column))
			} else {
				processMultiLine(buf, highlights, startLine, endLine, styleName,
					int(startPoint.Column), int(endPoint.Column))
			}
		}
	}

	// Handle context cancellation
	if ctx.Err() != nil {
		logger.Debugf("Context cancelled during query processing")
		tree.Close()
		return nil, oldTree, ctx.Err()
	}

	logger.Debugf("Processed %d matches with %d captures, found highlights on %d lines",
		matchCount, captureCount, len(highlights))

	return highlights, tree, nil
}

// Process a single line capture
func processLine(buf buffer.Buffer, highlights HighlightResult, line int, styleName string, startCol, endCol int) {
	lineBytes, err := buf.Line(line)
	if err != nil {
		logger.Warnf("Cannot get line %d for highlight: %v", line, err)
		return
	}

	start := utils.ByteOffsetToRuneIndex(lineBytes, startCol)
	end := utils.ByteOffsetToRuneIndex(lineBytes, endCol)

	if end > start {
		styledRange := types.StyledRange{StartCol: start, EndCol: end, StyleName: styleName}
		highlights[line] = append(highlights[line], styledRange)
	}
}

// Process a multi-line capture
func processMultiLine(buf buffer.Buffer, highlights HighlightResult, startLine, endLine int, styleName string,
	startCol, endCol int) {

	// 1. Start line
	startLineBytes, err := buf.Line(startLine)
	if err == nil {
		start := utils.ByteOffsetToRuneIndex(startLineBytes, startCol)
		end := utf8.RuneCount(startLineBytes)
		if end > start {
			styledRange := types.StyledRange{StartCol: start, EndCol: end, StyleName: styleName}
			highlights[startLine] = append(highlights[startLine], styledRange)
		}
	}

	// 2. Middle lines (full-line highlighting)
	for line := startLine + 1; line < endLine; line++ {
		lineBytes, err := buf.Line(line)
		if err == nil {
			endCol := utf8.RuneCount(lineBytes)
			if endCol > 0 {
				styledRange := types.StyledRange{StartCol: 0, EndCol: endCol, StyleName: styleName}
				highlights[line] = append(highlights[line], styledRange)
			}
		}
	}

	// 3. End line
	endLineBytes, err := buf.Line(endLine)
	if err == nil {
		end := utils.ByteOffsetToRuneIndex(endLineBytes, endCol)
		if end > 0 {
			styledRange := types.StyledRange{StartCol: 0, EndCol: end, StyleName: styleName}
			highlights[endLine] = append(highlights[endLine], styledRange)
		}
	}
}
