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
	if isMovementAction && !isShift && mh.currentMode != ModeVisual {
		mh.editor.ClearSelection()
	}

	// Execute the action
	switch action {
	// Mode Switching
	case input.ActionEnterInsertMode:
		mh.editor.ClearSelection()
		mh.currentMode = ModeInsert
		mh.statusBar.SetTemporaryMessage("-- INSERT --")
		logger.Debugf("ModeHandler: Entering Insert Mode")

	case input.ActionEnterNormalMode:
		mh.editor.ClearSelection()
		mh.currentMode = ModeNormal
		mh.statusBar.SetTemporaryMessage("-- NORMAL --")
		logger.Debugf("ModeHandler: Entering Normal Mode")

	case input.ActionEnterVisualMode:
		mh.editor.StartOrUpdateSelection()
		mh.currentMode = ModeVisual
		mh.statusBar.SetTemporaryMessage("-- VISUAL --")
		logger.Debugf("ModeHandler: Entering Visual Mode")

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

	case input.ActionCut:
		cut, err := mh.editor.CutSelection()
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Cut failed: %v", err)
			logger.Debugf("Cut error: %v", err)
			actionProcessed = false
		} else if cut {
			mh.statusBar.SetTemporaryMessage("Text cut to clipboard")
		} else {
			mh.statusBar.SetTemporaryMessage("Nothing selected to cut")
		}

	case input.ActionPaste:
		pasted, err := mh.editor.Paste(true) // Default to pasting after
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

	case input.ActionPasteBefore:
		pasted, err := mh.editor.Paste(false) // Paste before
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
	case input.ActionInsertTab:
		if hasHighlights {
			mh.editor.ClearHighlights()
		}
		err := mh.editor.InsertTab()
		if err != nil {
			logger.Debugf("Err InsertTab: %v", err)
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
		mh.eventManager.Dispatch(event.TypeCursorMoved, event.CursorMovedData{
			OldPosition: originalCursor,
			NewPosition: newCursor,
		})
	}

	// Reset force quit flag
	if action != input.ActionQuit && action != input.ActionUnknown && actionProcessed {
		mh.forceQuitPending = false
	}

	return actionProcessed
}

// handleActionInsert handles key events specific to Insert Mode.
func (mh *ModeHandler) handleActionInsert(actionEvent input.ActionEvent, ev *tcell.EventKey) bool {
	if actionEvent.Action == input.ActionQuit {
		return mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
	}
	return mh.executeAction(actionEvent.Action, actionEvent, ev)
}

