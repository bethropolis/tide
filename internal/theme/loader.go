// internal/theme/loader.go
package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/gdamore/tcell/v2"
)

// TomlStyleDef represents a single style definition in the TOML file
type TomlStyleDef struct {
	Fg        *string `toml:"fg"` // Use pointers to detect missing values
	Bg        *string `toml:"bg"`
	Bold      *bool   `toml:"bold"`
	Italic    *bool   `toml:"italic"`
	Underline *bool   `toml:"underline"`
	Reverse   *bool   `toml:"reverse"`
	// Add other attributes like Blink, Dim if needed
}

// TomlTheme represents the structure of a theme file
type TomlTheme struct {
	Name   string                  `toml:"name"`
	IsDark bool                    `toml:"is_dark"` // Default is false if missing
	Styles map[string]TomlStyleDef `toml:"styles"`
}

// LoadThemeFromFile parses a TOML file and converts it to a Theme object
func LoadThemeFromFile(filePath string) (*Theme, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read theme file '%s': %w", filePath, err)
	}

	var tomlTheme TomlTheme
	metadata, err := toml.Decode(string(data), &tomlTheme)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TOML theme file '%s': %w", filePath, err)
	}

	// Check for undedecoded keys (potential typos in theme file)
	if len(metadata.Undecoded()) > 0 {
		logger.Warnf("Theme '%s': Unrecognized keys in file '%s': %v", tomlTheme.Name, filePath, metadata.Undecoded())
	}

	if tomlTheme.Name == "" {
		// Use filename as fallback name if not specified
		tomlTheme.Name = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
		logger.Debugf("Theme file '%s' missing 'name', using filename '%s'", filePath, tomlTheme.Name)
	}

	// Convert to internal Theme struct
	theme := &Theme{
		Name:   tomlTheme.Name,
		IsDark: tomlTheme.IsDark,
		Styles: make(map[string]tcell.Style),
	}

	// Get the base default style to inherit from if defined, otherwise use tcell's default
	baseStyle := tcell.StyleDefault
	if defaultTomlStyle, ok := tomlTheme.Styles["Default"]; ok {
		var parseErr error
		baseStyle, parseErr = convertTomlStyle(defaultTomlStyle, tcell.StyleDefault) // Base inherits from tcell default
		if parseErr != nil {
			logger.Warnf("Theme '%s': Failed to parse 'Default' style, using tcell default as base: %v", theme.Name, parseErr)
			baseStyle = tcell.StyleDefault
		}
	}
	theme.Styles["Default"] = baseStyle // Ensure Default style is set

	// Convert other styles, inheriting from the theme's Default style
	for name, tomlStyle := range tomlTheme.Styles {
		if name == "Default" {
			continue // Already processed
		}
		style, err := convertTomlStyle(tomlStyle, baseStyle) // Inherit from theme's Default
		if err != nil {
			logger.Warnf("Theme '%s': Failed to parse style '%s', skipping: %v", theme.Name, name, err)
			continue
		}
		theme.Styles[name] = style
	}

	logger.Debugf("Successfully loaded theme '%s' from '%s'", theme.Name, filePath)
	return theme, nil
}

// convertTomlStyle converts the TOML definition to a tcell.Style, inheriting from a base
func convertTomlStyle(tomlStyle TomlStyleDef, baseStyle tcell.Style) (tcell.Style, error) {
	style := baseStyle // Start with the base style to inherit unset properties

	// Apply foreground color
	if tomlStyle.Fg != nil {
		color, err := parseColorString(*tomlStyle.Fg)
		if err != nil {
			return style, fmt.Errorf("invalid foreground color '%s': %w", *tomlStyle.Fg, err)
		}
		style = style.Foreground(color)
	}

	// Apply background color
	if tomlStyle.Bg != nil {
		color, err := parseColorString(*tomlStyle.Bg)
		if err != nil {
			return style, fmt.Errorf("invalid background color '%s': %w", *tomlStyle.Bg, err)
		}
		style = style.Background(color)
	}

	// Apply attributes
	if tomlStyle.Bold != nil {
		style = style.Bold(*tomlStyle.Bold)
	}
	if tomlStyle.Italic != nil {
		style = style.Italic(*tomlStyle.Italic)
	}
	if tomlStyle.Underline != nil {
		style = style.Underline(*tomlStyle.Underline)
	}
	if tomlStyle.Reverse != nil {
		// Handle reverse specially - often means swap default FG/BG
		// Tcell's Reverse swaps current FG/BG, which might not be intended
		// If reverse=true, maybe explicitly set FG=baseBG, BG=baseFG?
		// For simplicity now, just use tcell's Reverse.
		style = style.Reverse(*tomlStyle.Reverse)
	}

	return style, nil
}

// parseColorString converts hex codes or named colors (future) to tcell.Color
func parseColorString(s string) (tcell.Color, error) {
	s = stringsToLower(strings.TrimSpace(s))
	if strings.HasPrefix(s, "#") {
		if len(s) != 7 {
			return tcell.ColorDefault, fmt.Errorf("invalid hex color format '%s', must be #RRGGBB", s)
		}
		val, err := strconv.ParseInt(s[1:], 16, 32)
		if err != nil {
			return tcell.ColorDefault, fmt.Errorf("invalid hex value '%s': %w", s, err)
		}
		return tcell.NewHexColor(int32(val)), nil
	}

	// TODO: Add support for named colors (e.g., "red", "blue") by mapping to tcell.ColorX?
	// Or special keywords like "reset", "default"?

	// Handle "reset" keyword to mean tcell.ColorReset
	if s == "reset" {
		return tcell.ColorReset, nil
	}
    // Handle "default" keyword to mean tcell.ColorDefault
	if s == "default" {
		return tcell.ColorDefault, nil
	}


	return tcell.ColorDefault, fmt.Errorf("unknown color format or name '%s'", s)
}

// Helper for case-insensitive string ops if needed later
func stringsToLower(s string) string {
	// This is basic, might need full Unicode lowercasing if handling complex names
	return strings.ToLower(s)
}