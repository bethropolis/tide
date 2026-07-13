package modehandler

import (
	"sort"
	"strings"

	"github.com/bethropolis/tide/internal/core/find"
	"github.com/bethropolis/tide/internal/input"
	"github.com/bethropolis/tide/internal/logger"
)

// handleActionCommand handles actions when in ModeCommand.
func (mh *ModeHandler) handleActionCommand(actionEvent input.ActionEvent) bool {
	actionProcessed := true
	needsUpdate := false // Track if status bar text needs update

	switch actionEvent.Action {
	case input.ActionInsertRune:
		mh.resetCommandAutocomplete()
		mh.cmdBuffer += string(actionEvent.Rune)
		needsUpdate = true

	case input.ActionDeleteCharBackward: // Backspace
		mh.resetCommandAutocomplete()
		if len(mh.cmdBuffer) > 0 {
			// Correct handling for multi-byte runes might be needed here
			mh.cmdBuffer = mh.cmdBuffer[:len(mh.cmdBuffer)-1]
			needsUpdate = true
		} else {
			mh.currentMode = ModeNormal
			mh.statusBar.SetTemporaryMessage("") // Clear status explicitly
			logger.Debugf("ModeHandler: Exiting Command Mode via Backspace")
		}

	case input.ActionInsertTab: // Autocomplete forward
		mh.handleCommandAutocomplete(false)
		needsUpdate = true

	case input.ActionInsertBacktab: // Autocomplete backward
		mh.handleCommandAutocomplete(true)
		needsUpdate = true

	case input.ActionInsertNewLine: // Enter: Execute command
		mh.resetCommandAutocomplete()
		mh.executeCommand()
		mh.currentMode = ModeNormal // Return to normal mode
		// executeCommand sets status message, redraw is needed

	case input.ActionQuit: // Escape: Cancel command
		mh.resetCommandAutocomplete()
		mh.currentMode = ModeNormal
		mh.cmdBuffer = ""
		mh.statusBar.SetTemporaryMessage("") // Clear status
		logger.Debugf("ModeHandler: Canceled Command Mode via Escape")

	default:
		actionProcessed = false // Ignore other actions
	}

	// Update status bar display if buffer changed
	if needsUpdate && mh.currentMode == ModeCommand {
		mh.updateCommandStatusBar()
	}

	return actionProcessed
}

func (mh *ModeHandler) resetCommandAutocomplete() {
	mh.cmdSuggestions = nil
	mh.cmdSuggestionIdx = -1
	mh.cmdOriginalBuf = ""
}

// handleCommandAutocomplete cycles through command suggestions based on the current buffer.
func (mh *ModeHandler) handleCommandAutocomplete(reverse bool) {
	// If starting fresh or typing a new word
	if mh.cmdSuggestionIdx == -1 {
		// Only attempt to autocomplete if there's no space in the buffer
		if strings.Contains(mh.cmdBuffer, " ") {
			return // Don't autocomplete arguments yet
		}

		mh.cmdOriginalBuf = mh.cmdBuffer
		mh.cmdSuggestions = []string{mh.cmdOriginalBuf}

		// Find matches
		var matches []string
		for name := range mh.commands {
			if strings.HasPrefix(name, mh.cmdOriginalBuf) {
				matches = append(matches, name)
			}
		}

		if len(matches) == 0 {
			return // Nothing to complete
		}

		sort.Strings(matches)
		mh.cmdSuggestions = append(mh.cmdSuggestions, matches...)
		mh.cmdSuggestionIdx = 0 // Start pointing at original string
	}

	if len(mh.cmdSuggestions) <= 1 {
		return // No choices
	}

	// Cycle forward or backward
	if reverse {
		mh.cmdSuggestionIdx--
		if mh.cmdSuggestionIdx < 0 {
			mh.cmdSuggestionIdx = len(mh.cmdSuggestions) - 1
		}
	} else {
		mh.cmdSuggestionIdx++
		if mh.cmdSuggestionIdx >= len(mh.cmdSuggestions) {
			mh.cmdSuggestionIdx = 0
		}
	}

	// Update buffer to the selected suggestion
	mh.cmdBuffer = mh.cmdSuggestions[mh.cmdSuggestionIdx]
}

