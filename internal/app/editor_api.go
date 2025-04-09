// internal/app/editor_api.go
package app

import (
	"fmt" // Keep
	// Keep for now
	"github.com/bethropolis/tide/internal/commands"
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/plugin"
	"github.com/bethropolis/tide/internal/theme"
	"github.com/bethropolis/tide/internal/types"
	"github.com/gdamore/tcell/v2"
)

// Ensure appEditorAPI implements the plugin.EditorAPI interface.
var _ plugin.EditorAPI = (*appEditorAPI)(nil)

// Verify that appEditorAPI implements the theme.ThemeAPI interface
var _ theme.ThemeAPI = (*appEditorAPI)(nil)

// Add verification for commands.ThemeAPI interface
var _ commands.ThemeAPI = (*appEditorAPI)(nil)

// appEditorAPI provides the concrete implementation of the EditorAPI interface.
type appEditorAPI struct {
	app *App // Reference back to the main application
}

// newEditorAPI creates a new API adapter instance.
// Changed from NewEditorAPI to be unexported, as it's internal to app package.
func newEditorAPI(app *App) *appEditorAPI {
	return &appEditorAPI{app: app}
}

// --- Buffer Access ---

func (api *appEditorAPI) GetBufferLines(startLine, endLine int) ([][]byte, error) {
	// TODO: Implement range checking and slicing logic on app.editor.GetBuffer()
	// For now, return all lines as a placeholder
	return api.app.editor.GetBuffer().Lines(), nil
}

func (api *appEditorAPI) GetBufferLine(line int) ([]byte, error) {
	return api.app.editor.GetBuffer().Line(line)
}

func (api *appEditorAPI) GetBufferLineCount() int {
	return api.app.editor.GetBuffer().LineCount()
}

func (api *appEditorAPI) GetBufferFilePath() string {
	return api.app.editor.GetBuffer().FilePath()
}

func (api *appEditorAPI) IsBufferModified() bool {
	return api.app.editor.GetBuffer().IsModified()
}

func (api *appEditorAPI) GetBufferBytes() []byte {
	return api.app.editor.GetBuffer().Bytes() // Delegate to buffer's Bytes()
}

// --- Buffer Modification ---

func (api *appEditorAPI) InsertText(pos types.Position, text []byte) error {
	editInfo, err := api.app.editor.GetBuffer().Insert(pos, text) // Capture EditInfo
	if err == nil {
		api.app.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
		// We likely need to redraw if buffer changes via plugin
		api.app.requestRedraw()
	}
	// TODO: Does the cursor need updating? Does the editor method handle it?
	return err
}

func (api *appEditorAPI) DeleteRange(start, end types.Position) error {
	// Similar delegation and considerations as InsertText
	editInfo, err := api.app.editor.GetBuffer().Delete(start, end) // Capture EditInfo
	if err == nil {
		api.app.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
		api.app.requestRedraw()
	}
	// TODO: Cursor update?
	return err
}

// Replace implements the Replace method for substitution command
func (api *appEditorAPI) Replace(pattern, replacement string, global bool) (int, error) {
	// Delegate to editor's Replace method which delegates to findManager.Replace
	return api.app.editor.Replace(pattern, replacement, global)
}

// --- Cursor & Viewport ---

func (api *appEditorAPI) GetCursor() types.Position {
	return api.app.editor.GetCursor()
}

func (api *appEditorAPI) SetCursor(pos types.Position) {
	api.app.editor.SetCursor(pos) // Editor's method handles clamping/scrolling/events
	api.app.requestRedraw()       // Ensure redraw happens after API call sets cursor
}

func (api *appEditorAPI) GetViewport() (int, int) {
	return api.app.editor.GetViewport()
}

// --- Event Bus Interaction ---

func (api *appEditorAPI) DispatchEvent(eventType event.Type, data interface{}) {
	api.app.eventManager.Dispatch(eventType, data) // Delegate to app's manager
}

func (api *appEditorAPI) SubscribeEvent(eventType event.Type, handler event.Handler) {
	api.app.eventManager.Subscribe(eventType, handler) // Delegate to app's manager
}

// --- Command Registration ---

func (api *appEditorAPI) RegisterCommand(name string, cmdFunc plugin.CommandFunc) error {
	// Get the mode handler via the app reference and delegate
	if api.app == nil || api.app.GetModeHandler() == nil {
		// This would be a programming error during setup
		logger.Debugf("ERROR: appEditorAPI cannot register command '%s', app or modeHandler is nil", name)
		return fmt.Errorf("internal error: API cannot access command registration")
	}
	return api.app.GetModeHandler().RegisterCommand(name, cmdFunc) // <<< DELEGATE
}

// RegisterThemeCommand implements the theme.ThemeAPI interface
func (api *appEditorAPI) RegisterThemeCommand(name string, cmdFunc theme.CommandFunc) error {
	// Since theme.CommandFunc is a type alias for func([]string) error,
	// we can delegate to the ModeHandler's RegisterCommand directly
	if api.app == nil || api.app.GetModeHandler() == nil {
		logger.Debugf("ERROR: appEditorAPI cannot register theme command '%s', app or modeHandler is nil", name)
		return fmt.Errorf("internal error: API cannot access command registration")
	}
	return api.app.GetModeHandler().RegisterCommand(name, cmdFunc)
}

// --- Status Bar ---

func (api *appEditorAPI) SetStatusMessage(format string, args ...interface{}) {
	api.app.statusBar.SetTemporaryMessage(format, args...) // Delegate to status bar
	api.app.requestRedraw()                                // Ensure redraw to show message
}

// --- Theme Access ---
func (api *appEditorAPI) GetThemeStyle(styleName string) tcell.Style {
	return api.app.activeTheme.GetStyle(styleName)
}

// SetTheme sets the active theme by name
func (api *appEditorAPI) SetTheme(name string) error {
	theme, ok := api.app.GetThemeManager().GetTheme(name)
	if !ok {
		return fmt.Errorf("theme '%s' not found", name)
	}

	// Set the theme in the app (which updates both the app's activeTheme and the global reference)
	api.app.SetTheme(theme)

	// Explicitly request a redraw to show the theme change immediately
	api.app.requestRedraw()

	logger.Debugf("Theme changed to '%s', redraw requested", name)
	return nil
}

// GetTheme returns the current active theme
func (api *appEditorAPI) GetTheme() *theme.Theme {
	return api.app.GetTheme()
}

// ListThemes returns a list of all available theme names
func (api *appEditorAPI) ListThemes() []string {
	return api.app.GetThemeManager().ListThemes()
}

// SaveBuffer saves the current buffer to disk, optionally to a new filename
func (api *appEditorAPI) SaveBuffer(filePath ...string) error {
	// Delegate to editor method, passing arguments through
	return api.app.editor.SaveBuffer(filePath...)
}

// RequestQuit signals the application to quit
func (api *appEditorAPI) RequestQuit(force bool) {
	if force {
		logger.Debugf("API: Force quit requested.")
		close(api.app.quit) // Close directly if forced
	} else {
		// Check modified status via buffer
		if api.app.editor.GetBuffer().IsModified() {
			logger.Debugf("API: Quit requested, but buffer modified. Setting status.")
			api.SetStatusMessage("No write since last change (use :q! or force quit key)")
			// Don't close the channel here. Let the command fail.
		} else {
			logger.Debugf("API: Quit requested (buffer not modified).")
			close(api.app.quit)
		}
	}
}
