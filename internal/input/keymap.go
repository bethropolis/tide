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
	ctrlMap[tcell.KeyCtrlY] = ActionYank      // Add Yank binding
	ctrlMap[tcell.KeyCtrlP] = ActionPaste     // Add Paste binding

	p.modKeymap[tcell.ModCtrl] = ctrlMap

	// --- Rune Mappings ---
	p.runeKeymap[':'] = ActionEnterCommandMode // Trigger command mode
	p.runeKeymap['/'] = ActionEnterFindMode    // Map '/' to Find Mode

	// --- Normal Mode Specific Keys (can be handled by ModeHandler based on action) ---
	// Map 'n' and 'N' to their actions. ModeHandler will check mode before acting.
	p.runeKeymap['n'] = ActionFindNext
	p.runeKeymap['N'] = ActionFindPrevious

	// Default for other runes is handled in ProcessEvent
}

// ProcessEvent takes a tcell key event and returns the corresponding ActionEvent.
func (p *InputProcessor) ProcessEvent(ev *tcell.EventKey) ActionEvent {
	key := ev.Key()
	mod := ev.Modifiers()
	runeVal := ev.Rune()

	// 1. Check Modifier + Key combinations (Ctrl+S, etc.) - Keep this
	if mod&tcell.ModCtrl != 0 || mod&tcell.ModAlt != 0 { // Check Ctrl or Alt explicitly
		if modKeyMap, modOk := p.modKeymap[mod]; modOk {
			if action, keyOk := modKeyMap[key]; keyOk {
				return ActionEvent{Action: action} // Return action WITH modifier info implicitly handled
			}
		}
		// If Ctrl/Alt + Rune, potentially block default insert? Return Unknown for now.
		if key == tcell.KeyRune {
			return ActionEvent{Action: ActionUnknown}
		}
	}
	// We handle Shift modifier below for specific keys

	// 2. Check simple Key mappings (Arrows, PgUp/Dn, Home, End, Del, Esc...)
	// We *don't* filter out ModShift here anymore. Let the action handler decide based on Shift.
	if action, ok := p.keymap[key]; ok {
		// Pass the original event (including modifiers like Shift)
		// The action handler (ModeHandler) will check ev.Modifiers()
		return ActionEvent{Action: action} // Return the base action (e.g., ActionMoveUp)
	}

	// 3. Check Rune mappings (like ':') - Keep this
	if key == tcell.KeyRune && mod == tcell.ModNone { // Only handle plain runes here
		if action, ok := p.runeKeymap[runeVal]; ok {
			return ActionEvent{Action: action, Rune: runeVal}
		}
		return ActionEvent{Action: ActionInsertRune, Rune: runeVal}
	}

	// Handle Enter (pass action, let handler decide based on mode)
	if key == tcell.KeyEnter {
		return ActionEvent{Action: ActionInsertNewLine}
	}

	// 4. No mapping found
	return ActionEvent{Action: ActionUnknown}
}
