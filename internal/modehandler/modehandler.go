// internal/modehandler/modehandler.go
package modehandler

import (
	"fmt"
	"time"

	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/input"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/plugin"
	"github.com/bethropolis/tide/internal/statusbar"
	"github.com/bethropolis/tide/internal/types"
	"github.com/gdamore/tcell/v2"
)

// InputMode defines the different states for user input.
type InputMode int

const (
	ModeNormal InputMode = iota
	ModeCommand
	ModeFind
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
	currentMode      InputMode
	cmdBuffer        string
	findBuffer       string
	commands         map[string]plugin.CommandFunc
	forceQuitPending bool

	// Find State
	lastSearchTerm    string
	lastSearchForward bool
	lastMatchPos      *types.Position

	// Leader Key State
	leaderWaiting bool
	leaderTimer   *time.Timer
	leaderKey     rune
}

// Config holds dependencies for the ModeHandler.
type Config struct {
	Editor         *core.Editor
	InputProcessor *input.InputProcessor
	EventManager   *event.Manager
	StatusBar      *statusbar.StatusBar
	QuitSignal     chan<- struct{}
}

// New creates a new ModeHandler.
func New(cfg Config) *ModeHandler {
	if cfg.Editor == nil || cfg.InputProcessor == nil || cfg.EventManager == nil || cfg.StatusBar == nil || cfg.QuitSignal == nil {
		panic("modehandler.New: Missing required dependencies in Config")
	}
	mh := &ModeHandler{
		editor:            cfg.Editor,
		inputProcessor:    cfg.InputProcessor,
		eventManager:      cfg.EventManager,
		statusBar:         cfg.StatusBar,
		quitSignal:        cfg.QuitSignal,
		currentMode:       ModeNormal,
		commands:          make(map[string]plugin.CommandFunc),
		cmdBuffer:         "",
		lastSearchForward: true, // Default search direction
	}
	mh.leaderKey = cfg.InputProcessor.GetLeaderKey() // Cache leader key
	return mh
}

// HandleKeyEvent decides what to do based on current mode and key event.
// Returns true if the event resulted in an action requiring redraw.
func (mh *ModeHandler) HandleKeyEvent(ev *tcell.EventKey) bool {
	// Dispatch raw key event first
	mh.eventManager.Dispatch(event.TypeKeyPressed, event.KeyPressedData{KeyEvent: ev})

	actionEvent := mh.inputProcessor.ProcessEvent(ev) // Get base action

	var actionProcessed bool
	needsRedraw := false

	// --- Leader Sequence Handling ---
	if mh.leaderWaiting {
		mh.stopLeaderTimer() // Received a key, stop the timer

		// Check if the current key completes a sequence
		if actionEvent.Action == input.ActionInsertRune {
			if seqAction, isSequence := mh.inputProcessor.IsLeaderSequenceKey(actionEvent.Rune); isSequence {
				// Valid sequence completed
				logger.Debugf("Leader sequence completed: Leader + %c -> Action %v", actionEvent.Rune, seqAction)
				mh.resetLeaderState()
				// Execute the sequence action instead of the rune insert action
				actionProcessed = mh.executeAction(seqAction, input.ActionEvent{Action: seqAction, Rune: actionEvent.Rune}, ev)
				needsRedraw = actionProcessed
				return needsRedraw
			}
		}

		// Invalid sequence key or non-rune key pressed after leader
		logger.Debugf("Invalid leader sequence key or timeout occurred")
		// Insert the literal leader key first
		_ = mh.executeAction(input.ActionInsertRune, input.ActionEvent{Action: input.ActionInsertRune, Rune: mh.leaderKey}, nil)
		mh.resetLeaderState() // Reset state after inserting literal leader
		// Continue processing the current key normally
	}

	// --- Leader Key Pressed? ---
	if actionEvent.Action == input.ActionInsertRune && actionEvent.Rune == mh.leaderKey && mh.currentMode == ModeNormal {
		// Enter waiting state for leader sequence
		mh.leaderWaiting = true
		mh.statusBar.SetTemporaryMessage("<leader>")
		logger.Debugf("Entered leader waiting state")

		// Start timeout timer
		mh.leaderTimer = time.AfterFunc(input.LeaderTimeout, func() {
			// This executes in a separate goroutine
			mh.handleLeaderTimeout()
		})

		actionProcessed = true // Starting leader sequence is an action
		needsRedraw = true     // Show leader indicator in status bar
		return needsRedraw     // Don't process leader key as regular insert
	}

	// --- Normal Action Processing ---
	switch mh.currentMode {
	case ModeNormal:
		actionProcessed = mh.executeAction(actionEvent.Action, actionEvent, ev)
	case ModeCommand:
		actionProcessed = mh.handleActionCommand(actionEvent)
	case ModeFind:
		actionProcessed = mh.handleActionFind(actionEvent)
	default:
		logger.Debugf("Warning: Unknown input mode: %v", mh.currentMode)
		actionProcessed = false
	}

	needsRedraw = actionProcessed || (actionEvent.Action == input.ActionQuit && mh.forceQuitPending)
	return needsRedraw
}

// RegisterCommand adds a command to the registry. Called via EditorAPI.
func (mh *ModeHandler) RegisterCommand(name string, cmdFunc plugin.CommandFunc) error {
	if name == "" {
		return fmt.Errorf("command name cannot be empty")
	}
	if _, exists := mh.commands[name]; exists {
		return fmt.Errorf("command '%s' already registered", name)
	}
	mh.commands[name] = cmdFunc
	logger.Debugf("ModeHandler: Registered command ':%s'", name)
	return nil
}

// GetCurrentMode returns the current input mode.
func (mh *ModeHandler) GetCurrentMode() InputMode {
	return mh.currentMode
}

// GetCurrentModeString returns the current input mode as a user-friendly string.
func (mh *ModeHandler) GetCurrentModeString() string {
	switch mh.currentMode {
	case ModeNormal:
		return "NORMAL"
	case ModeCommand:
		return "COMMAND"
	case ModeFind:
		return "FIND"
	// Add other modes later
	default:
		return "UNKNOWN"
	}
}

// GetCommandBuffer returns the current command buffer content (e.g., for display).
func (mh *ModeHandler) GetCommandBuffer() string {
	// Only relevant in command mode, but safe to return otherwise
	if mh.currentMode == ModeCommand {
		return mh.cmdBuffer
	}
	return ""
}

// GetFindBuffer returns the find buffer content.
func (mh *ModeHandler) GetFindBuffer() string {
	if mh.currentMode == ModeFind {
		return mh.findBuffer
	}
	return ""
}

// handleLeaderTimeout resets leader state after timeout and inserts the literal leader key
func (mh *ModeHandler) handleLeaderTimeout() {
	mh.resetLeaderState()
	mh.statusBar.ResetTemporaryMessage()
	// Insert the literal leader key
	_ = mh.executeAction(input.ActionInsertRune, input.ActionEvent{Action: input.ActionInsertRune, Rune: mh.leaderKey}, nil)
}
