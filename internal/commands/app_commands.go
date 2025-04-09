package commands

import (
	"fmt"
	"strings"

	"github.com/bethropolis/tide/internal/core/find"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/plugin"
)

// RegisterAppCommands registers built-in commands like :theme
func RegisterAppCommands(api plugin.EditorAPI, themeAPI ThemeAPI) {
	// Register theme commands
	RegisterThemeCommands(api, themeAPI)

	// --- Core File/Quit Commands ---

	// :w [filename] - Write buffer to file
	writeCmdFunc := func(args []string) error {
		// TODO: Handle optional filename argument `args[0]`
		// For now, just save to current path
		if len(args) > 0 {
			return fmt.Errorf(":w with filename not implemented yet")
		}
		err := api.SaveBuffer() // API method calls editor.SaveBuffer
		if err != nil {
			return fmt.Errorf("failed to save buffer: %w", err) // Return error to show in status
		}
		api.SetStatusMessage("Buffer saved successfully.") // Show success
		return nil
	}

	// :q - Quit if not modified
	quitCmdFunc := func(args []string) error {
		if api.IsBufferModified() {
			// Return an error, ModeHandler will display it via status bar
			return fmt.Errorf("no write since last change (use :q! to override)")
		}
		// Buffer not modified, request normal quit
		api.RequestQuit(false) // Signal App to quit
		return nil             // Return nil, quit signal is sent
	}

	// :wq - Write and Quit
	writeQuitCmdFunc := func(args []string) error {
		err := api.SaveBuffer()
		if err != nil {
			return fmt.Errorf("save failed, not quitting: %w", err) // Report save error
		}
		// Save successful, request normal quit
		api.SetStatusMessage("Buffer saved successfully.") // Show success before quit signal
		api.RequestQuit(false)                             // Signal App to quit
		return nil
	}

	// :q! - Force Quit
	forceQuitCmdFunc := func(args []string) error {
		api.RequestQuit(true) // Signal App to force quit
		return nil            // Return nil, quit signal is sent
	}

	// --- :s substitution command ---
	substituteCmdFunc := func(args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("usage: :s/pattern/replacement/[g]")
		}
		cmdStr := args[0]

		// Parse the substitute command
		pattern, replacement, global, err := find.ParseSubstituteCommand(cmdStr)
		if err != nil {
			return err // Return parsing error
		}

		// Perform the replacement
		count, err := api.Replace(pattern, replacement, global)
		if err != nil {
			return fmt.Errorf("replace failed: %w", err)
		}

		if count == 0 {
			api.SetStatusMessage("Pattern not found: %s", pattern)
		} else {
			api.SetStatusMessage("Replaced %d occurrence(s)", count)
		}
		return nil
	}

	// Register commands
	err := api.RegisterCommand("w", writeCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':w' command: %v", err)
	}

	err = api.RegisterCommand("q", quitCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':q' command: %v", err)
	}

	err = api.RegisterCommand("wq", writeQuitCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':wq' command: %v", err)
	}

	err = api.RegisterCommand("q!", forceQuitCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':q!' command: %v", err)
	}

	err = api.RegisterCommand("s", substituteCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':s' command: %v", err)
	}

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
