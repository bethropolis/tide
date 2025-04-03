// internal/modehandler/modehandler.go
package modehandler

import (
	"fmt"
	"log"
	"strings"

	// We need access to core components to execute actions
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/input"
	"github.com/bethropolis/tide/internal/plugin" // For CommandFunc type
	"github.com/bethropolis/tide/internal/statusbar"
	"github.com/gdamore/tcell/v2"
)

// InputMode defines the different states for user input.
type InputMode int

const (
	ModeNormal InputMode = iota
	ModeCommand
	// Future: ModeInsert, ModeVisual, etc.
)

// ModeHandler manages input modes, command execution, and related state.
type ModeHandler struct {
	// Dependencies (references to components managed by App)
	editor         *core.Editor
	inputProcessor *input.InputProcessor
	eventManager   *event.Manager
	statusBar      *statusbar.StatusBar
	quitSignal     chan<- struct{} // Channel to signal app termination

	// Internal State
	currentMode InputMode
	cmdBuffer   string
	commands    map[string]plugin.CommandFunc // Command registry
	forceQuitPending bool // Moved from App
}

// Config holds dependencies for the ModeHandler.
type Config struct {
	Editor         *core.Editor
	InputProcessor *input.InputProcessor
	EventManager   *event.Manager
	StatusBar      *statusbar.StatusBar
	QuitSignal     chan<- struct{} // Write-only channel to signal quit
}

// New creates a new ModeHandler.
func New(cfg Config) *ModeHandler {
	if cfg.Editor == nil || cfg.InputProcessor == nil || cfg.EventManager == nil || cfg.StatusBar == nil || cfg.QuitSignal == nil {
		// Should ideally return an error, but panic indicates programming error during setup
		panic("modehandler.New: Missing required dependencies in Config")
	}
	return &ModeHandler{
		editor:         cfg.Editor,
		inputProcessor: cfg.InputProcessor,
		eventManager:   cfg.EventManager,
		statusBar:      cfg.StatusBar,
		quitSignal:     cfg.QuitSignal,
		currentMode:    ModeNormal,
		commands:       make(map[string]plugin.CommandFunc),
		cmdBuffer:      "",
	}
}

// HandleKeyEvent decides what to do based on current mode and key event.
// Returns true if the event resulted in an action requiring redraw.
func (mh *ModeHandler) HandleKeyEvent(ev *tcell.EventKey) bool {
	// Always dispatch raw key event first
	mh.eventManager.Dispatch(event.TypeKeyPressed, event.KeyPressedData{KeyEvent: ev})

	actionEvent := mh.inputProcessor.ProcessEvent(ev)

	var actionProcessed bool
	switch mh.currentMode {
	case ModeNormal:
		actionProcessed = mh.handleActionNormal(actionEvent)
	case ModeCommand:
		actionProcessed = mh.handleActionCommand(actionEvent)
	default:
		log.Printf("Warning: Unknown input mode: %v", mh.currentMode)
		actionProcessed = false
	}

	// Determine if redraw is needed based on whether action was processed
	// or if specific non-processed states require status update (quit prompt).
	needsRedraw := actionProcessed || (actionEvent.Action == input.ActionQuit && mh.forceQuitPending)

	return needsRedraw
}

