package app

import (
	"fmt"
	"time"

	"github.com/bethropolis/tide/internal/config"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/modehandler"
	"github.com/bethropolis/tide/internal/render"
)

// drawEditor clears screen and redraws all components.
func (a *App) drawEditor() {
	// Update status bar content (might involve modehandler state)
	a.updateStatusBarContent()

	// Get the current theme from the manager
	currentTheme := a.themeManager.Current()
	a.activeTheme = currentTheme // Update activeTheme reference

	screen := a.tuiManager.GetScreen()
	width, height := a.tuiManager.Size()
	statusBarHeight := config.Get().Editor.StatusBarHeight
	viewHeight := height - statusBarHeight

	// --- Add Detailed Logging ---
	logger.DebugTagf("draw", "drawEditor: Screen Size (%d x %d), StatusBarHeight: %d, Calculated ViewHeight: %d",
		width, height, statusBarHeight, viewHeight)
	// --- End Logging ---

	a.tuiManager.Clear()
	// Use the render package to draw the buffer
	render.Buffer(a.tuiManager, a.editor, a.activeTheme)
	a.statusBar.Draw(screen, width, height, a.activeTheme) // Pass theme to status bar
	render.Cursor(a.tuiManager, a.editor)
	a.tuiManager.Show()
}

// updateStatusBarContent pushes current editor state to the status bar component.
func (a *App) updateStatusBarContent() {
	buffer := a.editor.GetBuffer()
	a.statusBar.SetFileInfo(buffer.FilePath(), buffer.IsModified())
	a.statusBar.SetCursorInfo(a.editor.GetCursor())
	a.statusBar.SetEditorMode(a.modeHandler.GetCurrentModeString())

	// If in command mode, ensure the command buffer is displayed via status bar's temp message
	if a.modeHandler.GetCurrentMode() == modehandler.ModeCommand {
		a.statusBar.SetTemporaryMessage(":%s", a.modeHandler.GetCommandBuffer())
	} else if a.modeHandler.GetCurrentMode() == modehandler.ModeFind {
		// Update status bar with find buffer in find mode
		a.statusBar.SetTemporaryMessage("/%s", a.modeHandler.GetFindBuffer())
	}

	// If we have an active status message, transfer it to the status bar
	// This code will help during transition
	if !a.statusMessageTime.IsZero() && time.Since(a.statusMessageTime) <= 4*time.Second {
		a.statusBar.SetTemporaryMessage(a.statusMessage)
	}
}

// SetStatusMessage updates the status message.
// Keeping temporarily for backward compatibility during transition
func (a *App) SetStatusMessage(format string, args ...interface{}) {
	a.statusMessage = fmt.Sprintf(format, args...)
	a.statusMessageTime = time.Now()
	// Forward to the new status bar
	a.statusBar.SetTemporaryMessage(a.statusMessage)
}

// requestRedraw sends a redraw signal non-blockingly.
func (a *App) requestRedraw() {
	select {
	case a.redrawRequest <- struct{}{}:
	default: // Don't block if a redraw is already pending
	}
}
