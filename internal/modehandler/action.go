package modehandler

import (
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/input"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
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
		input.ActionMovePageUp, input.ActionMovePageDown, input.ActionMoveHome, input.ActionMoveEnd,
		input.ActionMoveFileStart, input.ActionMoveFileEnd:
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

	case input.ActionEnterVisualBlockMode:
		mh.editor.StartOrUpdateSelection()
		mh.editor.SetBlockwise(true)
		mh.currentMode = ModeVisualBlock
		mh.statusBar.SetTemporaryMessage("-- VISUAL BLOCK --")
		logger.Debugf("ModeHandler: Entering Visual Block Mode")

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

	case input.ActionDeleteWordForward:
		if hasHighlights {
			mh.editor.ClearHighlights()
		}
		err := mh.editor.DeleteWordForward()
		if err != nil {
			logger.Debugf("Err DeleteWordForward: %v", err)
			actionProcessed = false
		} else {
			mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{})
		}

	case input.ActionDeleteWordBackward:
		if hasHighlights {
			mh.editor.ClearHighlights()
		}
		err := mh.editor.DeleteWordBackward()
		if err != nil {
			logger.Debugf("Err DeleteWordBackward: %v", err)
			actionProcessed = false
		} else {
			mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{})
		}

	case input.ActionMoveFileStart:
		mh.editor.GoToFileStart()

	case input.ActionMoveFileEnd:
		mh.editor.GoToFileEnd()

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
		mh.recordingInsert = false
		return mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
	}
	// Ctrl+V in insert mode should paste, not enter visual block mode
	if actionEvent.Action == input.ActionEnterVisualBlockMode {
		actionEvent.Action = input.ActionPaste
	}
	// Record edit actions for dot-repeat (only actual edits, not cursor movement)
	if actionEvent.Action != input.ActionUnknown {
		if !mh.recordingInsert {
			mh.lastInsertActions = nil
			mh.recordingInsert = true
		}
		mh.lastInsertActions = append(mh.lastInsertActions, input.ActionEvent{
			Action: actionEvent.Action,
			Rune:   actionEvent.Rune,
		})
	}
	processed := mh.executeAction(actionEvent.Action, actionEvent, ev)
	if processed && mh.onInsertEdit != nil {
		mh.onInsertEdit()
	}
	return processed
}

