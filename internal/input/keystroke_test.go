// internal/input/keystroke_test.go
package input

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestParseKeyString(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    KeyStroke
		wantErr bool
	}{
		{name: "ctrl+s", in: "ctrl+s", want: KeyStroke{Key: tcell.KeyCtrlS}},
		{name: "ctrl+z", in: "ctrl+z", want: KeyStroke{Key: tcell.KeyCtrlZ}},
		{name: "escape", in: "escape", want: KeyStroke{Key: tcell.KeyEscape}},
		{name: "esc alias", in: "esc", want: KeyStroke{Key: tcell.KeyEscape}},
		{name: "enter", in: "enter", want: KeyStroke{Key: tcell.KeyEnter}},
		{name: "return alias", in: "return", want: KeyStroke{Key: tcell.KeyEnter}},
		{name: "tab", in: "tab", want: KeyStroke{Key: tcell.KeyTab}},
		{name: "backspace", in: "backspace", want: KeyStroke{Key: tcell.KeyBackspace}},
		{name: "delete", in: "delete", want: KeyStroke{Key: tcell.KeyDelete}},
		{name: "up", in: "up", want: KeyStroke{Key: tcell.KeyUp}},
		{name: "down", in: "down", want: KeyStroke{Key: tcell.KeyDown}},
		{name: "left", in: "left", want: KeyStroke{Key: tcell.KeyLeft}},
		{name: "right", in: "right", want: KeyStroke{Key: tcell.KeyRight}},
		{name: "home", in: "home", want: KeyStroke{Key: tcell.KeyHome}},
		{name: "end", in: "end", want: KeyStroke{Key: tcell.KeyEnd}},
		{name: "pgup", in: "pgup", want: KeyStroke{Key: tcell.KeyPgUp}},
		{name: "pgdn", in: "pgdn", want: KeyStroke{Key: tcell.KeyPgDn}},
		{name: "single rune a", in: "a", want: KeyStroke{Key: tcell.KeyRune, Rune: 'a'}},
		{name: "single rune uppercase", in: "A", want: KeyStroke{Key: tcell.KeyRune, Rune: 'A'}},
		{name: "space", in: "space", want: KeyStroke{Key: tcell.KeyRune, Rune: ' '}},
		{name: "alt+x", in: "alt+x", want: KeyStroke{Key: tcell.KeyRune, Mod: tcell.ModAlt, Rune: 'x'}},
		{name: "shift+up", in: "shift+up", want: KeyStroke{Key: tcell.KeyUp, Mod: tcell.ModShift}},
		{name: "ctrl+shift+s keeps rune+mod", in: "ctrl+shift+s", want: KeyStroke{Key: tcell.KeyRune, Mod: tcell.ModCtrl | tcell.ModShift, Rune: 's'}},
		{name: "empty errors", in: "", wantErr: true},
		{name: "unknown modifier", in: "win+x", wantErr: true},
		{name: "too long", in: "abc", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseKeyString(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseKeyString(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if got.Key != tc.want.Key || got.Mod != tc.want.Mod || got.Rune != tc.want.Rune {
				t.Errorf("ParseKeyString(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}
