// internal/theme/manager.go
package theme

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/gdamore/tcell/v2"
)

// Manager holds loaded themes and manages the active theme.
type Manager struct {
	themes       map[string]*Theme // Map theme name (lowercase) -> Theme object
	activeTheme  *Theme
	themesDir    string
	configDir    string // Store the base config directory
	defaultTheme string // Path to the default theme file
	mutex        sync.RWMutex
	loadError    error // Store error from initial load
}

// NewManager creates and initializes a theme manager.
func NewManager() *Manager {
	mgr := &Manager{
		themes: make(map[string]*Theme),
	}

	// Find config directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		logger.Warnf("Could not find user config dir: %v. Themes cannot be loaded from default location.", err)
		mgr.themesDir = "" // No directory to load from
	} else {
		mgr.configDir = filepath.Join(configDir, "tide")
		mgr.themesDir = filepath.Join(mgr.configDir, "themes")
		mgr.defaultTheme = filepath.Join(mgr.configDir, "theme.toml") // Default theme at config root
	}

	// 1. Load built-in themes first (provides fallbacks)
	mgr.loadBuiltinThemes()

	var loadDirErr error
	// 2. Load themes from directory (if found)
	if mgr.themesDir != "" {
		loadDirErr = mgr.LoadThemesFromDir() // Load custom *.toml files
		if loadDirErr != nil {
			logger.Errorf("Error loading themes from directory '%s': %v", mgr.themesDir, loadDirErr)
			// Continue, but custom themes might be missing
		}
	}

	// 3. Attempt to load the specific default user theme file (now from config root)
	var userDefaultTheme *Theme // Store if loaded successfully
	if mgr.configDir != "" {
		if _, err := os.Stat(mgr.defaultTheme); err == nil {
			// File exists, try loading it
			logger.Infof("Found default user theme file: %s", mgr.defaultTheme)
			theme, loadErr := LoadThemeFromFile(mgr.defaultTheme)
			if loadErr != nil {
				logger.Warnf("Failed to load default theme file '%s': %v", mgr.defaultTheme, loadErr)
				// Store the error if needed, but don't overwrite loadDirErr yet
				if mgr.loadError == nil {
					mgr.loadError = loadErr
				}
			} else {
				// Successfully loaded theme.toml
				userDefaultTheme = theme // Mark this as the preferred theme
				// Add/overwrite it in the map, ensuring priority
				themeNameLower := stringsToLower(theme.Name)
				if existing, ok := mgr.themes[themeNameLower]; ok {
					logger.Infof("Default theme file ('%s') defines theme '%s', overriding previous definition from '%s'",
						mgr.defaultTheme, theme.Name, existing.Name)
				} else {
					logger.Infof("Loaded theme '%s' from default file '%s'", theme.Name, mgr.defaultTheme)
				}
				mgr.themes[themeNameLower] = theme
			}
		} else if !os.IsNotExist(err) {
			// Error stating the file, other than not existing
			logger.Warnf("Error checking for default theme file '%s': %v", mgr.defaultTheme, err)
			if mgr.loadError == nil {
				mgr.loadError = err
			}
		} else {
			logger.Debugf("Default user theme file not found: %s", mgr.defaultTheme)
		}
	}
	// Assign final overall load error if one occurred
	if loadDirErr != nil && mgr.loadError == nil {
		mgr.loadError = loadDirErr
	}

	// 4. Set initial active theme with priority
	var initialThemeSet bool
	// Priority 1: Use the theme loaded from theme.toml if successful
	if userDefaultTheme != nil {
		mgr.activeTheme = userDefaultTheme
		initialThemeSet = true
		logger.Infof("Setting active theme from default user file: %s", userDefaultTheme.Name)
	}

	// Priority 2: Fallback to preferred built-in (e.g., DevComfort) if not set yet
	if !initialThemeSet {
		preferredBuiltInName := "devcomfort dark" // lowercase
		if theme, ok := mgr.themes[preferredBuiltInName]; ok {
			mgr.activeTheme = theme
			initialThemeSet = true
			logger.Infof("Setting active theme to preferred built-in: %s", theme.Name)
		}
	}

	// Priority 3: Fallback to the first theme found if still not set
	if !initialThemeSet && len(mgr.themes) > 0 {
		for _, t := range mgr.themes { // Iteration order isn't guaranteed, but it's a fallback
			mgr.activeTheme = t
			initialThemeSet = true
			logger.Infof("Setting active theme to first available: %s", t.Name)
			break
		}
	}

	// Priority 4: Failsafe if absolutely no themes loaded
	if !initialThemeSet {
		logger.Errorf("No themes loaded successfully, using failsafe theme!")
		mgr.activeTheme = &Theme{
			Name: "Failsafe",
			Styles: map[string]tcell.Style{
				"Default": tcell.StyleDefault,
			},
		}
	}

	// Ensure global CurrentTheme reflects the manager's choice (for any code still using it)
	SetCurrentTheme(mgr.activeTheme) // Updates the global variable

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

	// Ensure directory exists, CREATE if not found
	if _, err := os.Stat(m.themesDir); os.IsNotExist(err) {
		logger.Infof("Theme directory '%s' does not exist. Creating directory.", m.themesDir)
		if err := os.MkdirAll(m.themesDir, 0755); err != nil { // Use MkdirAll
			return fmt.Errorf("failed to create theme dir '%s': %w", m.themesDir, err)
		}
		return nil // Directory created, no themes to load yet
	} else if err != nil {
		// Error stating the directory other than not existing
		return fmt.Errorf("error checking theme directory '%s': %w", m.themesDir, err)
	}

	logger.Infof("Loading themes from: %s", m.themesDir)
	files, err := os.ReadDir(m.themesDir)
	if err != nil {
		return fmt.Errorf("failed to read theme directory '%s': %w", m.themesDir, err)
	}

	loadedCount := 0
	for _, file := range files {
		fileNameLower := stringsToLower(file.Name())
		// No longer skip theme.toml - we treat all .toml files as themes
		if !file.IsDir() && strings.HasSuffix(fileNameLower, ".toml") {
			filePath := filepath.Join(m.themesDir, file.Name())
			theme, err := LoadThemeFromFile(filePath) // Use the loader
			if err != nil {
				logger.Warnf("Failed to load theme from '%s': %v", filePath, err)
				continue // Skip problematic file
			}

			themeNameLower := stringsToLower(theme.Name)
			if existing, ok := m.themes[themeNameLower]; ok {
				// Don't warn if overriding built-in, only if overriding another file
				// This check is tricky. For now, let later loads win.
				logger.Debugf("Theme '%s' from '%s' potentially overrides existing theme '%s'", theme.Name, filePath, existing.Name)
			}
			m.themes[themeNameLower] = theme
			loadedCount++
		}
	}
	logger.Infof("Loaded %d custom themes from directory scan.", loadedCount)
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

		// Save the current theme as the default
		if err := m.saveCurrentThemeAsDefault(); err != nil {
			logger.Warnf("Failed to save theme '%s' as default: %v", theme.Name, err)
			// Continue anyway - the theme change was successful
		}
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