// handleActionNormal handles key events specific to Normal Mode.
func (mh *ModeHandler) handleActionNormal(actionEvent input.ActionEvent, ev *tcell.EventKey) bool {
	// Non-rune actions go directly to executeAction
	if actionEvent.Action != input.ActionInsertRune && actionEvent.Action != input.ActionUnknown {
		return mh.executeAction(actionEvent.Action, actionEvent, ev)
	}

	if actionEvent.Action == input.ActionInsertRune {
		r := actionEvent.Rune

		// Handle pending operators (dd, yy, dw, db, gg)
		if mh.pendingOperator != 0 {
			op := mh.pendingOperator
			mh.statusBar.ResetTemporaryMessage()

			// gg sequence
			if op == 'g' && r == 'g' {
				mh.pendingOperator = 0
				count := mh.drainCount()
				if count > 1 {
					mh.editor.SetCursor(types.Position{Line: count - 1, Col: 0})
				} else {
					mh.editor.GoToFileStart()
				}
				return true
			}
			// Cancel pending if not a valid continuation
			if op == 'g' {
				mh.pendingOperator = 0
				mh.statusBar.SetTemporaryMessage("g (pending)")
				// Fall through to handle this rune as a new key
			} else {
				mh.pendingOperator = 0
				count := mh.drainCount()

				switch {
				case r == op && (op == 'd' || op == 'y'):
					mh.editor.ClearSelection()
					for i := 0; i < count; i++ {
						mh.editor.StartOrUpdateSelection()
						mh.editor.SetLinewise(true)
					}
					if op == 'd' {
						return mh.executeAction(input.ActionCut, input.ActionEvent{Action: input.ActionCut}, ev)
					}
					return mh.executeAction(input.ActionYank, input.ActionEvent{Action: input.ActionYank}, ev)

				case op == 'd' && r == 'w':
					for i := 0; i < count; i++ {
						if err := mh.editor.DeleteWordForward(); err != nil {
							logger.Debugf("dw error: %v", err)
						}
					}
					mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{})
					return true

				case op == 'd' && r == 'b':
					for i := 0; i < count; i++ {
						if err := mh.editor.DeleteWordBackward(); err != nil {
							logger.Debugf("db error: %v", err)
						}
					}
					mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{})
					return true
				}

				mh.statusBar.SetTemporaryMessage("Operator cancelled")
				return true
			}
		}

		// Count prefix: accumulate digits in normal mode
		if r >= '1' && r <= '9' {
			mh.countAccumulator = int(r - '0')
			mh.statusBar.SetTemporaryMessage("%d", mh.countAccumulator)
			return true
		}
		if r == '0' && mh.countAccumulator > 0 {
			mh.countAccumulator *= 10
			mh.statusBar.SetTemporaryMessage("%d", mh.countAccumulator)
			return true
		}

		// Resolve count (default 1)
		count := mh.drainCount()

		// Dot-repeat: replay last insert actions
		if r == '.' {
			if len(mh.lastInsertActions) == 0 {
				mh.statusBar.SetTemporaryMessage("Nothing to repeat")
				return true
			}
			for _, ae := range mh.lastInsertActions {
				mh.executeAction(ae.Action, ae, ev)
			}
			return true
		}

		// gg / G
		if r == 'g' && mh.pendingOperator != 'g' {
			mh.pendingOperator = 'g'
			mh.statusBar.SetTemporaryMessage("g (pending)")
			return true
		}

		// * and # (search word under cursor)
		if r == '*' || r == '#' {
			return mh.searchWordUnderCursor(r == '*')
		}

		switch r {
		case 'd', 'y':
			mh.pendingOperator = r
			mh.statusBar.SetTemporaryMessage(string(r)+" (pending)")
			return true

		case 'i':
			for i := 0; i < count-1; i++ {
				mh.executeAction(input.ActionMoveLeft, input.ActionEvent{Action: input.ActionMoveLeft}, ev)
			}
			return mh.executeAction(input.ActionEnterInsertMode, actionEvent, ev)
		case 'a':
			for i := 0; i < count; i++ {
				mh.executeAction(input.ActionMoveRight, input.ActionEvent{Action: input.ActionMoveRight}, ev)
			}
			return mh.executeAction(input.ActionEnterInsertMode, actionEvent, ev)
		case 'v':
			return mh.executeAction(input.ActionEnterVisualMode, actionEvent, ev)
		case 'V':
			mh.editor.StartOrUpdateSelection()
			mh.editor.SetLinewise(true)
			mh.currentMode = ModeVisualLine
			mh.statusBar.SetTemporaryMessage("-- VISUAL LINE --")
			return true
		case 'A':
			mh.executeAction(input.ActionMoveEnd, input.ActionEvent{Action: input.ActionMoveEnd}, ev)
			return mh.executeAction(input.ActionEnterInsertMode, actionEvent, ev)
		case 'I':
			mh.executeAction(input.ActionMoveHome, input.ActionEvent{Action: input.ActionMoveHome}, ev)
			return mh.executeAction(input.ActionEnterInsertMode, actionEvent, ev)
		case 'o':
			mh.executeAction(input.ActionMoveEnd, input.ActionEvent{Action: input.ActionMoveEnd}, ev)
			mh.executeAction(input.ActionInsertNewLine, input.ActionEvent{Action: input.ActionInsertNewLine}, ev)
			return mh.executeAction(input.ActionEnterInsertMode, actionEvent, ev)
		case 'O':
			mh.executeAction(input.ActionMoveHome, input.ActionEvent{Action: input.ActionMoveHome}, ev)
			mh.executeAction(input.ActionInsertNewLine, input.ActionEvent{Action: input.ActionInsertNewLine}, ev)
			mh.executeAction(input.ActionMoveUp, input.ActionEvent{Action: input.ActionMoveUp}, ev)
			return mh.executeAction(input.ActionEnterInsertMode, actionEvent, ev)

		case 'h':
			for i := 0; i < count; i++ {
				mh.executeAction(input.ActionMoveLeft, input.ActionEvent{Action: input.ActionMoveLeft}, ev)
			}
			return true
		case 'j':
			for i := 0; i < count; i++ {
				mh.executeAction(input.ActionMoveDown, input.ActionEvent{Action: input.ActionMoveDown}, ev)
			}
			return true
		case 'k':
			for i := 0; i < count; i++ {
				mh.executeAction(input.ActionMoveUp, input.ActionEvent{Action: input.ActionMoveUp}, ev)
			}
			return true
		case 'l':
			for i := 0; i < count; i++ {
				mh.executeAction(input.ActionMoveRight, input.ActionEvent{Action: input.ActionMoveRight}, ev)
			}
			return true

		case 'w':
			for i := 0; i < count; i++ {
				mh.editor.WordForward()
			}
			return true
		case 'b':
			for i := 0; i < count; i++ {
				mh.editor.WordBackward()
			}
			return true
		case 'e':
			for i := 0; i < count; i++ {
				mh.editor.WordEnd()
			}
			return true

		case '0':
			mh.editor.HardHome()
			return true

		case 'x':
			mh.editor.ClearSelection()
			mh.editor.StartOrUpdateSelection()
			mh.editor.MoveCursor(0, 1)
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
		case 'n':
			return mh.executeAction(input.ActionFindNext, input.ActionEvent{Action: input.ActionFindNext}, ev)
		case 'N':
			return mh.executeAction(input.ActionFindPrevious, input.ActionEvent{Action: input.ActionFindPrevious}, ev)
		case 'G':
			if count > 1 {
				mh.editor.SetCursor(types.Position{Line: count - 1, Col: 0})
			} else {
				mh.editor.GoToFileEnd()
			}
			return true
		case 'J':
			// Join current line with next
			return mh.joinLines()
		}

		mh.statusBar.SetTemporaryMessage("Unmapped key in Normal mode: %c", r)
		return true
	}

	return false
}

