// plugins/wordcount/wordcount.go
package wordcount

import (
	"bytes" // Needed for word counting
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/plugin" // Import the plugin interface definitions
	"github.com/bethropolis/tide/internal/types"  // For Position type
)

// Ensure WordCount implements plugin.Plugin
var _ plugin.Plugin = (*WordCount)(nil)

// WordCount is a simple plugin to count lines, words, and bytes.
type WordCount struct {
	api plugin.EditorAPI // Store the API for later use
}

// New creates a new instance of the WordCount plugin.
func New() *WordCount {
	return &WordCount{}
}

// Name returns the unique name of the plugin.
func (p *WordCount) Name() string {
	return "WordCount"
}

// Initialize is called when the plugin loads.
// We register our command here.
func (p *WordCount) Initialize(api plugin.EditorAPI) error {
	p.api = api // Store the API

	// Register the :wc command
	err := api.RegisterCommand("wc", p.executeWordCount)
	if err != nil {
		return fmt.Errorf("failed to register 'wc' command: %w", err)
	}

	// Register the :s command for substitution
	err = api.RegisterCommand("s", p.executeSubstitute)
	if err != nil {
		return fmt.Errorf("failed to register 's' command: %w", err)
	}

	return nil
}

// Shutdown performs cleanup (nothing needed for this simple plugin).
func (p *WordCount) Shutdown() error {
	return nil
}

// executeWordCount is the function called when the :wc command runs.
func (p *WordCount) executeWordCount(args []string) error {
	if p.api == nil {
		return fmt.Errorf("wordcount plugin not initialized with API")
	}

	// 1. Get buffer content
	bufferBytes := p.api.GetBufferBytes()
	lineCount := p.api.GetBufferLineCount() // Get line count directly

	// 2. Calculate stats
	byteCount := len(bufferBytes)
	// Simple word count: split by whitespace
	wordCount := len(bytes.Fields(bufferBytes))

	// 3. Display results in the status bar
	resultMsg := fmt.Sprintf("Lines: %d, Words: %d, Bytes: %d", lineCount, wordCount, byteCount)
	p.api.SetStatusMessage(resultMsg) // Use API to show message

	return nil
}

// executeSubstitute implements a basic :s/old/new/g command.
func (p *WordCount) executeSubstitute(args []string) error {
	if p.api == nil {
		return fmt.Errorf("wordcount plugin not initialized with API")
	}

	if len(args) == 0 {
		return fmt.Errorf("usage: :s/pattern/replacement/[g]")
	}

	// Parse the substitute command format
	cmdStr := args[0]
	parts := strings.SplitN(cmdStr, "/", 4)
	if len(parts) < 3 {
		return fmt.Errorf("invalid format: use /pattern/replacement/[g]")
	}

	pattern := parts[1]
	replacement := parts[2]
	global := false
	if len(parts) > 3 && parts[3] == "g" {
		global = true
	}

	if pattern == "" {
		return fmt.Errorf("search pattern cannot be empty")
	}

	// Compile regex
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	// Get buffer contents
	bufferBytes := p.api.GetBufferBytes()

	// Perform the replacement
	var resultBytes []byte
	var replaceCount int

	if global {
		// Replace all occurrences
		resultBytes = re.ReplaceAll(bufferBytes, []byte(replacement))
		matches := re.FindAllIndex(bufferBytes, -1)
		replaceCount = len(matches)
	} else {
		// Replace only first occurrence
		loc := re.FindIndex(bufferBytes)
		if loc != nil {
			resultBytes = make([]byte, 0, len(bufferBytes))
			resultBytes = append(resultBytes, bufferBytes[:loc[0]]...)
			resultBytes = append(resultBytes, []byte(replacement)...)
			resultBytes = append(resultBytes, bufferBytes[loc[1]:]...)
			replaceCount = 1
		} else {
			resultBytes = bufferBytes
			replaceCount = 0
		}
	}

	if replaceCount > 0 {
		// Replace buffer content
		// First clear the buffer
		lineCount := p.api.GetBufferLineCount()
		if lineCount > 0 {
			lastLineBytes, _ := p.api.GetBufferLine(lineCount - 1)
			lastColRunes := utf8.RuneCount(lastLineBytes)

			err = p.api.DeleteRange(
				types.Position{Line: 0, Col: 0},
				types.Position{Line: lineCount - 1, Col: lastColRunes},
			)
			if err != nil {
				return fmt.Errorf("failed to clear buffer: %w", err)
			}

			// Then insert the new content
			err = p.api.InsertText(types.Position{Line: 0, Col: 0}, resultBytes)
			if err != nil {
				return fmt.Errorf("failed to insert new content: %w", err)
			}
		}

		p.api.SetStatusMessage("Replaced %d occurrence(s)", replaceCount)
	} else {
		p.api.SetStatusMessage("Pattern not found: %s", pattern)
	}

	return nil
}

// Helper function (alternative word count if needed) - counts sequences of non-space chars
func countWords(data []byte) int {
	count := 0
	inWord := false
	for _, r := range string(data) { // Iterate runes
		if !strings.ContainsRune(" \t\n\r", r) { // If it's not whitespace
			if !inWord {
				count++
				inWord = true
			}
		} else {
			inWord = false
		}
	}
	return count
}
