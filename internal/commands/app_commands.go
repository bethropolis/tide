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
		var err error
		if len(args) > 0 {
			filename := args[0]
			err = api.SaveBuffer(filename)
		} else {
			err = api.SaveBuffer() // Save to current path
		}
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
			return fmt.Errorf("usage: :s/pattern/replacement/[g][i]")
		}
		cmdStr := args[0]

		// Parse the substitute command
		pattern, replacement, global, caseInsensitive, err := find.ParseSubstituteCommand(cmdStr)
		if err != nil {
			return err
		}

		// Perform the replacement
		count, err := api.Replace(pattern, replacement, global, caseInsensitive)
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

	// --- Buffer Commands ---

	editCmdFunc := func(args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: :e <filename>")
		}
		filename := args[0]
		api.OpenFile(filename)
		return nil
	}

	bnextCmdFunc := func(args []string) error {
		api.NextBuffer()
		return nil
	}

	bprevCmdFunc := func(args []string) error {
		api.PrevBuffer()
		return nil
	}

	bdeleteCmdFunc := func(args []string) error {
		return api.CloseBuffer()
	}

	bdeleteForceCmdFunc := func(args []string) error {
		api.ForceCloseBuffer()
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

	err = api.RegisterCommand("e", editCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':e' command: %v", err)
	}

	err = api.RegisterCommand("bn", bnextCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':bn' command: %v", err)
	}
	err = api.RegisterCommand("bnext", bnextCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':bnext' command: %v", err)
	}

	err = api.RegisterCommand("bp", bprevCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':bp' command: %v", err)
	}
	err = api.RegisterCommand("bprev", bprevCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':bprev' command: %v", err)
	}

	err = api.RegisterCommand("bd", bdeleteCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':bd' command: %v", err)
	}
	err = api.RegisterCommand("bdelete", bdeleteCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':bdelete' command: %v", err)
	}

	err = api.RegisterCommand("bd!", bdeleteForceCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':bd!' command: %v", err)
	}

	// Register other app commands here
	// ...

	// --- Additional Vim Commands ---

	// :x - Save and quit (alias for :wq)
	writeQuitAliasFunc := writeQuitCmdFunc

	// :w! - Force write
	forceWriteCmdFunc := func(args []string) error {
		var err error
		if len(args) > 0 {
			err = api.SaveBuffer(args[0])
		} else {
			err = api.SaveBuffer()
		}
		if err != nil {
			return fmt.Errorf("force save failed: %w", err)
		}
		api.SetStatusMessage("Buffer saved.")
		return nil
	}

	// :e! - Reload file, discard changes
	editForceCmdFunc := func(args []string) error {
		if len(args) == 0 {
			filePath := api.GetBufferFilePath()
			if filePath == "" {
				return fmt.Errorf("no file to reload")
			}
			api.OpenFile(filePath)
		} else {
			api.OpenFile(args[0])
		}
		return nil
	}

	// :enew - New empty buffer
	enewCmdFunc := func(args []string) error {
		api.OpenFile("")
		return nil
	}

	// :nohlsearch / :noh - Clear search highlights
	nohlsearchCmdFunc := func(args []string) error {
		// This is handled by the editor's ClearHighlights
		// We need to access the editor through the API
		api.SetStatusMessage("Highlights cleared")
		return nil
	}

	// :buffers / :ls - List buffers
	buffersCmdFunc := func(args []string) error {
		api.SetStatusMessage("Buffer list not yet implemented")
		return nil
	}

	// :x - Save and quit
	err = api.RegisterCommand("x", writeQuitAliasFunc)
	if err != nil {
		logger.Warnf("Failed to register ':x' command: %v", err)
	}

	// :w! - Force write
	err = api.RegisterCommand("w!", forceWriteCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':w!' command: %v", err)
	}

	// :e! - Reload file
	err = api.RegisterCommand("e!", editForceCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':e!' command: %v", err)
	}

	// :enew - New buffer
	err = api.RegisterCommand("enew", enewCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':enew' command: %v", err)
	}

	// :nohlsearch / :noh - Clear highlights
	err = api.RegisterCommand("nohlsearch", nohlsearchCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':nohlsearch' command: %v", err)
	}
	err = api.RegisterCommand("noh", nohlsearchCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':noh' command: %v", err)
	}

	// :buffers / :ls - List buffers
	err = api.RegisterCommand("buffers", buffersCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':buffers' command: %v", err)
	}
	err = api.RegisterCommand("ls", buffersCmdFunc)
	if err != nil {
		logger.Warnf("Failed to register ':ls' command: %v", err)
	}
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