// drainCount returns the accumulated count and resets it to 0.
func (mh *ModeHandler) drainCount() int {
	c := mh.countAccumulator
	mh.countAccumulator = 0
	if c == 0 {
		c = 1
	}
	return c
}

// searchWordUnderCursor searches forward (*) or backward (#) for the word under the cursor.
func (mh *ModeHandler) searchWordUnderCursor(forward bool) bool {
	buf := mh.editor.GetBuffer()
	if buf == nil {
		return false
	}
	pos := mh.editor.GetCursor()
	line, err := buf.Line(pos.Line)
	if err != nil {
		return false
	}
	if len(line) == 0 {
		return false
	}

	// Extract word boundaries at cursor
	col := pos.Col
	if col >= len(line) {
		col = len(line) - 1
	}

	// Find word start
	start := col
	for start > 0 && line[start-1] != ' ' && line[start-1] != '\t' {
		start--
	}
	// Find word end
	end := col
	for end < len(line)-1 && line[end+1] != ' ' && line[end+1] != '\t' {
		end++
	}

	if start >= end {
		return false
	}

	term := string(line[start : end+1])
	mh.lastSearchTerm = term
	mh.lastSearchForward = forward
	mh.editor.HighlightMatches(term)

	matchPos, found := mh.editor.Find(term, pos, forward)
	if found {
		mh.editor.SetCursor(matchPos)
		mh.editor.ScrollToCursor()
		mh.statusBar.SetTemporaryMessage("/%s", term)
	} else {
		mh.statusBar.SetTemporaryMessage("Pattern not found: %s", term)
	}
	return true
}

