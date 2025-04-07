package highlighter

import (
	"context"
	"embed" // Use embed for multiple files
	"fmt"
	"path/filepath" // To get file extensions
	"strings"
	"sync" // Needed for lazy loading maps
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
	sitter "github.com/smacker/go-tree-sitter"

	// Import the grammar bindings
	gosrc "github.com/smacker/go-tree-sitter/golang"
	jsonsrc "github.com/smacker/go-tree-sitter/javascript"
	pythonsrc "github.com/smacker/go-tree-sitter/python"
)

//go:embed queries/*/*.scm
var embeddedQueries embed.FS // Embed the whole queries directory

// --- Language Configuration ---

type LanguageConfig struct {
	Language *sitter.Language
	Query    []byte
}

var (
	// Map file extensions to language names (lowercase)
	extToLangName = map[string]string{
		".go":   "golang",
		".py":   "python",
		".json": "json",
		".js":   "javascript", // Add JavaScript extension mapping
		// Add more extensions here: .ts, .html, .css, .md, .yaml, .toml, etc.
	}

	// Lazy-loaded maps for language configs
	languageConfigs map[string]*LanguageConfig
	langLoadOnce    sync.Once
	langLoadErr     error
)

// loadLanguages initializes grammars and loads queries ONCE.
func loadLanguages() {
	langLoadOnce.Do(func() {
		logger.Debugf("Highlighter: Initializing languages and loading queries...")
		languageConfigs = make(map[string]*LanguageConfig)

		// Helper to load query, handling errors
		loadQuery := func(langName string, filename string) []byte {
			// If no specific filename provided, use default pattern
			if filename == "" {
				filename = "highlights.scm"
			}

			path := fmt.Sprintf("queries/%s/%s", langName, filename)
			queryBytes, err := embeddedQueries.ReadFile(path)
			if err != nil {
				logger.Warnf("Failed to load highlight query '%s': %v", path, err)
				return nil // Return nil if query doesn't exist
			}
			logger.Debugf("Loaded query for %s (%d bytes)", langName, len(queryBytes))
			return queryBytes
		}

		// Add supported languages
		languageConfigs["golang"] = &LanguageConfig{
			Language: gosrc.GetLanguage(),
			Query:    loadQuery("go", ""), // Directory name is 'go'
		}
		languageConfigs["python"] = &LanguageConfig{
			Language: pythonsrc.GetLanguage(),
			Query:    loadQuery("python", ""),
		}
		languageConfigs["json"] = &LanguageConfig{
			Language: jsonsrc.GetLanguage(),
			Query:    loadQuery("json", ""),
		}
		languageConfigs["javascript"] = &LanguageConfig{
			Language: jsonsrc.GetLanguage(),
			Query:    loadQuery("javascript", ""), 
		}

		// Add more languages here... 

		logger.Debugf("Highlighter: Language initialization complete.")
	})
}

// HighlightResult holds computed highlights for efficient lookup during drawing.
// Maps line number -> slice of styled ranges on that line.
type HighlightResult map[int][]types.StyledRange

// Highlighter service manages parsing and querying syntax trees.
type Highlighter struct {
	parser *sitter.Parser
}

// NewHighlighter ensures languages are loaded.
func NewHighlighter() *Highlighter {
	loadLanguages() // Ensure languages are loaded when highlighter is created
	if langLoadErr != nil {
		// Handle error during language loading if necessary
		logger.Errorf("Highlighter creation failed due to language loading error: %v", langLoadErr)
	}
	parser := sitter.NewParser()
	return &Highlighter{
		parser: parser,
	}
}

// GetLanguage looks up the language config based on file extension.
// It returns the sitter.Language and the loaded query bytes.
func (h *Highlighter) GetLanguage(filePath string) (*sitter.Language, []byte) {
	loadLanguages() // Ensure maps are populated
	ext := strings.ToLower(filepath.Ext(filePath))
	langName, ok := extToLangName[ext]
	if !ok {
		logger.Debugf("GetLanguage: No language registered for extension '%s'", ext)
		return nil, nil // Unknown extension
	}

	config, configOk := languageConfigs[langName]
	if !configOk || config.Language == nil {
		logger.Warnf("GetLanguage: Language '%s' configured but grammar not loaded", langName)
		return nil, nil // Configured but not loaded correctly
	}

	// Return language and query bytes (query might be nil if loading failed)
	return config.Language, config.Query
}

// HighlightBuffer uses the provided language and query bytes.
func (h *Highlighter) HighlightBuffer(ctx context.Context, buf buffer.Buffer, lang *sitter.Language, queryBytes []byte, oldTree *sitter.Tree) (HighlightResult, *sitter.Tree, error) {
	if lang == nil {
		return make(HighlightResult), oldTree, fmt.Errorf("no language provided for highlighting")
	}

	// Query bytes might be nil if loading failed or not provided for language
	if queryBytes == nil {
		logger.Debugf("HighlightBuffer: No query available for language %v, skipping query phase.", lang)
		// Still parse, just don't query
		h.parser.SetLanguage(lang)
		tree, err := h.parser.ParseCtx(ctx, oldTree, buf.Bytes())
		if err != nil {
			logger.Errorf("Tree-sitter parsing error: %v", err)
			return make(HighlightResult), oldTree, fmt.Errorf("parsing failed: %w", err)
		}
		return make(HighlightResult), tree, nil // Return empty highlights but new tree
	}

	h.parser.SetLanguage(lang)
	sourceCode := buf.Bytes()
	tree, err := h.parser.ParseCtx(ctx, oldTree, sourceCode)
	if err != nil {
		logger.Errorf("Tree-sitter parsing error: %v", err)
		return nil, oldTree, fmt.Errorf("parsing failed: %w", err)
	}

	query, err := sitter.NewQuery(queryBytes, lang) // Use provided query bytes
	if err != nil {
		logger.Errorf("Failed to parse highlight query: %v", err)
		tree.Close()
		return nil, oldTree, fmt.Errorf("query parse failed: %w", err)
	}
	defer query.Close()

	qc := sitter.NewQueryCursor()
	defer qc.Close()
	qc.Exec(query, tree.RootNode())

	highlights := make(HighlightResult)
	matchCount := 0
	captureCount := 0

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
	// Strip the leading '@' if present
	if len(captureName) > 0 && captureName[0] == '@' {
		captureName = captureName[1:]
	}

	// Option 1: Use only base name for simpler themes
	if dotIndex := strings.Index(captureName, "."); dotIndex != -1 {
		return captureName[:dotIndex]
	}

	return captureName // Return full name if no dot
}

// byteOffsetToRuneIndex converts a byte offset to a rune index in a byte slice.
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
