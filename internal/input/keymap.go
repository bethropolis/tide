// internal/input/keymap.go
package input

import (
	"github.com/gdamore/tcell/v2"
)

// Keymap maps specific key events to editor actions.
// We use a simple map for now. Could evolve to handle sequences/modes later.
type Keymap map[tcell.Key]Action        // For special keys (Enter, Arrows, etc.)
type RuneKeymap map[rune]Action         // For simple rune bindings (rarely needed beyond insert)
type ModKeymap map[tcell.ModMask]Keymap // For keys combined with modifiers (Ctrl, Alt, Shift)

// InputProcessor translates tcell events into ActionEvents.
type InputProcessor struct {
	keymap     Keymap
	runeKeymap RuneKeymap // Primarily for ActionInsertRune default
	modKeymap  ModKeymap
	// TODO: Add state for multi-key sequences (e.g., leader keys)
}

// NewInputProcessor creates a processor with default keybindings.
func NewInputProcessor() *InputProcessor {
	p := &InputProcessor{
		keymap:     make(Keymap),
		runeKeymap: make(RuneKeymap),
		modKeymap:  make(ModKeymap),
	}
	p.loadDefaultBindings()
	return p
}

// loadDefaultBindings sets up the initial key mappings.
// This is where default keybindings are defined. TODO: Load from config later.
func (p *InputProcessor) loadDefaultBindings() {
	// --- Simple Keys ---
	p.keymap[tcell.KeyUp] = ActionMoveUp
	p.keymap[tcell.KeyDown] = ActionMoveDown
	p.keymap[tcell.KeyLeft] = ActionMoveLeft
	p.keymap[tcell.KeyRight] = ActionMoveRight
	p.keymap[tcell.KeyPgUp] = ActionMovePageUp
	p.keymap[tcell.KeyPgDn] = ActionMovePageDown
	p.keymap[tcell.KeyHome] = ActionMoveHome
	p.keymap[tcell.KeyEnd] = ActionMoveEnd
	// p.keymap[tcell.KeyEnter] = ActionInsertNewLine // Enter is handled differently by mode now
	p.keymap[tcell.KeyBackspace] = ActionDeleteCharBackward
	p.keymap[tcell.KeyBackspace2] = ActionDeleteCharBackward // Often used for Backspace
	p.keymap[tcell.KeyDelete] = ActionDeleteCharForward
	p.keymap[tcell.KeyEscape] = ActionQuit // Primary quit action (checks modified)
	p.keymap[tcell.KeyCtrlC] = ActionQuit  // Also try to quit gracefully

	// --- Modifier Keys (Example: Ctrl+S for Save) ---
	// Note: tcell ModMask combines modifiers (Ctrl | Shift etc.)
	ctrlMap := make(Keymap)
	ctrlMap[tcell.KeyCtrlS] = ActionSave
	// Add more Ctrl bindings here, e.g., Ctrl+Q for ForceQuit?
	ctrlMap[tcell.KeyCtrlQ] = ActionForceQuit // Example force quit

	p.modKeymap[tcell.ModCtrl] = ctrlMap

	// --- Rune Mappings (Special Case for :) ---
	p.runeKeymap[':'] = ActionEnterCommandMode // Trigger command mode

	// Default for other runes is handled in ProcessEvent
}

// ProcessEvent takes a tcell key event and returns the corresponding ActionEvent.
// INPUT MODE IS NOT HANDLED HERE - App decides based on mode + action.
func (p *InputProcessor) ProcessEvent(ev *tcell.EventKey) ActionEvent {
	key := ev.Key()
	mod := ev.Modifiers()
	runeVal := ev.Rune()

	// 1. Check Modifier + Key combinations
	if modKeyMap, modOk := p.modKeymap[mod]; modOk {
		if action, keyOk := modKeyMap[key]; keyOk {
			return ActionEvent{Action: action}
		}
		// Could also check mod + rune here if needed
	}
	// Clear modifier if it was part of a standard key name (like tcell.KeyCtrlS itself)
	// This prevents Ctrl+S from also being interpreted as just 's' if Ctrl map check fails
	if key >= tcell.KeyCtrlA && key <= tcell.KeyCtrlZ {
		mod &^= tcell.ModCtrl // Remove Ctrl modifier if the Key already implies it
	}
	// Similar checks for Alt if needed

	// 2. Check simple Key mappings (no significant modifiers or handled above)
	if mod == tcell.ModNone || mod == tcell.ModShift { // Allow Shift with arrows etc.
		if action, ok := p.keymap[key]; ok {
			// Special keys that might depend on mode
			// Let App handle mode-specific interpretation of Esc, Enter, Backspace
			// based on the action returned here.
			return ActionEvent{Action: action}
		}
	}

	// 3. Check Rune mappings (like ':')
	if key == tcell.KeyRune && mod == tcell.ModNone { // Only insert plain runes (no Ctrl+rune etc.)
		if action, ok := p.runeKeymap[runeVal]; ok { // Check specific rune map first (e.g., ':')
			return ActionEvent{Action: action, Rune: runeVal}
		}
		// Default: Treat as rune insertion *request*
		// App will decide whether to insert into buffer or command line
		return ActionEvent{Action: ActionInsertRune, Rune: runeVal}
	}

	// Handle Enter and Backspace specifically (as they weren't mapped above for runes)
	// Let App interpret these based on mode. Return generic actions.
	if key == tcell.KeyEnter {
		return ActionEvent{Action: ActionInsertNewLine} // Default intention is newline
	}
	// Backspace already handled by keymap check above

	// 4. No mapping found
	return ActionEvent{Action: ActionUnknown}
}
