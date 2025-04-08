package commands

import (
	"fmt"
	"strings"

	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/plugin"
)

// RegisterAppCommands registers built-in commands like :theme
func RegisterAppCommands(api plugin.EditorAPI, themeAPI ThemeAPI) {
	// Register theme commands
	RegisterThemeCommands(api, themeAPI)

	// Register other app commands here
	// ...
}

// RegisterThemeCommands registers only theme-related commands
func RegisterThemeCommands(api plugin.EditorAPI, themeAPI ThemeAPI) {
	// --- Theme Command ---
	themeCmdFunc := func(args []string) error {
		if len(args) == 0 {
			// Show current theme
			currentTheme := themeAPI.GetTheme()
			themeAPI.SetStatusMessage("Current theme: %s", currentTheme.Name)
			return nil
		}

		themeName := strings.Join(args, " ") // Allow theme names with spaces
		err := themeAPI.SetTheme(themeName)  // API call handles manager update and redraw request
		if err != nil {
			themes := themeAPI.ListThemes()
			themeList := strings.Join(themes, ", ")
			return fmt.Errorf("theme '%s' not found. Available: %s", themeName, themeList)
		}
		themeAPI.SetStatusMessage("Theme set to: %s", themeName)
		return nil
	}

	// --- Theme List Command ---
	themeListCmdFunc := func(args []string) error {
		themes := themeAPI.ListThemes()
		themeList := strings.Join(themes, ", ")
		themeAPI.SetStatusMessage("Available themes: %s", themeList)
		return nil
	}

	// --- Register the commands ---
	err := api.RegisterCommand("theme", themeCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':theme' command: %v", err)
	}
	err = api.RegisterCommand("themes", themeListCmdFunc) // Alias :themes for listing
	if err != nil {
		logger.Warnf("Failed to register ':themes' command: %v", err)
	}
}
