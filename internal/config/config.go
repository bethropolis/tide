// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/bethropolis/tide/internal/logger"
)

// Config holds the application's combined configuration.
type Config struct {
	Logger logger.Config `toml:"logger"` // Embed logger config under [logger] table
	Editor EditorConfig  `toml:"editor"` // Editor-specific settings
	// Add other sections like 'Theme', 'Keymap' later if needed
}

// EditorConfig holds editor-specific settings.
type EditorConfig struct {
	TabWidth        int  `toml:"tab_width"`
	ScrollOff       int  `toml:"scroll_off"`
	SystemClipboard bool `toml:"system_clipboard"`
	StatusBarHeight int  `toml:"status_bar_height"` // Add StatusBarHeight
	// AutoIndent bool `toml:"auto_indent"` // Future setting
}

var (
	loadedConfig *Config
	loadOnce     sync.Once
	loadErr      error
)

// NewDefaultConfig creates a Config struct with default values.
func NewDefaultConfig() *Config {
	return &Config{
		Logger: logger.Config{
			LogLevel:    "info",
			LogFilePath: "", // Empty means default path logic in logger.Init applies
			// Filter lists default to empty/nil
		},
		Editor: EditorConfig{
			TabWidth:        DefaultTabWidth,
			ScrollOff:       DefaultScrollOff,
			SystemClipboard: SystemClipboard,
			StatusBarHeight: StatusBarHeight, // Initialize with the constant value
		},
	}
}

// loadFromFile attempts to load configuration from a TOML file.
// It returns the loaded config and an error (nil if file not found or loaded successfully).
func loadFromFile(filePath string, verbose bool) (*Config, error) {
	cfg := &Config{} // Start empty, we'll merge later
	_, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		if verbose {
			logger.Debugf("Config file not found: %s", filePath)
		}
		return cfg, nil // File not found is not an error here
	}
	if err != nil {
		// Other error stating the file
		return cfg, fmt.Errorf("error checking config file '%s': %w", filePath, err)
	}

	if verbose {
		logger.Debugf("Attempting to load configuration from: %s", filePath)
	}
	metadata, err := toml.DecodeFile(filePath, cfg)
	if err != nil {
		return cfg, fmt.Errorf("failed to parse config file '%s': %w", filePath, err)
	}
	if len(metadata.Undecoded()) > 0 && verbose {
		logger.Warnf("Config file '%s': Unrecognized keys: %v", filePath, metadata.Undecoded())
	}
	if verbose {
		logger.Infof("Successfully loaded configuration from: %s", filePath)
	}
	return cfg, nil
}

// validate checks config values and resets invalid ones to defaults.
func (c *Config) validate() {
	defaults := NewDefaultConfig() // Get defaults for comparison/reset

	// Validate Editor config
	if c.Editor.TabWidth <= 0 {
		c.Editor.TabWidth = defaults.Editor.TabWidth
	}
	if c.Editor.ScrollOff < 0 { // Allow 0
		c.Editor.ScrollOff = defaults.Editor.ScrollOff
	}

	// Validate Logger config
	if c.Logger.LogLevel == "" {
		c.Logger.LogLevel = defaults.Logger.LogLevel
	}

	// Ensure StatusBarHeight has a valid value
	if c.Editor.StatusBarHeight <= 0 {
		c.Editor.StatusBarHeight = defaults.Editor.StatusBarHeight
	}
}

// LoadConfig orchestrates loading defaults, file, applying flags, and validation.
// It should be called only once, typically from main.
func LoadConfig(configFilePath string, flags *Flags) (*Config, error) {
	loadOnce.Do(func() {
		// During initial load, avoid logging as logger isn't initialized yet
		verbose := false

		cfg := NewDefaultConfig() // Start with defaults

		// Determine effective config file path
		effectivePath := configFilePath
		if effectivePath == "" { // If flag not set, try default location
			configDir, err := os.UserConfigDir()
			if err == nil {
				effectivePath = filepath.Join(configDir, AppName, DefaultConfigFileName)
			} else {
				// We can't log this yet as logger isn't initialized
				effectivePath = "" // Cannot load default path
			}
		}

		// Load from file if path is determined
		if effectivePath != "" {
			fileCfg, err := loadFromFile(effectivePath, verbose)
			if err != nil {
				// Store error to return later (can't log yet)
				loadErr = err
			} else if fileCfg != nil {
				// Merge file config settings that are set
				if fileCfg.Logger.LogLevel != "" {
					cfg.Logger = fileCfg.Logger
				}

				// Only apply Editor config if values are valid
				if fileCfg.Editor.TabWidth > 0 {
					cfg.Editor.TabWidth = fileCfg.Editor.TabWidth
				}
				if fileCfg.Editor.ScrollOff >= 0 {
					cfg.Editor.ScrollOff = fileCfg.Editor.ScrollOff
				}
				// Apply boolean values from config file
				cfg.Editor.SystemClipboard = fileCfg.Editor.SystemClipboard
			}
		}

		// Apply flag overrides (if flags were parsed)
		if flags != nil {
			flags.ApplyOverrides(cfg, verbose) // Pass verbose flag here
		}

		// Validate the final merged configuration (no logging during initial load)
		cfg.validate()

		loadedConfig = cfg // Store globally
	})

	return loadedConfig, loadErr
}

// Get returns the loaded application configuration. Panics if LoadConfig wasn't called.
func Get() *Config {
	if loadedConfig == nil {
		// This indicates a programming error - LoadConfig should be called in main.
		panic("config.Get() called before config.LoadConfig()")
	}
	return loadedConfig
}
