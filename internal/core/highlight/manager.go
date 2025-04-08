package highlight

import (
	"sync"

	"github.com/bethropolis/tide/internal/buffer" // Import main buffer package
	hl "github.com/bethropolis/tide/internal/highlighter"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// Manager handles syntax highlighting
type Manager struct {
	editor           EditorInterface
	highlighter      *hl.Highlighter
	syntaxHighlights hl.HighlightResult
	syntaxTree       *sitter.Tree // Use concrete type
	mutex            sync.RWMutex
	searchHighlights []types.HighlightRegion
}

// EditorInterface defines methods needed from editor
type EditorInterface interface {
	GetBuffer() buffer.Buffer // Changed return type to concrete buffer.Buffer
	// Other required methods...
}

// NewManager creates a highlight manager
func NewManager(editor EditorInterface) *Manager {
	return &Manager{
		editor:           editor,
		syntaxHighlights: make(hl.HighlightResult),
		searchHighlights: make([]types.HighlightRegion, 0),
	}
}

// SetHighlighter sets the highlighter instance
func (m *Manager) SetHighlighter(h *hl.Highlighter) {
	m.highlighter = h
}

// UpdateHighlights updates syntax highlights with thread safety
func (m *Manager) UpdateHighlights(newHighlights hl.HighlightResult, newTree *sitter.Tree) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Close old tree if needed
	if m.syntaxTree != nil {
		m.syntaxTree.Close() // Call Close() directly
	}

	m.syntaxHighlights = newHighlights
	m.syntaxTree = newTree // Store directly as concrete type
}

// GetHighlightsForLine gets syntax highlights for a line
func (m *Manager) GetHighlightsForLine(lineNum int) []types.StyledRange {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if styles, ok := m.syntaxHighlights[lineNum]; ok {
		return styles
	}
	return nil
}

// GetCurrentTree gets the current syntax tree
func (m *Manager) GetCurrentTree() *sitter.Tree {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.syntaxTree
}

// TriggerHighlight triggers a highlight operation
func (m *Manager) TriggerHighlight() {
	logger.Debugf("Highlight: TriggerHighlight called (async)")
}

// HighlightSearchMatches adds search highlights
func (m *Manager) HighlightSearchMatches(term string) {
	// ... search highlight logic moved from editor
}

// HasHighlights checks if there are active highlights
func (m *Manager) HasHighlights() bool {
	return len(m.searchHighlights) > 0
}

// ClearHighlights removes all highlights
func (m *Manager) ClearHighlights() {
	if len(m.searchHighlights) > 0 {
		logger.Debugf("Highlight: Clearing %d highlights", len(m.searchHighlights))
		m.searchHighlights = m.searchHighlights[:0]
	}
}

// GetHighlights returns all search highlights
func (m *Manager) GetHighlights() []types.HighlightRegion {
	return m.searchHighlights
}
