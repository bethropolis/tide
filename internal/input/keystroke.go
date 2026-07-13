// internal/input/keystroke.go
package input

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// KeyStroke holds the decoded components of a key combination from config.
type KeyStroke struct {
	Key  tcell.Key
	Mod  tcell.ModMask
	Rune rune
}

// ParseKeyString converts a config string like "ctrl+s" or "escape" into
// the raw key components understood by the InputProcessor.
//
// Supported modifiers (joined with '+'): ctrl, alt, shift.
// Recognized special keys: escape/esc, enter/return, tab, backspace,
// delete/del, up, down, left, right, home, end, pgup, pgdn.
// Any single-character string is treated as a rune (case preserved).
func ParseKeyString(s string) (KeyStroke, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return KeyStroke{}, fmt.Errorf("empty key string")
	}

	var mod tcell.ModMask
	parts := splitModifiers(s)
	for i := 0; i < len(parts)-1; i++ {
		switch strings.ToLower(strings.TrimSpace(parts[i])) {
		case "ctrl":
			mod |= tcell.ModCtrl
		case "alt":
			mod |= tcell.ModAlt
		case "shift":
			mod |= tcell.ModShift
		default:
			return KeyStroke{}, fmt.Errorf("unknown modifier %q", parts[i])
		}
	}

	rawKey := parts[len(parts)-1]
	if rawKey == "" {
		return KeyStroke{}, fmt.Errorf("empty key after modifiers")
	}

	ks := KeyStroke{Mod: mod}

	switch lower := strings.ToLower(rawKey); lower {
	case "escape", "esc":
		ks.Key = tcell.KeyEscape
	case "enter", "return":
		ks.Key = tcell.KeyEnter
	case "tab":
		ks.Key = tcell.KeyTab
	case "backspace", "bs":
		ks.Key = tcell.KeyBackspace
	case "delete", "del":
		ks.Key = tcell.KeyDelete
	case "up":
		ks.Key = tcell.KeyUp
	case "down":
		ks.Key = tcell.KeyDown
	case "left":
		ks.Key = tcell.KeyLeft
	case "right":
		ks.Key = tcell.KeyRight
	case "home":
		ks.Key = tcell.KeyHome
	case "end":
		ks.Key = tcell.KeyEnd
	case "pgup", "pageup":
		ks.Key = tcell.KeyPgUp
	case "pgdn", "pagedown":
		ks.Key = tcell.KeyPgDn
	case "space":
		ks.Rune = ' '
		ks.Key = tcell.KeyRune
	default:
		runes := []rune(rawKey)
		if len(runes) == 1 {
			r := runes[0]
			ks.Rune = r
			ks.Key = tcell.KeyRune
			// Translate standard control characters Ctrl+a..Ctrl+z into
			// the corresponding tcell.KeyCtrlX constant. We only do this
			// when Ctrl is the *only* modifier — combinations like
			// Ctrl+Shift+s keep the rune and the modifier mask intact so
			// the InputProcessor can match them as a modifier+rune.
			if mod == tcell.ModCtrl {
				lr := r
				if lr >= 'A' && lr <= 'Z' {
					lr = lr - 'A' + 'a'
				}
				if lr >= 'a' && lr <= 'z' {
					ks.Key = tcell.Key(int(tcell.KeyCtrlA) + int(lr-'a'))
					ks.Rune = 0
					ks.Mod = 0
				}
			}
		} else {
			return KeyStroke{}, fmt.Errorf("unrecognized key %q", rawKey)
		}
	}

	return ks, nil
}

// splitModifiers splits a key spec on '+' but preserves the final segment
// (the key itself) verbatim, so single-character runes keep their case.
func splitModifiers(s string) []string {
	parts := strings.Split(s, "+")
	for i := range parts {
		if i < len(parts)-1 {
			parts[i] = strings.TrimSpace(parts[i])
		}
	}
	return parts
}
