package commands

import "github.com/bethropolis/tide/internal/theme"

// ThemeAPI extends the commands functionality to support theme operations
type ThemeAPI interface {
	SetTheme(name string) error
	GetTheme() *theme.Theme
	ListThemes() []string
	SetStatusMessage(format string, args ...interface{})
}
