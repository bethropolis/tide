package modehandler

import (
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/input"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/gdamore/tcell/v2"
)

// executeAction handles actions when in ModeNormal.
// This can also be used for leader sequences.
func (mh *ModeHandler) executeAction(action input.Action, actionEvent input.ActionEvent, ev *tcell.EventKey) bool {
	actionProcessed := true
	originalCursor := mh.editor.GetCursor()

	// Check for Shift modifier if original event is available
	isShift := false
	if ev != nil {
		isShift = ev.Modifiers()&tcell.ModShift != 0
	}

	hasHighlights := mh.editor.HasHighlights()

	// Determine if it's a movement action
	isMovementAction := false
	switch action {
	case input.ActionMoveUp, input.ActionMoveDown, input.ActionMoveLeft, input.ActionMoveRight,
		input.ActionMovePageUp, input.ActionMovePageDown, input.ActionMoveHome, input.ActionMoveEnd:
		isMovementAction = true
	}

	// Handle selection start/update based on Shift+Movement
	if isMovementAction && isShift {
		mh.editor.StartOrUpdateSelection()
	}

	// Handle selection clear based on non-Shift movement
	if isMovementAction && !isShift {
		mh.editor.ClearSelection()
	}

	// Execute the action
	switch action {
	// Mode Switching
	case input.ActionEnterCommandMode:
		mh.editor.ClearSelection()
		mh.currentMode = ModeCommand
		mh.cmdBuffer = ""
		mh.statusBar.SetTemporaryMessage(":")
		logger.Debugf("ModeHandler: Entering Command Mode")

	case input.ActionEnterFindMode:
		mh.editor.ClearSelection()
		mh.currentMode = ModeFind
		mh.findBuffer = ""
		mh.editor.ClearHighlights()
		mh.statusBar.SetTemporaryMessage("/")
		logger.Debugf("ModeHandler: Entering Find Mode")

	// Quit/Save actions
	case input.ActionQuit: // ESC or Ctrl+C in Normal Mode
		hasHighlights := mh.editor.HasHighlights()

		if hasHighlights {
			// If highlights exist, ESC just clears them
			mh.editor.ClearHighlights()
			mh.statusBar.SetTemporaryMessage("Highlights cleared")
			actionProcessed = true // Action processed, need redraw
		} else if mh.editor.GetBuffer().IsModified() && !mh.forceQuitPending {
			// No highlights, but buffer modified -> Show quit prompt
			mh.statusBar.SetTemporaryMessage("Unsaved changes! Press ESC again or Ctrl+Q to force quit.")
			mh.forceQuitPending = true
			actionProcessed = false // Don't process further, redraw needed for status
		} else {
			// No highlights, not modified -> Quit
			close(mh.quitSignal)
			actionProcessed = false
		}
	case input.ActionForceQuit:
		close(mh.quitSignal)
		actionProcessed = false

	case input.ActionSave:
		mh.editor.ClearSelection()
		err := mh.editor.SaveBuffer()
		savedPath := mh.editor.GetBuffer().FilePath()
		if savedPath == "" {
			savedPath = "[No Name]"
		}
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Save FAILED: %v", err)
		} else {
			mh.statusBar.SetTemporaryMessage("Buffer saved to %s", savedPath)
			mh.eventManager.Dispatch(event.TypeBufferSaved, event.BufferSavedData{FilePath: savedPath})
		}

	// Find Next/Previous
	case input.ActionFindNext:
		if mh.lastSearchTerm != "" {
			mh.executeFind(mh.lastSearchForward, true)
		} else {
			mh.statusBar.SetTemporaryMessage("No previous search term")
			actionProcessed = false
		}
	case input.ActionFindPrevious:
		if mh.lastSearchTerm != "" {
			mh.executeFind(!mh.lastSearchForward, true)
		} else {
			mh.statusBar.SetTemporaryMessage("No previous search term")
			actionProcessed = false
		}

	// Movement actions
	case input.ActionMoveUp:
		mh.editor.MoveCursor(-1, 0)
	case input.ActionMoveDown:
		mh.editor.MoveCursor(1, 0)
	case input.ActionMoveLeft:
		mh.editor.MoveCursor(0, -1)
	case input.ActionMoveRight:
		mh.editor.MoveCursor(0, 1)
	case input.ActionMovePageUp:
		mh.editor.PageMove(-1)
	case input.ActionMovePageDown:
		mh.editor.PageMove(1)
	case input.ActionMoveHome:
		mh.editor.Home()
	case input.ActionMoveEnd:
		mh.editor.End()

	// Yank/Paste actions
	case input.ActionYank:
		copied, err := mh.editor.YankSelection()
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Yank failed: %v", err)
			logger.Debugf("Yank error: %v", err)
			actionProcessed = false
		} else if copied {
			mh.statusBar.SetTemporaryMessage("Text copied to clipboard")
		} else {
			mh.statusBar.SetTemporaryMessage("Nothing selected to copy")
		}

	case input.ActionPaste:
		pasted, err := mh.editor.Paste()
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Paste failed: %v", err)
			logger.Debugf("Paste error: %v", err)
			actionProcessed = false
		} else if !pasted {
			mh.statusBar.SetTemporaryMessage("Clipboard empty - nothing to paste")
			actionProcessed = false
		} else {
			mh.statusBar.SetTemporaryMessage("Text pasted from clipboard")
		}

	// Undo/Redo actions
	case input.ActionUndo:
		undone, err := mh.editor.Undo()
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Undo failed: %v", err)
			logger.Debugf("Undo error: %v", err)
			actionProcessed = false
		} else if !undone {
			mh.statusBar.SetTemporaryMessage("Nothing to undo")
			actionProcessed = false
		} else {
			mh.statusBar.SetTemporaryMessage("Undo completed")
			// Event already dispatched by Undo method
		}

	case input.ActionRedo:
		redone, err := mh.editor.Redo()
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Redo failed: %v", err)
			logger.Debugf("Redo error: %v", err)
			actionProcessed = false
		} else if !redone {
			mh.statusBar.SetTemporaryMessage("Nothing to redo")
			actionProcessed = false
		} else {
			mh.statusBar.SetTemporaryMessage("Redo completed")
			// Event already dispatched by Redo method
		}

	// Text Modification actions
	case input.ActionInsertRune:
		if hasHighlights {
			mh.editor.ClearHighlights()
		}
		err := mh.editor.InsertRune(actionEvent.Rune)
		if err != nil {
			logger.Debugf("Err InsertRune: %v", err)
			actionProcessed = false
		} else {
			mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{})
		}
	case input.ActionInsertNewLine:
		if hasHighlights {
			mh.editor.ClearHighlights()
			mh.statusBar.SetTemporaryMessage("Highlights cleared")
		} else {
			err := mh.editor.InsertNewLine()
			if err != nil {
				logger.Debugf("Err InsertNewLine: %v", err)
				actionProcessed = false
			} else {
				mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{})
			}
		}
	case input.ActionDeleteCharBackward:
		if hasHighlights {
			mh.editor.ClearHighlights()
		}
		err := mh.editor.DeleteBackward()
		if err != nil {
			logger.Debugf("Err DeleteBackward: %v", err)
			actionProcessed = false
		} else {
			mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{})
		}
	case input.ActionDeleteCharForward:
		if hasHighlights {
			mh.editor.ClearHighlights()
		}
		err := mh.editor.DeleteForward()
		if err != nil {
			logger.Debugf("Err DeleteForward: %v", err)
			actionProcessed = false
		} else {
			mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{})
		}

	case input.ActionUnknown:
		actionProcessed = false
	default:
		actionProcessed = false
	}

	// Post-Action handling
	newCursor := mh.editor.GetCursor()
	if actionProcessed && newCursor != originalCursor {
		mh.eventManager.Dispatch(event.TypeCursorMoved, event.CursorMovedData{NewPosition: newCursor})
	}

	// Reset force quit flag
	if action != input.ActionQuit && action != input.ActionUnknown && actionProcessed {
		mh.forceQuitPending = false
	}

	return actionProcessed
}
