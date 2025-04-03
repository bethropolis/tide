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

// executeFind performs the search and updates editor state.
// isSubsequent indicates if this is an n/N navigation after initial search.
func (mh *ModeHandler) executeFind(forward bool, isSubsequent bool) {
	if mh.lastSearchTerm == "" {
		mh.statusBar.SetTemporaryMessage("No search term")
		return
	}

	// Start search from current cursor pos + direction offset
	startPos := mh.editor.GetCursor()

	if isSubsequent && mh.lastMatchPos != nil {
		// For n/N, start from last match position
		startPos = *mh.lastMatchPos
		// Offset depends on direction to avoid finding the same match repeatedly
		if forward {
			startPos.Col++ // Move one column past start of last match for forward search
		}
		// For backward search, the editor.Find will handle starting before startPos
	} else if !isSubsequent {
		// For initial search (Enter key), start from current cursor position
		// and clear the last match position
		mh.lastMatchPos = nil
	}

	foundPos, found := mh.editor.Find(mh.lastSearchTerm, startPos, forward)

	if found {
		mh.editor.SetCursor(foundPos)                                      // Move cursor to start of match
		mh.editor.HighlightMatches(mh.lastSearchTerm)                      // Highlight all matches
		mh.lastMatchPos = &foundPos                                        // Store found position
		mh.statusBar.SetTemporaryMessage("Found: '%s'", mh.lastSearchTerm) // Show success message
		logger.Debugf("ModeHandler: Found '%s' at %v", mh.lastSearchTerm, foundPos)
	} else {
		// Only clear highlights if pattern not found
		mh.editor.ClearHighlights() // Clear previous highlights when not found
		mh.lastMatchPos = nil       // Reset last match position
		mh.statusBar.SetTemporaryMessage("Pattern not found: %s", mh.lastSearchTerm)
		logger.Debugf("ModeHandler: Pattern not found: '%s'", mh.lastSearchTerm)
	}
}
