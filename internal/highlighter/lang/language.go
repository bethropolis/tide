package lang

import (
	"fmt"
	"io/fs"

	"github.com/bethropolis/tide/internal/logger"
	sitter "github.com/smacker/go-tree-sitter"
)

// QueryFS is the filesystem interface for accessing embedded queries
var QueryFS fs.FS

// Language represents a programming language with its syntax highlighting configuration
type Language struct {
	// Name is the display name of the language
	Name string

	// TreeSitterLang is the tree-sitter language instance
	TreeSitterLang *sitter.Language

	// Extensions maps file extensions to this language
	Extensions []string

	// QueryPath is the path to the highlight query file
	QueryPath string
}

// GetQuery loads and returns the highlight query for this language
func (l *Language) GetQuery() []byte {
	if QueryFS == nil {
		logger.Warnf("QueryFS not set - cannot load queries")
		return nil
	}

	if l.QueryPath == "" {
		logger.Warnf("No query path defined for language %s", l.Name)
		return nil
	}

	// Try standard highlight.scm first
	queryPath := fmt.Sprintf("queries/%s/highlight.scm", l.QueryPath)
	query, err := fs.ReadFile(QueryFS, queryPath)
	if err == nil {
		logger.Debugf("Loaded query from %s for %s (%d bytes)", queryPath, l.Name, len(query))
		return query
	}

	// Try highlights.scm as fallback
	queryPath = fmt.Sprintf("queries/%s/highlights.scm", l.QueryPath)
	query, err = fs.ReadFile(QueryFS, queryPath)
	if err == nil {
		logger.Debugf("Loaded query from %s for %s (%d bytes)", queryPath, l.Name, len(query))
		return query
	}

	logger.Warnf("Failed to load query for language %s: %v", l.Name, err)
	return nil
}