// handleActionNormal handles key events specific to Normal Mode.
func (mh *ModeHandler) handleActionNormal(actionEvent input.ActionEvent, ev *tcell.EventKey) bool {
	if actionEvent.Action != input.ActionInsertRune && actionEvent.Action != input.ActionUnknown {
		return mh.executeAction(actionEvent.Action, actionEvent, ev)
	}

	if actionEvent.Action == input.ActionInsertRune {
		// Handle pending operators (dd, yy)
		if mh.pendingOperator != 0 {
			op := mh.pendingOperator
			mh.pendingOperator = 0 // reset
			mh.statusBar.ResetTemporaryMessage()

			if actionEvent.Rune == op {
				// We have a double operator (dd or yy)
				mh.editor.ClearSelection()
				mh.editor.StartOrUpdateSelection()
				mh.editor.SetLinewise(true) // line-wise selection

				if op == 'd' {
					return mh.executeAction(input.ActionCut, input.ActionEvent{Action: input.ActionCut}, ev)
				} else if op == 'y' {
					return mh.executeAction(input.ActionYank, input.ActionEvent{Action: input.ActionYank}, ev)
				}
			}
			// If not a double operator, you could handle movements (dw, d$) here in the future
			// For now, cancel operator if not matched
			mh.statusBar.SetTemporaryMessage("Operator cancelled")
			return true
		}

		switch actionEvent.Rune {
		case 'd', 'y':
			mh.pendingOperator = actionEvent.Rune
			mh.statusBar.SetTemporaryMessage(string(actionEvent.Rune) + " (pending)")
			return true
		case 'i':
			return mh.executeAction(input.ActionEnterInsertMode, actionEvent, ev)
		case 'a':
			mh.editor.MoveCursor(0, 1)
			return mh.executeAction(input.ActionEnterInsertMode, actionEvent, ev)
		case 'v':
			return mh.executeAction(input.ActionEnterVisualMode, actionEvent, ev)
		case 'V':
			// Enter line-wise visual mode
			mh.editor.StartOrUpdateSelection()
			mh.editor.SetLinewise(true)
			mh.currentMode = ModeVisualLine
			mh.statusBar.SetTemporaryMessage("-- VISUAL LINE --")
			logger.Debugf("ModeHandler: Entering Visual Line Mode")
			return true
		case 'A':
			// Move to end of line and insert
			mh.executeAction(input.ActionMoveEnd, input.ActionEvent{Action: input.ActionMoveEnd}, ev)
			return mh.executeAction(input.ActionEnterInsertMode, actionEvent, ev)
		case 'I':
			// Move to start of line and insert
			mh.executeAction(input.ActionMoveHome, input.ActionEvent{Action: input.ActionMoveHome}, ev)
			return mh.executeAction(input.ActionEnterInsertMode, actionEvent, ev)
		case 'o':
			// Insert line below and insert
			mh.executeAction(input.ActionMoveEnd, input.ActionEvent{Action: input.ActionMoveEnd}, ev)
			mh.executeAction(input.ActionInsertNewLine, input.ActionEvent{Action: input.ActionInsertNewLine}, ev)
			return mh.executeAction(input.ActionEnterInsertMode, actionEvent, ev)
		case 'O':
			// Insert line above and insert
			mh.executeAction(input.ActionMoveHome, input.ActionEvent{Action: input.ActionMoveHome}, ev)
			mh.executeAction(input.ActionInsertNewLine, input.ActionEvent{Action: input.ActionInsertNewLine}, ev)
			mh.executeAction(input.ActionMoveUp, input.ActionEvent{Action: input.ActionMoveUp}, ev)
			return mh.executeAction(input.ActionEnterInsertMode, actionEvent, ev)
		case 'h':
			return mh.executeAction(input.ActionMoveLeft, input.ActionEvent{Action: input.ActionMoveLeft}, ev)
		case 'j':
			return mh.executeAction(input.ActionMoveDown, input.ActionEvent{Action: input.ActionMoveDown}, ev)
		case 'k':
			return mh.executeAction(input.ActionMoveUp, input.ActionEvent{Action: input.ActionMoveUp}, ev)
		case 'l':
			return mh.executeAction(input.ActionMoveRight, input.ActionEvent{Action: input.ActionMoveRight}, ev)
		case 'w':
			mh.editor.WordForward()
			return true
		case 'b':
			mh.editor.WordBackward()
			return true
		case 'e':
			mh.editor.WordEnd()
			return true
		case '0':
			mh.editor.HardHome()
			return true
		case 'x':
			// Select current character and cut it
			mh.editor.ClearSelection()
			mh.editor.StartOrUpdateSelection()
			mh.editor.MoveCursor(0, 1) // Select 1 char
			return mh.executeAction(input.ActionCut, input.ActionEvent{Action: input.ActionCut}, ev)
		case 'u':
			return mh.executeAction(input.ActionUndo, input.ActionEvent{Action: input.ActionUndo}, ev)
		case 'p':
			return mh.executeAction(input.ActionPaste, input.ActionEvent{Action: input.ActionPaste}, ev)
		case 'P':
			return mh.executeAction(input.ActionPasteBefore, input.ActionEvent{Action: input.ActionPasteBefore}, ev)
		case '/':
			return mh.executeAction(input.ActionEnterFindMode, input.ActionEvent{Action: input.ActionEnterFindMode}, ev)
		case ':':
			return mh.executeAction(input.ActionEnterCommandMode, input.ActionEvent{Action: input.ActionEnterCommandMode}, ev)
		}

		mh.statusBar.SetTemporaryMessage("Unmapped key in Normal mode: %c", actionEvent.Rune)
		return true
	}

	return false
}

