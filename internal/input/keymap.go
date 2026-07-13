// internal/input/keymap.go
package input

import (
	"fmt"
	"time"

	"github.com/bethropolis/tide/internal/config"
	"github.com/gdamore/tcell/v2"
)

// DefaultLeaderKey is the default key used to initiate sequences.
const DefaultLeaderKey = ',' // Using comma as leader key

// LeaderTimeout is the duration to wait for the second key in a sequence.
const LeaderTimeout = 500 * time.Millisecond

// Keymap maps specific key events to editor actions.
// We use a simple map for now. Could evolve to handle sequences/modes later.
type Keymap map[tcell.Key]Action        // For special keys (Enter, Arrows, etc.)
type RuneKeymap map[rune]Action         // For simple rune bindings (rarely needed beyond insert)
type ModKeymap map[tcell.ModMask]Keymap // For keys combined with modifiers (Ctrl, Alt, Shift)
// LeaderSequenceMap maps the key following the leader to an action
type LeaderSequenceMap map[rune]Action // For keys that follow the leader key

// InputProcessor translates tcell events into ActionEvents.
type InputProcessor struct {
	keymap     Keymap
	runeKeymap RuneKeymap // Primarily for ActionInsertRune default
	modKeymap  ModKeymap
	leaderMap  LeaderSequenceMap // Maps keys following the leader
	leaderKey  rune              // Configurable leader key
	// TODO: Add state for multi-key sequences (e.g., leader keys)
}

// NewInputProcessor creates a processor with default keybindings.
func NewInputProcessor() *InputProcessor {
	p := &InputProcessor{
		keymap:     make(Keymap),
		runeKeymap: make(RuneKeymap),
		modKeymap:  make(ModKeymap),
		leaderMap:  make(LeaderSequenceMap),
		leaderKey:  DefaultLeaderKey, // Use default leader key
	}
	p.loadDefaultBindings()
	return p
}

// loadDefaultBindings sets up the initial key mappings.
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
	p.keymap[tcell.KeyTab] = ActionInsertTab
	p.keymap[tcell.KeyBacktab] = ActionInsertBacktab
	p.keymap[tcell.KeyBackspace] = ActionDeleteCharBackward
	p.keymap[tcell.KeyBackspace2] = ActionDeleteCharBackward
	p.keymap[tcell.KeyDelete] = ActionDeleteCharForward
	p.keymap[tcell.KeyEscape] = ActionQuit
	p.keymap[tcell.KeyCtrlC] = ActionQuit

	// --- Modifier Keys (Ctrl) ---
	ctrlMap := make(Keymap)
	ctrlMap[tcell.KeyCtrlS] = ActionSave
	ctrlMap[tcell.KeyCtrlQ] = ActionForceQuit
	ctrlMap[tcell.KeyCtrlZ] = ActionUndo
	ctrlMap[tcell.KeyCtrlY] = ActionRedo
	ctrlMap[tcell.KeyCtrlR] = ActionRedo
	ctrlMap[tcell.KeyCtrlX] = ActionYank
	ctrlMap[tcell.KeyCtrlV] = ActionEnterVisualBlockMode
	ctrlMap[tcell.KeyCtrlA] = ActionMoveHome
	ctrlMap[tcell.KeyCtrlE] = ActionMoveEnd
	p.modKeymap[tcell.ModCtrl] = ctrlMap

	// --- Leader Key Sequences ---
	p.leaderMap['/'] = ActionEnterFindMode
	p.leaderMap[':'] = ActionEnterCommandMode
	p.leaderMap['f'] = ActionFuzzyFind
	p.leaderMap['n'] = ActionFindNext
	p.leaderMap['N'] = ActionFindPrevious
	p.leaderMap['w'] = ActionSave
	p.leaderMap['q'] = ActionQuit
	p.leaderMap['u'] = ActionUndo
	p.leaderMap['r'] = ActionRedo
	p.leaderMap['y'] = ActionYank
	p.leaderMap['p'] = ActionPaste
	p.leaderMap['d'] = ActionCut
	p.leaderMap['x'] = ActionCut
}

// setModeBinding parses a key string → action name pair and installs the
// result into the correct mode layer. The mode name matches the fields of
// config.KeybindConfig ("normal", "insert", "command", "find", "visual",
// "visual_line").
func (p *InputProcessor) setModeBinding(_ string, keyStr, actionName string) error {
	ks, err := ParseKeyString(keyStr)
	if err != nil {
		return fmt.Errorf("invalid key %q: %w", keyStr, err)
	}
	action, ok := ActionFromName(actionName)
	if !ok {
		return fmt.Errorf("unknown action %q", actionName)
	}

	// Place into the appropriate map based on key/run/mod composition.
	if ks.Key == tcell.KeyRune && ks.Mod == 0 {
		p.runeKeymap[ks.Rune] = action
		return nil
	}
	if ks.Mod != 0 {
		if p.modKeymap[ks.Mod] == nil {
			p.modKeymap[ks.Mod] = make(Keymap)
		}
		p.modKeymap[ks.Mod][ks.Key] = action
		return nil
	}
	p.keymap[ks.Key] = action
	return nil
}

// LoadUserBindings clears all overridable maps, re-applies defaults, then
// applies each binding from cfg. Returns an error if any binding fails to
// parse; partial updates are preserved for those that succeeded.
func (p *InputProcessor) LoadUserBindings(cfg *config.KeybindConfig) error {
	// Rebuild defaults as a clean base.
	p.keymap = make(Keymap)
	p.runeKeymap = make(RuneKeymap)
	p.modKeymap = make(ModKeymap)
	p.leaderMap = make(LeaderSequenceMap)
	p.loadDefaultBindings()

	if cfg == nil {
		return nil
	}

	for _, mode := range []map[string]string{cfg.Normal, cfg.Insert, cfg.Command, cfg.Find, cfg.Visual, cfg.VisualLine} {
		for keyStr, actionName := range mode {
			if err := p.setModeBinding("", keyStr, actionName); err != nil {
				return err
			}
		}
	}

	return nil
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

	// 3. Check Rune mappings
	// Note: We don't check for ModNone here because Shift might be applied
	// for uppercase letters or symbols (like ':'). Ctrl and Alt are handled above.
	if key == tcell.KeyRune {
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

// GetLeaderKey returns the configured leader key rune.
func (p *InputProcessor) GetLeaderKey() rune {
	return p.leaderKey
}

// IsLeaderSequenceKey checks if a rune completes a known leader sequence.
func (p *InputProcessor) IsLeaderSequenceKey(r rune) (Action, bool) {
	action, exists := p.leaderMap[r]
	return action, exists
}
