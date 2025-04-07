// internal/theme/theme.go
package theme

import (
	"strings"

	"github.com/bethropolis/tide/internal/logger" // For logging missing styles
	"github.com/gdamore/tcell/v2"
)

// Theme struct definition (remains the same)
type Theme struct {
	Name   string
	IsDark bool
	Styles map[string]tcell.Style
}

// GetStyle method (remains the same - using base name fallback is good)
func (t *Theme) GetStyle(name string) tcell.Style {
	// 1. Try exact name
	if style, ok := t.Styles[name]; ok {
		return style
	}

	// 2. Try base name (part before first dot)
	baseName := name
	if dotIndex := strings.Index(name, "."); dotIndex != -1 {
		baseName = name[:dotIndex]
		if style, ok := t.Styles[baseName]; ok {
			// Log only if the base name is different from the original name
			if baseName != name {
				logger.Debugf("Theme '%s': Style '%s' not found, using base '%s'", t.Name, name, baseName)
			}
			return style
		}
	}

	// 3. Return "Default" style
	if defStyle, ok := t.Styles["Default"]; ok {
		if name != "Default" {
			logger.Debugf("Theme '%s': Style '%s' not found, falling back to 'Default'", t.Name, name)
		}
		return defStyle
	}

	// 4. Absolute fallback
	logger.Warnf("Theme '%s': Style '%s' and 'Default' style not found, using tcell default.", t.Name, name)
	return tcell.StyleDefault
}

// --- DevComfort Dark Theme Definition ---

var DevComfortDark Theme // Define variable for this theme

