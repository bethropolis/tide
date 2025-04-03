package modehandler

import (
	"strings"

	"github.com/bethropolis/tide/internal/input"
	"github.com/bethropolis/tide/internal/logger"
)

// handleActionCommand handles actions when in ModeCommand.
func (mh *ModeHandler) handleActionCommand(actionEvent input.ActionEvent) bool {
	actionProcessed := true
	needsUpdate := false // Track if status bar text needs update

	switch actionEvent.Action {
	case input.ActionInsertRune:
		mh.cmdBuffer += string(actionEvent.Rune)
		needsUpdate = true

	case input.ActionDeleteCharBackward: // Backspace
		if len(mh.cmdBuffer) > 0 {
			// Correct handling for multi-byte runes might be needed here
			mh.cmdBuffer = mh.cmdBuffer[:len(mh.cmdBuffer)-1]
			needsUpdate = true
		} else {
			mh.currentMode = ModeNormal
			mh.statusBar.SetTemporaryMessage("") // Clear status explicitly
			logger.Debugf("ModeHandler: Exiting Command Mode via Backspace")
		}

	case input.ActionInsertNewLine: // Enter: Execute command
		mh.executeCommand()
		mh.currentMode = ModeNormal // Return to normal mode
		// executeCommand sets status message, redraw is needed

	case input.ActionQuit: // Escape: Cancel command
		mh.currentMode = ModeNormal
		mh.cmdBuffer = ""
		mh.statusBar.SetTemporaryMessage("") // Clear status
		logger.Debugf("ModeHandler: Canceled Command Mode via Escape")

	default:
		actionProcessed = false // Ignore other actions
	}

	// Update status bar display if buffer changed
	if needsUpdate && mh.currentMode == ModeCommand {
		mh.statusBar.SetTemporaryMessage(":%s", mh.cmdBuffer)
	}

	return actionProcessed
}

// executeCommand parses and runs the command in cmdBuffer.
func (mh *ModeHandler) executeCommand() {
	if mh.cmdBuffer == "" {
		mh.statusBar.SetTemporaryMessage("")
		return
	}
	cmdStr := mh.cmdBuffer // Copy buffer before clearing
	mh.cmdBuffer = ""      // Clear buffer now

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
