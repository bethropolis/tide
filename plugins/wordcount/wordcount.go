// plugins/wordcount/wordcount.go
package wordcount

import (
	"bytes" // Needed for word counting
	"fmt"
	"strings"

	"github.com/bethropolis/tide/internal/plugin" // Import the plugin interface definitions
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
	// Note: Command execution isn't fully wired up yet in the core app,
	// but we register it now according to the plan.
	err := api.RegisterCommand("wc", p.executeWordCount)
	if err != nil {
		return fmt.Errorf("failed to register 'wc' command: %w", err)
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