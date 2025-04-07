// internal/event/events.go
package event

import (
	"github.com/bethropolis/tide/internal/types" // Assuming types holds Position
	"github.com/gdamore/tcell/v2"
)

// Type identifies the kind of event.
type Type int

// Define specific event types.
const (
	TypeUnknown Type = iota

	// Core Editor Events
	TypeBufferModified // Fired when buffer content changes (insert/delete)
	TypeBufferLoaded   // Fired after a buffer is successfully loaded
	TypeBufferSaved    // Fired after a buffer is successfully saved
	TypeCursorMoved    // Fired when the cursor position changes
	TypeModeChanged    // Fired when editor mode changes (e.g., Normal -> Insert) - Future

	// Input Events (potentially useful for plugins reacting to raw keys)
	TypeKeyPressed // Raw key press event forwarded

	// Application Lifecycle Events
	TypeAppReady // Fired when the application is fully initialized
	TypeAppQuit  // Fired just before application termination begins

	// Plugin specific events can be defined later or use custom data

	TypeThemeChanged // Fired when the theme is changed
)

// Event is the structure passed through the event bus.
type Event struct {
	Type Type        // The kind of event
	Data interface{} // Payload carrying event-specific data
	// Timestamp time.Time // Optional: Add timestamp if needed
}

// --- Specific Event Data Structures ---
// (Define structs for data associated with specific event types)

// BufferModifiedData contains info about buffer changes, including EditInfo.
type BufferModifiedData struct {
	Edit types.EditInfo // Information about the change for incremental parsing
}

// BufferLoadedData contains info about the loaded buffer.
type BufferLoadedData struct {
	FilePath string
	// Could add Buffer reference, but might create coupling issues
}

// BufferSavedData contains info about the saved buffer.
type BufferSavedData struct {
	FilePath string
}

// CursorMovedData contains the new cursor position.
type CursorMovedData struct {
	NewPosition types.Position
}

// KeyPressedData contains the raw tcell key event.
type KeyPressedData struct {
	KeyEvent *tcell.EventKey
}

// AppQuitData could contain exit code or reason later.
type AppQuitData struct{}

// AppReadyData could contain initial config or state later.
type AppReadyData struct{}
