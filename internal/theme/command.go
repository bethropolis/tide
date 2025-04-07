package theme

import (
	"fmt"
	"strings"

	"github.com/bethropolis/tide/internal/logger"
)

// CommandFunc defines the signature for theme commands
type CommandFunc = func(args []string) error

// ThemeAPI defines the minimal interface needed by theme commands
// This breaks the import cycle with plugin package
type ThemeAPI interface {
	// Rename method to avoid collision with plugin.EditorAPI.RegisterCommand
	RegisterThemeCommand(name string, cmdFunc CommandFunc) error
	SetStatusMessage(format string, args ...interface{})
	SetTheme(name string) error
}

// ThemeCommand implements a command for switching themes
type ThemeCommand struct {
	manager *Manager
	api     ThemeAPI
}

// NewThemeCommand creates a new theme switching command
func NewThemeCommand(manager *Manager, api ThemeAPI) *ThemeCommand {
	return &ThemeCommand{
		manager: manager,
		api:     api,
	}
}

// Register registers the theme command with the editor
func (tc *ThemeCommand) Register() error {
	// Register the "theme" command - use the new method name
	err := tc.api.RegisterThemeCommand("theme", tc.themeCommand)
	if err != nil {
		return fmt.Errorf("failed to register theme command: %w", err)
	}

	// Also register as "themelist" (without hyphen) for better compatibility
	err = tc.api.RegisterThemeCommand("themelist", tc.themeListCommand)
	if err != nil {
		return fmt.Errorf("failed to register themelist command: %w", err)
	}

	return nil
}

// themeCommand handles the :theme command
func (tc *ThemeCommand) themeCommand(args []string) error {
	if len(args) == 0 {
		// With no arguments, show the current theme
		currentTheme := tc.manager.Current()
		tc.api.SetStatusMessage("Current theme: %s", currentTheme.Name)
		return nil
	}

	// With an argument, try to set the theme
	themeName := strings.Join(args, " ") // Allow theme names with spaces

	// Use the API's SetTheme method which will:
	// 1. Get the theme from the manager
	// 2. Update the global CurrentTheme
	// 3. Update the app's activeTheme
	// 4. Trigger a redraw
	// 5. Save the theme as default
	err := tc.api.SetTheme(themeName)
	if err != nil {
		// Theme not found, list available themes
		themes := tc.manager.ListThemes()
		themeList := strings.Join(themes, ", ")
		return fmt.Errorf("theme '%s' not found. Available themes: %s", themeName, themeList)
	}

	// Inform the user
	tc.api.SetStatusMessage("Theme set to: %s (saved as default)", themeName)
	logger.Infof("Theme switched to: %s", themeName)

	return nil
}

// themeListCommand handles the :theme-list command
func (tc *ThemeCommand) themeListCommand(args []string) error {
	themes := tc.manager.ListThemes()
	themeList := strings.Join(themes, ", ")
	tc.api.SetStatusMessage("Available themes: %s", themeList)
	return nil
}
