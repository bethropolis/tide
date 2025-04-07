package lang

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/bethropolis/tide/internal/logger"
)

var (
	// Global language registry
	registry struct {
		sync.RWMutex
		languages     []*Language
		extToLanguage map[string]*Language
		initialized   bool
	}

	// One-time initialization
	initOnce sync.Once
)

// Initialize ensures the registry is ready for use
func Initialize() {
	initOnce.Do(func() {
		registry.extToLanguage = make(map[string]*Language)
		registry.languages = make([]*Language, 0)
		registry.initialized = true
		logger.Debugf("Language registry initialized")
	})
}

// Register adds a language to the registry
func Register(lang *Language) {
	// Ensure registry is initialized
	Initialize()

	registry.Lock()
	defer registry.Unlock()

	// Add to languages list
	registry.languages = append(registry.languages, lang)

	// Map each extension to this language
	for _, ext := range lang.Extensions {
		lowerExt := strings.ToLower(ext)
		if existing, ok := registry.extToLanguage[lowerExt]; ok {
			logger.Warnf("Extension %s already registered to %s, overriding with %s",
				lowerExt, existing.Name, lang.Name)
		}
		registry.extToLanguage[lowerExt] = lang
	}

	logger.Debugf("Registered language: %s with extensions: %v", lang.Name, lang.Extensions)
}

// GetForFile returns the language for a given file path
func GetForFile(filePath string) *Language {
	Initialize()

	registry.RLock()
	defer registry.RUnlock()

	ext := strings.ToLower(filepath.Ext(filePath))
	lang, ok := registry.extToLanguage[ext]
	if !ok {
		return nil
	}
	return lang
}

// GetAll returns all registered languages
func GetAll() []*Language {
	Initialize()

	registry.RLock()
	defer registry.RUnlock()

	result := make([]*Language, len(registry.languages))
	copy(result, registry.languages)
	return result
}