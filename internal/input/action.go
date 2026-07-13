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
	ActionMoveFileStart // Beginning of file (gg)
	ActionMoveFileEnd   // End of file (G)

	// --- Text Manipulation ---
	ActionInsertRune         // Requires Rune argument
	ActionInsertNewLine      // Specific action for Enter
	ActionInsertTab          // Specific action for Tab key
	ActionInsertBacktab      // Specific action for Shift+Tab key
	ActionDeleteCharForward  // Delete key
	ActionDeleteCharBackward // Backspace key
	ActionYank               // Copy selection to clipboard
	ActionCut                // Cut selection to clipboard
	ActionPaste              // Insert clipboard content
	ActionPasteBefore        // Insert clipboard content before cursor
	ActionUndo               // Undo last edit
	ActionRedo               // Redo previously undone edit
	ActionDeleteWordForward  // Delete word forward (dw)
	ActionDeleteWordBackward // Delete word backward (db)

	// --- Editor Mode ---
	ActionEnterNormalMode   // Special action to return to Normal Mode
	ActionEnterInsertMode   // Special action for 'i', 'a', etc.
	ActionEnterVisualMode   // Special action for 'v'
	ActionEnterVisualBlockMode // Special action for Ctrl+V
	ActionEnterCommandMode  // Special action for ':'
	ActionExecuteCommand    // Special action for Enter in Command Mode
	ActionCancelCommand     // Special action for Esc in Command Mode
	ActionAppendCommand     // Special action for runes in Command Mode
	ActionDeleteCommandChar // Special action for Backspace in Command Mode

	// --- find ---
	ActionEnterFindMode // Trigger find mode (e.g., '/')
	ActionFindNext      // Find next occurrence (e.g., 'n')
	ActionFindPrevious  // Find previous occurrence (e.g., 'N')
	ActionFuzzyFind     // Fuzzy find files

	// --- Viewport / Other ---
	// ActionScrollUp? ActionScrollDown? (Usually tied to cursor movement)
	// ActionFind?
	// ActionToggleHelp?
)

// ActionEvent represents a decoded input event resulting in an action.
type ActionEvent struct {
	Action Action
	Rune   rune // Used for ActionInsertRune
	Count  int  // Repeat count from numeric prefix (1 if none)
}

// actionNames maps action names (used in config TOML) to Actions.
// The names are lowercase, dot-free, human-readable identifiers.
var actionNames = map[string]Action{
	"unknown":           ActionUnknown,
	"quit":              ActionQuit,
	"force_quit":        ActionForceQuit,
	"save":              ActionSave,
	"move_up":           ActionMoveUp,
	"move_down":         ActionMoveDown,
	"move_left":         ActionMoveLeft,
	"move_right":        ActionMoveRight,
	"move_page_up":      ActionMovePageUp,
	"move_page_down":    ActionMovePageDown,
	"move_home":         ActionMoveHome,
	"move_end":          ActionMoveEnd,
	"move_file_start":   ActionMoveFileStart,
	"move_file_end":     ActionMoveFileEnd,
	"insert_rune":       ActionInsertRune,
	"insert_new_line":   ActionInsertNewLine,
	"insert_tab":        ActionInsertTab,
	"insert_backtab":   ActionInsertBacktab,
	"delete_char_forward":   ActionDeleteCharForward,
	"delete_char_backward":  ActionDeleteCharBackward,
	"yank":              ActionYank,
	"cut":               ActionCut,
	"paste":             ActionPaste,
	"paste_before":      ActionPasteBefore,
	"undo":              ActionUndo,
	"redo":              ActionRedo,
	"delete_word_forward":  ActionDeleteWordForward,
	"delete_word_backward": ActionDeleteWordBackward,
	"enter_normal":      ActionEnterNormalMode,
	"enter_insert":      ActionEnterInsertMode,
	"enter_visual":      ActionEnterVisualMode,
	"enter_visual_block": ActionEnterVisualBlockMode,
	"enter_command":     ActionEnterCommandMode,
	"execute_command":   ActionExecuteCommand,
	"cancel_command":    ActionCancelCommand,
	"append_command":    ActionAppendCommand,
	"delete_command_char": ActionDeleteCommandChar,
	"enter_find":        ActionEnterFindMode,
	"find_next":         ActionFindNext,
	"find_previous":     ActionFindPrevious,
	"fuzzy_find":        ActionFuzzyFind,
}

// ActionFromName resolves a config action name (e.g., "save") to an Action.
// Returns ActionUnknown and false when the name is not recognized.
func ActionFromName(name string) (Action, bool) {
	a, ok := actionNames[name]
	return a, ok
}

// NameFromAction returns the config name for an Action, or "" if unknown.
func NameFromAction(a Action) string {
	for name, act := range actionNames {
		if act == a {
			return name
		}
	}
	return ""
}
