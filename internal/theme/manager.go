// internal/theme/manager.go
package theme

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bethropolis/tide/internal/logger"
	"github.com/gdamore/tcell/v2" // Add missing import for tcell
)

// Manager holds loaded themes and manages the active theme.
type Manager struct {
	themes      map[string]*Theme // Map theme name (lowercase) -> Theme object
	activeTheme *Theme
	themesDir   string
	mutex       sync.RWMutex
	loadError   error // Store error from initial load
}

// NewManager creates and initializes a theme manager.
func NewManager() *Manager {
	mgr := &Manager{
		themes: make(map[string]*Theme),
	}

	// Find themes directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		logger.Warnf("Could not find user config dir: %v. Themes cannot be loaded from default location.", err)
		mgr.themesDir = "" // No directory to load from
	} else {
		mgr.themesDir = filepath.Join(configDir, "tide", "themes")
	}

	// Load built-in themes first
	mgr.loadBuiltinThemes()

	// Load themes from directory (if found)
	if mgr.themesDir != "" {
		mgr.loadError = mgr.LoadThemesFromDir()
		if mgr.loadError != nil {
			logger.Errorf("Error loading themes from '%s': %v", mgr.themesDir, mgr.loadError)
			// Continue, but themes might be missing
		}
	}

	// Set initial active theme (try default, fallback to first loaded)
	if _, ok := mgr.themes["devcomfort dark"]; ok { // Check lowercase name
		mgr.activeTheme = mgr.themes["devcomfort dark"]
	} else if len(mgr.themes) > 0 {
		// Fallback to the first theme found if default isn't there
		for _, t := range mgr.themes {
			mgr.activeTheme = t
			break
		}
	}

	if mgr.activeTheme == nil {
		logger.Errorf("No themes loaded, cannot set active theme!")
		// Create a minimal failsafe theme
		mgr.activeTheme = &Theme{
			Name: "Failsafe",
			Styles: map[string]tcell.Style{
				"Default": tcell.StyleDefault,
			},
		}
	} else {
		logger.Infof("Initial active theme set to: %s", mgr.activeTheme.Name)
	}

	return mgr
}

// loadBuiltinThemes adds themes compiled into the binary.
func (m *Manager) loadBuiltinThemes() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Add our DevComfortDark theme (ensure init() has run for it)
	themeNameLower := stringsToLower(DevComfortDark.Name)
	m.themes[themeNameLower] = &DevComfortDark // Use lowercase name as key
	logger.Debugf("Loaded built-in theme: %s", DevComfortDark.Name)
}

// LoadThemesFromDir scans the themes directory and loads .toml files.
func (m *Manager) LoadThemesFromDir() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.themesDir == "" {
		return errors.New("theme directory path is not set")
	}

	// Ensure directory exists
	if _, err := os.Stat(m.themesDir); os.IsNotExist(err) {
		logger.Infof("Theme directory '%s' does not exist. No custom themes loaded.", m.themesDir)

		// Attempt to create the directory
		if err := os.MkdirAll(m.themesDir, 0755); err != nil {
			return fmt.Errorf("failed to create theme dir: %w", err)
		}
		return nil // Not an error if dir doesn't exist
	}

	logger.Infof("Loading themes from: %s", m.themesDir)
	files, err := os.ReadDir(m.themesDir)
	if err != nil {
		return fmt.Errorf("failed to read theme directory '%s': %w", m.themesDir, err)
	}

	loadedCount := 0
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(stringsToLower(file.Name()), ".toml") {
			filePath := filepath.Join(m.themesDir, file.Name())
			theme, err := LoadThemeFromFile(filePath) // Use the loader
			if err != nil {
				logger.Warnf("Failed to load theme from '%s': %v", filePath, err)
				continue // Skip problematic file
			}

			themeNameLower := stringsToLower(theme.Name)
			if existing, ok := m.themes[themeNameLower]; ok {
				logger.Warnf("Theme '%s' from '%s' overrides existing theme '%s'", theme.Name, filePath, existing.Name)
			}
			m.themes[themeNameLower] = theme
			loadedCount++
		}
	}
	logger.Infof("Loaded %d custom themes.", loadedCount)
	return nil
}

// Current returns the currently active theme.
func (m *Manager) Current() *Theme {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if m.activeTheme == nil {
		// Should have been initialized, but provide ultimate fallback
		return &Theme{Name: "NilFallback", Styles: map[string]tcell.Style{"Default": tcell.StyleDefault}}
	}
	return m.activeTheme
}

// SetTheme sets the active theme by name (case-insensitive).
func (m *Manager) SetTheme(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	nameLower := stringsToLower(name)
	theme, ok := m.themes[nameLower]
	if !ok {
		return fmt.Errorf("theme '%s' not found", name)
	}

	// Only update if actually changed
	if m.activeTheme != theme {
		m.activeTheme = theme
		logger.Infof("Active theme set to: %s", theme.Name)

		// Update the global CurrentTheme reference for backward compatibility
		SetCurrentTheme(theme)
	} else {
		logger.Debugf("Theme '%s' already active, no change needed", name)
	}

	return nil
}

// ListThemes returns the names of all loaded themes.
func (m *Manager) ListThemes() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	names := make([]string, 0, len(m.themes))
	for _, theme := range m.themes {
		names = append(names, theme.Name) // Return original case name
	}
	// Sort names? For consistent listing.
	// sort.Strings(names)
	return names
}

// GetTheme returns a specific theme by name (case-insensitive).
func (m *Manager) GetTheme(name string) (*Theme, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	theme, ok := m.themes[stringsToLower(name)]
	return theme, ok
}
