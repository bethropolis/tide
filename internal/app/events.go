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
	return false
}

// handleBufferModifiedForStatus updates the status bar when buffer is modified
func (a *App) handleBufferModifiedForStatus(e event.Event) bool {
	a.updateStatusBarContent()
	return false
}

// handleBufferSavedForStatus updates the status bar when buffer is saved
func (a *App) handleBufferSavedForStatus(e event.Event) bool {
	a.updateStatusBarContent()
	return false
}

// handleBufferLoadedForStatus updates the status and triggers highlighting
func (a *App) handleBufferLoadedForStatus(e event.Event) bool {
	a.updateStatusBarContent()
	a.editor.TriggerSyntaxHighlight() // Re-highlight on load
	a.requestRedraw()                 // Request redraw after potential highlight changes
	return false
}

// handleBufferModifiedForHighlighting processes buffer modification events
func (a *App) handleBufferModifiedForHighlighting(e event.Event) bool {
	if data, ok := e.Data.(event.BufferModifiedData); ok {
		// Buffer was modified, accumulate the edit info
		logger.Debugf("App: Buffer modified event received, accumulating edit info.")
		a.highlightingManager.AccumulateEdit(data.Edit)
	} else {
		logger.Warnf("App: Received BufferModified event with unexpected data type: %T", e.Data)
		logger.Debugf("App: Falling back to non-incremental highlighting due to missing edit info")
	}
	return false // Allow other handlers for BufferModified to run
}
