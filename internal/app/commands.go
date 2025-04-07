package app

import (
	"fmt"
	"strings"

	"github.com/bethropolis/tide/internal/logger"
)

// registerAppCommands registers built-in commands like :theme
func registerAppCommands(app *App) {
	api := app.editorAPI // Get the API instance

	// --- Theme Command ---
	themeCmdFunc := func(args []string) error {
		if len(args) == 0 {
			// Show current theme
			currentTheme := app.GetTheme()
			api.SetStatusMessage("Current theme: %s", currentTheme.Name)
			return nil
		}

		themeName := strings.Join(args, " ") // Allow theme names with spaces
		err := api.SetTheme(themeName)       // API call handles manager update and redraw request
		if err != nil {
			themes := api.ListThemes()
			themeList := strings.Join(themes, ", ")
			return fmt.Errorf("theme '%s' not found. Available: %s", themeName, themeList)
		}
		api.SetStatusMessage("Theme set to: %s", themeName)
		return nil
	}

	// --- Theme List Command ---
	themeListCmdFunc := func(args []string) error {
		themes := api.ListThemes()
		themeList := strings.Join(themes, ", ")
		api.SetStatusMessage("Available themes: %s", themeList)
		return nil
	}

	// --- Register the commands ---
	err := api.RegisterCommand("theme", themeCmdFunc)
	if err != nil {
		// Log error - registration should generally succeed unless name reused
		logger.Warnf("Failed to register ':theme' command: %v", err)
	}
	err = api.RegisterCommand("themes", themeListCmdFunc) // Alias :themes for listing
	if err != nil {
		logger.Warnf("Failed to register ':themes' command: %v", err)
	}

	// Additional built-in commands can be registered here
}