// handleActionVisual handles key events specific to Visual Mode.
func (mh *ModeHandler) handleActionVisual(actionEvent input.ActionEvent, ev *tcell.EventKey) bool {
	if actionEvent.Action == input.ActionQuit {
		mh.editor.ClearSelection()
		return mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
	}

	if actionEvent.Action >= input.ActionMoveUp && actionEvent.Action <= input.ActionMoveEnd {
		mockShiftEv := tcell.NewEventKey(ev.Key(), ev.Rune(), ev.Modifiers()|tcell.ModShift)
		return mh.executeAction(actionEvent.Action, actionEvent, mockShiftEv)
	}

	if actionEvent.Action == input.ActionYank || (actionEvent.Action == input.ActionInsertRune && actionEvent.Rune == 'y') {
		res := mh.executeAction(input.ActionYank, actionEvent, ev)
		mh.editor.ClearSelection()
		mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
		return res
	}

	if actionEvent.Action == input.ActionDeleteCharForward || actionEvent.Action == input.ActionDeleteCharBackward || (actionEvent.Action == input.ActionInsertRune && (actionEvent.Rune == 'd' || actionEvent.Rune == 'x')) {
		res := mh.executeAction(input.ActionCut, actionEvent, ev)
		mh.editor.ClearSelection()
		mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
		return res
	}

	// p in visual mode: paste over selection (delete selection, then paste)
	if actionEvent.Action == input.ActionPaste || (actionEvent.Action == input.ActionInsertRune && actionEvent.Rune == 'p') {
		res := mh.executeAction(input.ActionPaste, input.ActionEvent{Action: input.ActionPaste}, ev)
		mh.editor.ClearSelection()
		mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
		return res
	}

	// P in visual mode: paste-before over selection
	if actionEvent.Action == input.ActionPasteBefore || (actionEvent.Action == input.ActionInsertRune && actionEvent.Rune == 'P') {
		res := mh.executeAction(input.ActionPasteBefore, input.ActionEvent{Action: input.ActionPasteBefore}, ev)
		mh.editor.ClearSelection()
		mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
		return res
	}

	return false
}

// handleActionVisualLine handles key events in line-wise Visual Mode (Vim 'V').
func (mh *ModeHandler) handleActionVisualLine(actionEvent input.ActionEvent, ev *tcell.EventKey) bool {
	// ESC → back to Normal
	if actionEvent.Action == input.ActionQuit {
		mh.editor.ClearSelection()
		return mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
	}

	// Movement: update line-wise selection (cursor moves, selection follows)
	isMovement := actionEvent.Action >= input.ActionMoveUp && actionEvent.Action <= input.ActionMoveEnd
	if isMovement {
		mockShiftEv := tcell.NewEventKey(ev.Key(), ev.Rune(), ev.Modifiers()|tcell.ModShift)
		res := mh.executeAction(actionEvent.Action, actionEvent, mockShiftEv)
		// Ensure linewise flag stays set after movement (executeAction may reset via ClearSelection)
		mh.editor.SetLinewise(true)
		return res
	}

	// Rune-based movement (hjkl, w, b, e, 0)
	if actionEvent.Action == input.ActionInsertRune {
		switch actionEvent.Rune {
		case 'h':
			mh.editor.MoveCursor(0, -1)
			mh.editor.SetLinewise(true)
			return true
		case 'j':
			mh.editor.MoveCursor(1, 0)
			mh.editor.SetLinewise(true)
			return true
		case 'k':
			mh.editor.MoveCursor(-1, 0)
			mh.editor.SetLinewise(true)
			return true
		case 'l':
			mh.editor.MoveCursor(0, 1)
			mh.editor.SetLinewise(true)
			return true
		case 'w':
			mh.editor.WordForward()
			mh.editor.SetLinewise(true)
			return true
		case 'b':
			mh.editor.WordBackward()
			mh.editor.SetLinewise(true)
			return true
		case 'e':
			mh.editor.WordEnd()
			mh.editor.SetLinewise(true)
			return true
		case '0':
			mh.editor.HardHome()
			mh.editor.SetLinewise(true)
			return true
		case 'y':
			// Yank selected lines then return to Normal
			res := mh.executeAction(input.ActionYank, actionEvent, ev)
			mh.editor.ClearSelection()
			mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
			return res
		case 'd', 'x':
			// Delete selected lines then return to Normal
			res := mh.executeAction(input.ActionCut, actionEvent, ev)
			mh.editor.ClearSelection()
			mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
			return res
		case 'p':
			// Paste over selected lines then return to Normal
			res := mh.executeAction(input.ActionPaste, input.ActionEvent{Action: input.ActionPaste}, ev)
			mh.editor.ClearSelection()
			mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
			return res
		case 'P':
			// Paste-before over selected lines then return to Normal
			res := mh.executeAction(input.ActionPasteBefore, input.ActionEvent{Action: input.ActionPasteBefore}, ev)
			mh.editor.ClearSelection()
			mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
			return res
		}
	}

	// Yank/Delete via actions
	if actionEvent.Action == input.ActionYank {
		res := mh.executeAction(input.ActionYank, actionEvent, ev)
		mh.editor.ClearSelection()
		mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
		return res
	}
	if actionEvent.Action == input.ActionDeleteCharForward || actionEvent.Action == input.ActionDeleteCharBackward {
		res := mh.executeAction(input.ActionCut, actionEvent, ev)
		mh.editor.ClearSelection()
		mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
		return res
	}

	return false
}
