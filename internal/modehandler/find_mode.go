package modehandler

import (
	"github.com/bethropolis/tide/internal/input"
	"github.com/bethropolis/tide/internal/logger"
)

// handleActionFind handles actions when in ModeFind.
func (mh *ModeHandler) handleActionFind(actionEvent input.ActionEvent) bool {
	actionProcessed := true
	needsUpdate := false // Track if status bar text needs update

	switch actionEvent.Action {
	case input.ActionInsertRune: // Append to find buffer
		mh.findBuffer += string(actionEvent.Rune)
		needsUpdate = true

	case input.ActionDeleteCharBackward: // Backspace in find buffer
		if len(mh.findBuffer) > 0 {
			// TODO: Correct multi-byte rune handling for backspace if needed
			mh.findBuffer = mh.findBuffer[:len(mh.findBuffer)-1]
			needsUpdate = true
		} else {
			// Backspace on empty find buffer returns to Normal mode
			mh.cancelFindMode() // Use the new helper function
		}

	case input.ActionInsertNewLine: // Enter key: Execute search
		if mh.findBuffer != "" {
			mh.lastSearchTerm = mh.findBuffer // Store for 'n'/'N'
			mh.lastSearchForward = true       // Initial search is forward
			mh.executeFind(true, false)       // Execute find (forward), not subsequent
		} else {
			mh.statusBar.SetTemporaryMessage("") // Clear "/" if nothing typed
			mh.editor.ClearHighlights()          // Clear highlights if no search term
		}
		mh.currentMode = ModeNormal // Return to normal mode AFTER search attempt
		mh.findBuffer = ""          // Clear buffer after storing lastSearchTerm

	case input.ActionQuit: // Escape key: Cancel find
		mh.cancelFindMode() // Use the new helper function

	default:
		// Ignore other actions like movement keys in find mode
		actionProcessed = false
	}

	// Update status bar display if buffer changed
	if needsUpdate && mh.currentMode == ModeFind {
		mh.statusBar.SetTemporaryMessage("/%s", mh.findBuffer) // Show search prefix
	}

	return actionProcessed
}

// cancelFindMode centralizes logic for exiting Find mode without executing search
func (mh *ModeHandler) cancelFindMode() {
	mh.currentMode = ModeNormal
	mh.findBuffer = ""
	mh.editor.ClearHighlights() // Always clear highlights when canceling
	mh.statusBar.SetTemporaryMessage("")
	logger.Debugf("ModeHandler: Canceled Find Mode")
}

// executeFind performs the search using the findManager.
func (mh *ModeHandler) executeFind(forward bool, isSubsequent bool) {
	if mh.lastSearchTerm == "" {
		mh.statusBar.SetTemporaryMessage("No search term")
		return
	}

	// For initial search (not n/N), highlight all matches first
	if !isSubsequent {
		err := mh.editor.HighlightMatches(mh.lastSearchTerm)
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Invalid pattern: %s", err)
			return
		}
	}

	// Get the findManager and use it directly
	findManager := mh.editor.GetFindManager()
	if findManager == nil {
		mh.statusBar.SetTemporaryMessage("Find error: find manager not initialized")
		return
	}

	foundPos, found := findManager.FindNext(forward)
	if found {
		mh.editor.SetCursor(foundPos)  // Move cursor to start of match
		mh.editor.ScrollToCursor()     // Ensure cursor is visible
		mh.lastMatchPos = &foundPos    // Store found position
		mh.lastSearchForward = forward // Remember direction for next 'n'/'N'
		mh.statusBar.SetTemporaryMessage("Found: '%s'", mh.lastSearchTerm)
		logger.Debugf("ModeHandler: Found '%s' at %v", mh.lastSearchTerm, foundPos)
	} else {
		mh.statusBar.SetTemporaryMessage("Pattern not found: %s", mh.lastSearchTerm)
		logger.Debugf("ModeHandler: Pattern not found: '%s'", mh.lastSearchTerm)
	}
}