// joinLines joins the current line with the next line (Vim 'J').
func (mh *ModeHandler) joinLines() bool {
	buf := mh.editor.GetBuffer()
	if buf == nil {
		return false
	}
	pos := mh.editor.GetCursor()
	lineCount := buf.LineCount()
	if pos.Line >= lineCount-1 {
		return false
	}

	line, _ := buf.Line(pos.Line)
	nextLine, _ := buf.Line(pos.Line + 1)

	trimmedNext := nextLine
	for len(trimmedNext) > 0 && (trimmedNext[0] == ' ' || trimmedNext[0] == '\t') {
		trimmedNext = trimmedNext[1:]
	}

	_, err := buf.Delete(types.Position{Line: pos.Line, Col: len(line)}, types.Position{Line: pos.Line + 1, Col: 0})
	if err != nil {
		return false
	}
	_, err = buf.Insert(types.Position{Line: pos.Line, Col: len(line)}, []byte(" "+string(trimmedNext)))
	if err != nil {
		return false
	}
	mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{})
	return true
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

// handleActionVisualBlock handles key events in block-wise Visual Mode (Vim Ctrl+V).
func (mh *ModeHandler) handleActionVisualBlock(actionEvent input.ActionEvent, ev *tcell.EventKey) bool {
	if actionEvent.Action == input.ActionQuit {
		mh.editor.ClearSelection()
		mh.editor.SetBlockwise(false)
		return mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
	}

	isMovement := actionEvent.Action >= input.ActionMoveUp && actionEvent.Action <= input.ActionMoveEnd
	if isMovement {
		mockShiftEv := tcell.NewEventKey(ev.Key(), ev.Rune(), ev.Modifiers()|tcell.ModShift)
		return mh.executeAction(actionEvent.Action, actionEvent, mockShiftEv)
	}

	if actionEvent.Action == input.ActionInsertRune {
		switch actionEvent.Rune {
		case 'h':
			mh.editor.MoveCursor(0, -1)
			return true
		case 'j':
			mh.editor.MoveCursor(1, 0)
			return true
		case 'k':
			mh.editor.MoveCursor(-1, 0)
			return true
		case 'l':
			mh.editor.MoveCursor(0, 1)
			return true
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
		case 'y':
			res := mh.executeAction(input.ActionYank, actionEvent, ev)
			mh.editor.ClearSelection()
			mh.editor.SetBlockwise(false)
			mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
			return res
		case 'd', 'x':
			res := mh.executeAction(input.ActionCut, actionEvent, ev)
			mh.editor.ClearSelection()
			mh.editor.SetBlockwise(false)
			mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
			return res
		case 'p':
			res := mh.executeAction(input.ActionPaste, input.ActionEvent{Action: input.ActionPaste}, ev)
			mh.editor.ClearSelection()
			mh.editor.SetBlockwise(false)
			mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
			return res
		case 'P':
			res := mh.executeAction(input.ActionPasteBefore, input.ActionEvent{Action: input.ActionPasteBefore}, ev)
			mh.editor.ClearSelection()
			mh.editor.SetBlockwise(false)
			mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
			return res
		case 'c', 's':
			// Change block: delete block, enter insert mode at start of block
			startLine, endLine, startCol, _ := mh.editor.GetBlockRange()
			if startLine < 0 {
				return false
			}
			for line := startLine; line <= endLine; line++ {
				mh.editor.SetCursor(types.Position{Line: line, Col: startCol})
				_ = mh.editor.DeleteForward()
			}
			mh.editor.ClearSelection()
			mh.editor.SetBlockwise(false)
			mh.editor.SetCursor(types.Position{Line: startLine, Col: startCol})
			mh.currentMode = ModeInsert
			mh.statusBar.SetTemporaryMessage("-- INSERT --")
			return true
		}
	}

	if actionEvent.Action == input.ActionYank {
		res := mh.executeAction(input.ActionYank, actionEvent, ev)
		mh.editor.ClearSelection()
		mh.editor.SetBlockwise(false)
		mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
		return res
	}
	if actionEvent.Action == input.ActionCut || actionEvent.Action == input.ActionDeleteCharForward || actionEvent.Action == input.ActionDeleteCharBackward {
		res := mh.executeAction(input.ActionCut, actionEvent, ev)
		mh.editor.ClearSelection()
		mh.editor.SetBlockwise(false)
		mh.executeAction(input.ActionEnterNormalMode, actionEvent, ev)
		return res
	}

	return false
}
