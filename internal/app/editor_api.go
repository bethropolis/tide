// internal/app/editor_api.go
package app

import (
	"fmt" // Keep
	// Keep for now
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/plugin"
	"github.com/bethropolis/tide/internal/types"
	// No longer need direct command types here
)

// Ensure appEditorAPI implements the plugin.EditorAPI interface.
var _ plugin.EditorAPI = (*appEditorAPI)(nil)

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

// --- Status Bar ---

func (api *appEditorAPI) SetStatusMessage(format string, args ...interface{}) {
	api.app.statusBar.SetTemporaryMessage(format, args...) // Delegate to status bar
	api.app.requestRedraw()                                // Ensure redraw to show message
}