// handleActionNormal handles actions when in ModeNormal.
func (mh *ModeHandler) handleActionNormal(actionEvent input.ActionEvent) bool {
	actionProcessed := true
	originalCursor := mh.editor.GetCursor() // Get cursor before action

	switch actionEvent.Action {
	case input.ActionEnterCommandMode:
		mh.currentMode = ModeCommand
		mh.cmdBuffer = ""
		mh.statusBar.SetTemporaryMessage(":")
		log.Println("ModeHandler: Entering Command Mode")

	case input.ActionQuit:
		if mh.editor.GetBuffer().IsModified() && !mh.forceQuitPending {
			mh.statusBar.SetTemporaryMessage("Unsaved changes! Press ESC again or Ctrl+Q to force quit.")
			mh.forceQuitPending = true
			actionProcessed = false // Needs redraw for status, but didn't quit yet
		} else {
			close(mh.quitSignal) // Signal app to quit
			actionProcessed = false
		}
	case input.ActionForceQuit:
		close(mh.quitSignal)
		actionProcessed = false

	case input.ActionSave:
		err := mh.editor.SaveBuffer()
		savedPath := mh.editor.GetBuffer().FilePath()
		// if savedPath == "" { savedPath = mh.filePath } // ModeHandler doesn't know initial filePath
		if savedPath == "" { savedPath = "[No Name]" }
		if err != nil { mh.statusBar.SetTemporaryMessage("Save FAILED: %v", err) } else { mh.statusBar.SetTemporaryMessage("Buffer saved to %s", savedPath); mh.eventManager.Dispatch(event.TypeBufferSaved, event.BufferSavedData{FilePath: savedPath}) }

	// --- Movement/Editing Actions ---
	case input.ActionMoveUp: mh.editor.MoveCursor(-1, 0)
	case input.ActionMoveDown: mh.editor.MoveCursor(1, 0)
	case input.ActionMoveLeft: mh.editor.MoveCursor(0, -1)
	case input.ActionMoveRight: mh.editor.MoveCursor(0, 1)
	case input.ActionMovePageUp: mh.editor.PageMove(-1)
	case input.ActionMovePageDown: mh.editor.PageMove(1)
	case input.ActionMoveHome: mh.editor.Home()
	case input.ActionMoveEnd: mh.editor.End()
	case input.ActionInsertRune: err := mh.editor.InsertRune(actionEvent.Rune); if err != nil { log.Printf("Err InsertRune: %v", err); actionProcessed = false } else { mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{}) }
	case input.ActionInsertNewLine: err := mh.editor.InsertNewLine(); if err != nil { log.Printf("Err InsertNewLine: %v", err); actionProcessed = false } else { mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{}) }
	case input.ActionDeleteCharBackward: err := mh.editor.DeleteBackward(); if err != nil { log.Printf("Err DeleteBackward: %v", err); actionProcessed = false } else { mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{}) }
	case input.ActionDeleteCharForward: err := mh.editor.DeleteForward(); if err != nil { log.Printf("Err DeleteForward: %v", err); actionProcessed = false } else { mh.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{}) }

	case input.ActionUnknown: actionProcessed = false
	default: actionProcessed = false
	}

	// Dispatch cursor move event
	newCursor := mh.editor.GetCursor()
	if actionProcessed && newCursor != originalCursor {
		mh.eventManager.Dispatch(event.TypeCursorMoved, event.CursorMovedData{NewPosition: newCursor})
	}

	// Reset force quit flag
	if actionEvent.Action != input.ActionQuit && actionEvent.Action != input.ActionUnknown && actionProcessed {
		mh.forceQuitPending = false
	}

	return actionProcessed
}

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
			log.Println("ModeHandler: Exiting Command Mode via Backspace")
		}

	case input.ActionInsertNewLine: // Enter: Execute command
		mh.executeCommand()
		mh.currentMode = ModeNormal // Return to normal mode
		// executeCommand sets status message, redraw is needed

	case input.ActionQuit: // Escape: Cancel command
		mh.currentMode = ModeNormal
		mh.cmdBuffer = ""
		mh.statusBar.SetTemporaryMessage("") // Clear status
		log.Println("ModeHandler: Canceled Command Mode via Escape")

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
	if len(parts) > 1 { args = parts[1:] }

	if cmdFunc, exists := mh.commands[cmdName]; exists {
		log.Printf("ModeHandler: Executing command ':%s' with args %v", cmdName, args)
		err := cmdFunc(args) // Execute
		if err != nil {
			mh.statusBar.SetTemporaryMessage("Error executing command '%s': %v", cmdName, err)
		}
		// Success message usually set by the command itself via API
	} else {
		mh.statusBar.SetTemporaryMessage("Unknown command: %s", cmdName)
	}
}

// RegisterCommand adds a command to the registry. Called via EditorAPI.
func (mh *ModeHandler) RegisterCommand(name string, cmdFunc plugin.CommandFunc) error {
	if name == "" { return fmt.Errorf("command name cannot be empty") }
	if _, exists := mh.commands[name]; exists { return fmt.Errorf("command '%s' already registered", name) }
	mh.commands[name] = cmdFunc
	log.Printf("ModeHandler: Registered command ':%s'", name)
	return nil
}

// GetCurrentMode returns the current input mode.
func (mh *ModeHandler) GetCurrentMode() InputMode {
	return mh.currentMode
}

// GetCommandBuffer returns the current command buffer content (e.g., for display).
func (mh *ModeHandler) GetCommandBuffer() string {
	// Only relevant in command mode, but safe to return otherwise
	if mh.currentMode == ModeCommand {
		return mh.cmdBuffer
	}
	return ""
}