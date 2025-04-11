package app

import (
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
)

// handleCursorMovedForStatus updates the status bar based on cursor position
func (a *App) handleCursorMovedForStatus(e event.Event) bool {
	if data, ok := e.Data.(event.CursorMovedData); ok {
		a.statusBar.SetCursorInfo(data.NewPosition)
	}
	return false // Not consumed
}

// handleBufferModifiedForStatus updates the status bar when buffer is modified
func (a *App) handleBufferModifiedForStatus(e event.Event) bool {
	a.updateStatusBarContent() // Update status bar (e.g., modified indicator)
	// The actual highlight triggering is handled by the other subscriber now
	return false // Not consumed
}

// handleBufferSavedForStatus updates the status bar when buffer is saved
func (a *App) handleBufferSavedForStatus(e event.Event) bool {
	a.updateStatusBarContent() // Update modified status
	return false               // Not consumed
}

// handleBufferLoadedForStatus updates the status and triggers highlighting
func (a *App) handleBufferLoadedForStatus(e event.Event) bool {
	a.updateStatusBarContent() // Update file path, modified status etc.

	// Trigger initial highlight after load
	// This might need adjustment depending on how initial highlighting is handled now
	// If initial highlighting happens synchronously in NewApp/Load, this might
	// only be needed if loading *into* an existing editor instance (e.g., :e command)
	// For now, let's assume initial highlight covers the load case.
	// If async highlighting is desired *after* load, we might need:
	if hm := a.editor.GetHighlightManager(); hm != nil {
		// Maybe trigger a full re-highlight async?
		// hm.AccumulateEdit(types.EditInfo{}) // Or a dedicated method?
		logger.Debugf("App: Buffer loaded, relying on initial sync highlight or subsequent edits.")
	}

	// Editor state might change significantly (cursor, viewport), request redraw
	a.requestRedraw()
	return false // Not consumed
}

// Note: handleBufferModifiedForHighlighting has been replaced by an inline subscription
// in app.go that directly calls the highlight manager's AccumulateEdit method
