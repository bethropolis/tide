// internal/highlighter/languages.go
package highlighter

import (
	 "embed"

	"github.com/bethropolis/tide/internal/highlighter/lang"
	"github.com/bethropolis/tide/internal/logger"

	// Import the grammar bindings
	gosrc "github.com/smacker/go-tree-sitter/golang"
	jssrc "github.com/smacker/go-tree-sitter/javascript" // JS parser used for JS and JSON
	pythonsrc "github.com/smacker/go-tree-sitter/python"
	rustsrc "github.com/smacker/go-tree-sitter/rust"
)


//go:embed queries/*/*.scm
var embeddedQueries embed.FS 

func RegisterLanguages() {
    if lang.QueryFS == nil {
        logger.Debugf("RegisterLanguages: Setting lang.QueryFS")
	    lang.QueryFS = embeddedQueries
    }

	logger.Debugf("Registering languages...")

	// Go
	lang.Register(&lang.Language{
		Name:           "Go",
		TreeSitterLang: gosrc.GetLanguage(),
		Extensions:     []string{".go"},
		QueryPath:      "go", // Matches directory name under queries/
	})

	// Python
	lang.Register(&lang.Language{
		Name:           "Python",
		TreeSitterLang: pythonsrc.GetLanguage(),
		Extensions:     []string{".py", ".pyw"}, // Add relevant extensions
		QueryPath:      "python",
	})

	// JavaScript (Using JS parser)
	lang.Register(&lang.Language{
		Name:           "JavaScript",
		TreeSitterLang: jssrc.GetLanguage(),
		Extensions:     []string{".js", ".mjs", ".cjs"},
		QueryPath:      "javascript", // Assumes queries/javascript/highlights.scm
	})

	lang.Register(&lang.Language{
		Name:           "JSON",
		TreeSitterLang: jssrc.GetLanguage(), // Or use jssrc.GetLanguage() if preferred
		Extensions:     []string{".json"},
		QueryPath:      "json", // <<< Ensure this matches queries/json/highlights.scm
	})

	// Rust
	lang.Register(&lang.Language{
		Name:           "Rust",
		TreeSitterLang: rustsrc.GetLanguage(),
		Extensions:     []string{".rs"},
		QueryPath:      "rust",
	})


	logger.Debugf("Registration complete. Registered %d languages.", len(lang.GetAll()))
}