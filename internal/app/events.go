package app

import (
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
)

// handleCursorMovedForStatus updates the status bar based on cursor position
func (a *App) handleCursorMovedForStatus(e event.Event) bool {
	if data, ok := e.Data.(event.CursorMovedData); ok {
		a.statusBar.SetCursorInfo(data.NewPosition)

		ed := a.getActiveEditor()
		// If a selection is active, all highlighted rows may change; force full redraw.
		// Otherwise mark both the line the cursor left and the line it moved to so
		// that the previous cursor position is cleared and the new one is painted.
		if _, _, selActive := ed.GetSelection(); selActive {
			ed.MarkAllDirty()
		} else {
			ed.MarkDirty(data.OldPosition.Line)
			ed.MarkDirty(data.NewPosition.Line)
		}
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

	if hm := a.getActiveEditor().GetHighlightManager(); hm != nil {
		logger.Debugf("App: Buffer loaded, relying on initial sync highlight or subsequent edits.")
	}

	// A full buffer load means everything on screen may have changed.
	a.getActiveEditor().MarkAllDirty()
	a.requestRedraw()
	return false // Not consumed
}

// Note: handleBufferModifiedForHighlighting has been replaced by an inline subscription
// in app.go that directly calls the highlight manager's AccumulateEdit method