// ThemeToToml converts a Theme to a TomlTheme for saving
func themeToToml(theme *Theme) TomlTheme {
	tomlTheme := TomlTheme{
		Name:   theme.Name,
		IsDark: theme.IsDark,
		Styles: make(map[string]TomlStyleDef),
	}

	// Convert each style to a TomlStyleDef
	for name, style := range theme.Styles {
		fg, bg, attrs := style.Decompose()

		// Convert colors to hex strings
		fgStr := colorToHexString(fg)
		bgStr := colorToHexString(bg)

		// Extract style attributes
		bold := (attrs & tcell.AttrBold) != 0
		italic := (attrs & tcell.AttrItalic) != 0
		underline := (attrs & tcell.AttrUnderline) != 0
		reverse := (attrs & tcell.AttrReverse) != 0

		// Create TomlStyleDef with pointers for optional fields
		tomlStyle := TomlStyleDef{}
		if fgStr != "" {
			tomlStyle.Fg = &fgStr
		}
		if bgStr != "" {
			tomlStyle.Bg = &bgStr
		}

		// Only set attributes that are true
		if bold {
			tomlStyle.Bold = &bold
		}
		if italic {
			tomlStyle.Italic = &italic
		}
		if underline {
			tomlStyle.Underline = &underline
		}
		if reverse {
			tomlStyle.Reverse = &reverse
		}

		tomlTheme.Styles[name] = tomlStyle
	}

	return tomlTheme
}

// colorToHexString converts a tcell.Color to a hex string "#RRGGBB"
func colorToHexString(color tcell.Color) string {
	if color == tcell.ColorDefault {
		return "default"
	}
	if color == tcell.ColorReset {
		return "reset"
	}

	r, g, b := color.RGB()
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// SaveThemeToFile saves a theme to a TOML file in the themes directory
func (m *Manager) SaveThemeToFile(themeName, fileName string) error {
	m.mutex.RLock()
	theme, ok := m.themes[stringsToLower(themeName)]
	m.mutex.RUnlock()

	if !ok {
		return fmt.Errorf("theme '%s' not found", themeName)
	}

	// Ensure themes directory exists
	if err := os.MkdirAll(m.themesDir, 0755); err != nil {
		return fmt.Errorf("failed to create themes directory: %w", err)
	}

	// Convert theme to TOML-compatible structure
	tomlTheme := themeToToml(theme)

	// Create the file
	filePath := filepath.Join(m.themesDir, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create theme file '%s': %w", filePath, err)
	}
	defer file.Close()

	// Write the TOML content
	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(tomlTheme); err != nil {
		return fmt.Errorf("failed to encode theme to TOML: %w", err)
	}

	logger.Infof("Theme '%s' saved to file '%s'", themeName, filePath)
	return nil
}

// saveCurrentThemeAsDefault saves the current theme as theme.toml in the config dir
func (m *Manager) saveCurrentThemeAsDefault() error {
	if m.activeTheme == nil {
		return fmt.Errorf("no active theme to save")
	}

	// Ensure config directory exists
	if err := os.MkdirAll(m.configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Convert theme to TOML-compatible structure
	tomlTheme := themeToToml(m.activeTheme)

	// Create the file
	file, err := os.Create(m.defaultTheme)
	if err != nil {
		return fmt.Errorf("failed to create default theme file '%s': %w", m.defaultTheme, err)
	}
	defer file.Close()

	// Write the TOML content
	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(tomlTheme); err != nil {
		return fmt.Errorf("failed to encode theme to TOML: %w", err)
	}

	logger.Infof("Default theme set to '%s' in file '%s'", m.activeTheme.Name, m.defaultTheme)
	return nil
}

// SaveCurrentThemeAsDefault saves the current theme as theme.toml (public version)
func (m *Manager) SaveCurrentThemeAsDefault() error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.saveCurrentThemeAsDefault()
}

// WatchForChanges sets up filesystem monitoring for theme directory changes
// This would allow real-time reloading of themes when files change
func (m *Manager) WatchForChanges() error {
	// This would use a filesystem watcher like fsnotify
	// When a .toml file changes, reload it and update the themes map

	// For now, just return a not implemented error
	return fmt.Errorf("theme hot-reloading is not yet implemented")
}
