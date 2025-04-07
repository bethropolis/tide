// internal/plugin/plugin.go
package plugin

import (
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/theme"
	"github.com/bethropolis/tide/internal/types"
	"github.com/gdamore/tcell/v2"
)

// CommandFunc defines the signature for commands registered by plugins.
// It takes arguments (e.g., from user input) and returns an error.
type CommandFunc func(args []string) error

// EditorAPI defines the methods plugins can use to interact with the editor core.
// This acts as a controlled interface, preventing plugins from accessing everything.
type EditorAPI interface {
	// --- Buffer Access (Read-Only Preferred) ---
	// GetBufferContent(start, end types.Position) ([]byte, error) // Get specific range
	GetBufferLines(startLine, endLine int) ([][]byte, error) // Get range of lines
	GetBufferLine(line int) ([]byte, error)                  // Get single line
	GetBufferLineCount() int                                 // Get line count
	GetBufferFilePath() string                               // Get current file path
	IsBufferModified() bool                                  // Check modified status
	GetBufferBytes() []byte

	// --- Buffer Modification ---
	// Use with caution! Ensure plugins don't corrupt state.
	InsertText(pos types.Position, text []byte) error

	DeleteRange(start, end types.Position) error
	// ReplaceRange(start, end types.Position, text []byte) error // Combine delete/insert

	// --- Cursor & Viewport ---
	GetCursor() types.Position
	SetCursor(pos types.Position) // Will clamp and scroll
	GetViewport() (y, x int)      // Get ViewportY, ViewportX
	// SetViewport(y, x int)? // Maybe less common for plugins to directly set viewport

	// --- Event Bus Interaction ---
	DispatchEvent(eventType event.Type, data interface{})
	SubscribeEvent(eventType event.Type, handler event.Handler) // Plugins can listen too

	// --- Command Registration ---
	RegisterCommand(name string, cmdFunc CommandFunc) error // Allow plugins to expose commands

	// --- Status Bar ---
	SetStatusMessage(format string, args ...interface{}) // Show temporary messages

	// --- Theme Access ---
	GetThemeStyle(styleName string) tcell.Style // Get a style from the active theme
	SetTheme(name string) error
	GetTheme() *theme.Theme
	ListThemes() []string

	// --- Configuration (Future) ---
	// GetConfigValue(key string) (interface{}, error)
}

// Plugin defines the interface that all plugins must implement.
type Plugin interface {
	// Name returns the unique identifier name of the plugin.
	Name() string

	// Initialize is called once when the plugin is loaded.
	// It receives the EditorAPI to interact with the core.
	// Used for setup, subscribing to events, registering commands.
	Initialize(api EditorAPI) error

	// Shutdown is called once when the editor is closing.
	// Used for cleanup tasks.
	Shutdown() error

	// HandleEvent (Optional?): Alternatively, plugins subscribe via API.
	// If included, called by plugin manager for relevant events.
	// HandleEvent(e event.Event) bool

	// Commands (Optional?): Alternatively, plugins register commands via API.
	// If included, returns a map of command names to functions.
	// Commands() map[string]CommandFunc
}