func init() {
	// --- Palette for DevComfort Dark ---
	dcBackground := tcell.NewHexColor(0x2a2f38) // Slightly muted dark blue/grey (StatusBar BG)
	dcForeground := tcell.NewHexColor(0xc5cdd9) // Soft off-white (Default Text)
	dcComment := tcell.NewHexColor(0x5c6370)    // Muted Grey (Comments, Punctuation)
	dcOrange := tcell.NewHexColor(0xd19a66)     // Muted Orange (Numbers, Constants)
	dcYellow := tcell.NewHexColor(0xe5c07b)     // Soft Yellow (Functions, Attributes)
	dcGreen := tcell.NewHexColor(0x98c379)      // Soft Green (Strings)
	dcCyan := tcell.NewHexColor(0x56b6c2)       // Soft Cyan (Types, Namespaces, Builtins)
	dcBlue := tcell.NewHexColor(0x61afef)       // Soft Blue (Keywords)
	dcMagenta := tcell.NewHexColor(0xc678dd)    // Soft Magenta/Purple (Maybe escapes, specific keywords?)

	// --- Base Style ---
	// Use terminal background, DevComfort foreground
	baseStyle := tcell.StyleDefault.Background(tcell.ColorReset).Foreground(dcForeground)

	// Populate the theme map
	DevComfortDark = Theme{
		Name:   "DevComfort Dark",
		IsDark: true,
		Styles: map[string]tcell.Style{
			// --- UI Elements ---
			"Default":         baseStyle,
			"Selection":       baseStyle.Reverse(true),                                                       // Invert default FG/BG
			"SearchHighlight": tcell.StyleDefault.Background(tcell.ColorOrange).Foreground(tcell.ColorBlack), // Keep high contrast search
			// Status Bar uses the theme background
			"StatusBar":         tcell.StyleDefault.Background(dcBackground).Foreground(dcForeground),
			"StatusBarModified": tcell.StyleDefault.Background(dcBackground).Foreground(dcYellow), // Yellow for modified indicator
			"StatusBarMessage":  tcell.StyleDefault.Background(dcBackground).Foreground(dcForeground).Bold(true),
			"StatusBarFind":     tcell.StyleDefault.Background(dcBackground).Foreground(dcGreen).Bold(true), // Green for find prefix

			// --- Syntax Highlighting ---
			"keyword":   baseStyle.Foreground(dcBlue).Bold(true),      // Soft blue, bold
			"string":    baseStyle.Foreground(dcGreen),                // Soft green
			"comment":   baseStyle.Foreground(dcComment).Italic(true), // Muted grey, italic
			"number":    baseStyle.Foreground(dcOrange),               // Muted orange
			"type":      baseStyle.Foreground(dcCyan),                 // Soft cyan
			"function":  baseStyle.Foreground(dcYellow),               // Soft yellow
			"constant":  baseStyle.Foreground(dcOrange),               // Use number color for constants like true/false/nil
			"variable":  baseStyle.Foreground(dcForeground),           // Default text color
			"operator":  baseStyle.Foreground(dcForeground),           // Default text color (keep readable)
			"namespace": baseStyle.Foreground(dcCyan),                 // Cyan like types
			"label":     baseStyle.Foreground(dcForeground),           // Default FG

			// Specific subtypes (ensure capture names match these if used)
			"string.import":   baseStyle.Foreground(dcGreen),
			"string.escape":   baseStyle.Foreground(dcMagenta),         // Magenta for escapes
			"string.special":  baseStyle.Foreground(dcMagenta),         // For rune literals in Go
			"type.builtin":    baseStyle.Foreground(dcCyan).Bold(true), // Cyan, bold
			"type.definition": baseStyle.Foreground(dcCyan).Bold(true), // For class definitions

			"function.definition": baseStyle.Foreground(dcYellow), //.Bold(true),
			"function.call":       baseStyle.Foreground(dcYellow),
			"function.builtin":    baseStyle.Foreground(dcCyan).Italic(true), // Cyan, italic for builtins like make/len
			"method.call":         baseStyle.Foreground(dcYellow),            // For method calls in Python
			"variable.member":     baseStyle.Foreground(dcForeground),        // Default FG for struct members, less emphasis

			"punctuation":           baseStyle.Foreground(dcComment), // Same as comments, less emphasis
			"punctuation.bracket":   baseStyle.Foreground(dcComment), // Brackets
			"punctuation.delimiter": baseStyle.Foreground(dcComment), // Delimiters like commas, dots

			// Keyword specific variants
			"keyword.function": baseStyle.Foreground(dcBlue).Bold(true), // For def, lambda in Python
			"keyword.type":     baseStyle.Foreground(dcBlue).Bold(true), // For Go type keywords
			"keyword.operator": baseStyle.Foreground(dcBlue),            // For operators that are keywords
			"keyword.storage":  baseStyle.Foreground(dcBlue).Bold(true), // For global, nonlocal in Python

			// Control flow keywords
			"keyword.control":            baseStyle.Foreground(dcBlue).Bold(true), // Generic control
			"keyword.control.flow":       baseStyle.Foreground(dcBlue).Bold(true), // break, continue, pass
			"keyword.control.return":     baseStyle.Foreground(dcBlue).Bold(true), // return statements
			"keyword.control.yield":      baseStyle.Foreground(dcBlue).Bold(true), // yield statements
			"keyword.control.import":     baseStyle.Foreground(dcBlue).Bold(true), // import statements
			"keyword.control.exception":  baseStyle.Foreground(dcBlue).Bold(true), // try, except, raise
			"keyword.control.context":    baseStyle.Foreground(dcBlue).Bold(true), // with statements
			"keyword.control.concurrent": baseStyle.Foreground(dcBlue).Bold(true), // go, select
			"keyword.control.defer":      baseStyle.Foreground(dcBlue).Bold(true), // defer

			// Operator variants
			"operator.logical": baseStyle.Foreground(dcBlue), // and, or, not in Python

			// JavaScript specific styles
			"variable.parameter":          baseStyle.Foreground(dcForeground).Italic(true), // Function parameters
			"variable.builtin":            baseStyle.Foreground(dcCyan),                    // this, arguments, etc.
			"function.method":             baseStyle.Foreground(dcYellow),                  // Method definitions
			"function.method.call":        baseStyle.Foreground(dcYellow),                  // Method calls
			"module":                      baseStyle.Foreground(dcGreen),                   // Module imports/exports
			"module.builtin":              baseStyle.Foreground(dcGreen).Bold(true),        // Built-in modules
			"constructor":                 baseStyle.Foreground(dcYellow).Bold(true),       // Constructor functions
			"attribute":                   baseStyle.Foreground(dcMagenta),                 // Decorators/attributes
			"boolean":                     baseStyle.Foreground(dcOrange),                  // true/false
			"character.special":           baseStyle.Foreground(dcMagenta),                 // Special characters
			"keyword.directive":           baseStyle.Foreground(dcMagenta).Bold(true),      // use strict
			"keyword.import":              baseStyle.Foreground(dcBlue).Bold(true),         // import/export statements
			"keyword.conditional.ternary": baseStyle.Foreground(dcBlue),                    // ? :
			"keyword.coroutine":           baseStyle.Foreground(dcBlue).Bold(true),         // async/await
			"punctuation.special":         baseStyle.Foreground(dcMagenta),                 // Template literal ${

			// Fallbacks (will inherit background from baseStyle)
			"control":     baseStyle.Foreground(dcBlue).Bold(true), // -> keyword
			"builtin":     baseStyle.Foreground(dcCyan),            // -> type/function.builtin
			"import":      baseStyle.Foreground(dcGreen),           // -> string
			"escape":      baseStyle.Foreground(dcMagenta),         // -> string.escape
			"repeat":      baseStyle.Foreground(dcBlue).Bold(true), // -> keyword
			"conditional": baseStyle.Foreground(dcBlue).Bold(true), // -> keyword
			"definition":  baseStyle.Foreground(dcYellow),          // -> function
			"call":        baseStyle.Foreground(dcYellow),          // -> function
			"member":      baseStyle.Foreground(dcForeground),      // -> variable.member
		},
	}

	// Set DevComfortDark as the default theme on init
	CurrentTheme = &DevComfortDark
}

// CurrentTheme, GetCurrentTheme, SetCurrentTheme (remain the same)
var CurrentTheme *Theme

func GetCurrentTheme() *Theme {
	if CurrentTheme == nil {
		// Initialize with the default if called before init finishes? Unlikely.
		if CurrentTheme == nil {
			CurrentTheme = &DevComfortDark // Ensure default theme is set
		}
	}
	return CurrentTheme
}

func SetCurrentTheme(theme *Theme) {
	if theme != nil {
		CurrentTheme = theme
		logger.Infof("Theme switched to: %s", theme.Name)
	}
}