func (mh *ModeHandler) updateCommandStatusBar() {
	if len(mh.cmdSuggestions) > 0 {
		// Display format: :command   [cmd1] cmd2 cmd3
		var parts []string
		for i, sug := range mh.cmdSuggestions {
			if i == 0 {
				continue // Skip the original buffer in the list
			}
			if i == mh.cmdSuggestionIdx {
				parts = append(parts, "["+sug+"]")
			} else {
				parts = append(parts, sug)
			}
		}

		msg := ":" + mh.cmdBuffer
		if len(parts) > 0 {
			msg += "   " + strings.Join(parts, " ")
		}
		mh.statusBar.SetTemporaryMessage(msg)
	} else {
		mh.statusBar.SetTemporaryMessage(":%s", mh.cmdBuffer)
	}
}

// executeCommand parses and runs the command in cmdBuffer.
func (mh *ModeHandler) executeCommand() {
	if mh.cmdBuffer == "" {
		mh.statusBar.SetTemporaryMessage("")
		return
	}
	cmdStr := mh.cmdBuffer // Copy buffer before clearing
	mh.cmdBuffer = ""      // Clear buffer now

	// --- Handle range-prefixed substitute commands before splitting on whitespace ---
	// :%s/pattern/replacement/[g][i]  → ReplaceAll across entire buffer
	if strings.HasPrefix(cmdStr, "%s/") {
		subStr := "/" + cmdStr[2:] // strip the leading '%', keep the /pat/rep/[g][i] part
		pattern, replacement, _, caseInsensitive, err := find.ParseSubstituteCommand(subStr)
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Invalid substitute: %v", err)
			return
		}
		if mh.api == nil {
			mh.statusBar.SetTemporaryMessage("No editor API available")
			return
		}
		count, err := mh.api.ReplaceAll(pattern, replacement, caseInsensitive)
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Replace failed: %v", err)
			return
		}
		if count == 0 {
			mh.statusBar.SetTemporaryMessage("Pattern not found: %s", pattern)
		} else {
			mh.statusBar.SetTemporaryMessage("Replaced %d occurrence(s)", count)
		}
		return
	}

	// :'<,'>s/pattern/replacement/[g][i]  → ReplaceInRange using current visual selection
	if strings.HasPrefix(cmdStr, "'<,'>s/") {
		subStr := "/" + cmdStr[6:] // strip "'<,'>", keep /pat/rep/[g][i]
		pattern, replacement, _, caseInsensitive, err := find.ParseSubstituteCommand(subStr)
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Invalid substitute: %v", err)
			return
		}
		if mh.api == nil {
			mh.statusBar.SetTemporaryMessage("No editor API available")
			return
		}
		// Determine range from the stored visual selection on the editor
		startLine, endLine := mh.editor.GetVisualSelectionLines()
		count, err := mh.api.ReplaceInRange(pattern, replacement, startLine, endLine, caseInsensitive)
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Replace failed: %v", err)
			return
		}
		if count == 0 {
			mh.statusBar.SetTemporaryMessage("Pattern not found: %s", pattern)
		} else {
			mh.statusBar.SetTemporaryMessage("Replaced %d occurrence(s)", count)
		}
		return
	}

	parts := strings.Fields(cmdStr)
	cmdName := parts[0]
	var args []string
	if len(parts) > 1 {
		args = parts[1:]
	}

	if cmdFunc, exists := mh.commands[cmdName]; exists {
		logger.Debugf("ModeHandler: Executing command ':%s' with args %v", cmdName, args)
		err := cmdFunc(args) // Execute
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Error executing command '%s': %v", cmdName, err)
		}
		// Success message usually set by the command itself via API
	} else {
		mh.statusBar.SetTemporaryMessage("Unknown command: %s", cmdName)
	}
}
