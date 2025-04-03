// internal/input/action.go
package input

// Action represents a command or operation to be performed by the editor.
type Action int

// Define the set of possible editor actions.
const (
	// --- Meta Actions ---
	ActionUnknown Action = iota // Default/invalid action
	ActionQuit
	ActionForceQuit // Quit without checking modified status
	ActionSave

	// --- Cursor Movement ---
	ActionMoveUp
	ActionMoveDown
	ActionMoveLeft
	ActionMoveRight
	ActionMovePageUp
	ActionMovePageDown
	ActionMoveHome // Beginning of line
	ActionMoveEnd  // End of line
	// TODO: ActionMoveFileStart, ActionMoveFileEnd

	// --- Text Manipulation ---
	ActionInsertRune    // Requires Rune argument
	ActionInsertNewLine // Specific action for Enter
	ActionDeleteCharForward  // Delete key
	ActionDeleteCharBackward // Backspace key
	// TODO: ActionDeleteWordForward, ActionDeleteWordBackward, ActionYank, ActionPaste

	// --- Editor Mode ---
	ActionEnterCommandMode // Special action for ':'
	ActionExecuteCommand   // Special action for Enter in Command Mode
	ActionCancelCommand    // Special action for Esc in Command Mode
	ActionAppendCommand    // Special action for runes in Command Mode
	ActionDeleteCommandChar // Special action for Backspace in Command Mode

	// --- Viewport / Other ---
	// ActionScrollUp? ActionScrollDown? (Usually tied to cursor movement)
	// ActionFind?
	// ActionToggleHelp?
)

// ActionEvent represents a decoded input event resulting in an action.
// It might carry payload data needed for the action (like the rune to insert).
type ActionEvent struct {
	Action Action
	Rune   rune // Used for ActionInsertRune
	// Add other fields later if needed (e.g., Count for repeating actions)
}