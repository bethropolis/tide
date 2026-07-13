// internal/tui/picker.go
package tui

import (
	"strings"

	"github.com/bethropolis/tide/internal/theme"
	"github.com/gdamore/tcell/v2"
)

// PickerItem represents a single row in the selection list.
type PickerItem struct {
	Label       string
	Description string
	Value       string
}

// Picker is a generic, reusable list-selection overlay.  Plugins (including
// Lua scripts) can use it to present menus, buffer switchers, palette
// systems, etc. without reimplementing keyboard navigation.
//
// It implements the Overlay interface so it slots into the existing drawing
// and input-dispatch paths the same way FuzzyFinder does.
type Picker struct {
	Active        bool
	Title         string
	Items         []PickerItem
	Filtered      []PickerItem // subset after fuzzy filtering
	SelectedIndex int
	ScrollOffset  int // first visible row in the filtered list
	SearchTerm    string
	OnSelect      func(val string)
	OnCancel      func()
}

// NewPicker creates a ready-to-use picker.
func NewPicker(title string, items []PickerItem, onSelect func(val string)) *Picker {
	p := &Picker{
		Title:    title,
		Items:    items,
		Filtered: items,
		OnSelect: onSelect,
	}
	return p
}

func (p *Picker) IsActive() bool { return p.Active }

// Activate opens the picker.  Resets internal state and starts with the full
// unfiltered list.
func (p *Picker) Activate() {
	p.Active = true
	p.SelectedIndex = 0
	p.ScrollOffset = 0
	p.SearchTerm = ""
	p.Filtered = p.Items
}

// Cancel closes the picker without making a selection.
func (p *Picker) Cancel() {
	p.Active = false
	if p.OnCancel != nil {
		p.OnCancel()
	}
}

// HandleKeyEvent implements the Overlay interface.  Returns true while the
// picker is visible — every keystroke is consumed by the picker, preventing
// the editor from processing it underneath.
func (p *Picker) HandleKeyEvent(ev *tcell.EventKey) bool {
	if !p.Active {
		return false
	}

	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyCtrlC:
		p.Cancel()
		return true

	case tcell.KeyEnter:
		if len(p.Filtered) > 0 && p.SelectedIndex < len(p.Filtered) {
			p.Active = false
			if p.OnSelect != nil {
				p.OnSelect(p.Filtered[p.SelectedIndex].Value)
			}
		}
		return true

	case tcell.KeyUp, tcell.KeyCtrlK:
		if p.SelectedIndex > 0 {
			p.SelectedIndex--
		}
		return true

	case tcell.KeyDown, tcell.KeyCtrlJ:
		if p.SelectedIndex < len(p.Filtered)-1 {
			p.SelectedIndex++
		}
		return true

	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if len(p.SearchTerm) > 0 {
			p.SearchTerm = p.SearchTerm[:len(p.SearchTerm)-1]
			p.applyFilter()
		}
		return true

	case tcell.KeyRune:
		p.SearchTerm += string(ev.Rune())
		p.applyFilter()
		return true
	}

	return true
}

// applyFilter narrows Items → Filtered by case-insensitive prefix match on
// Label and resets SelectedIndex to 0.
func (p *Picker) applyFilter() {
	if p.SearchTerm == "" {
		p.Filtered = p.Items
	} else {
		term := strings.ToLower(p.SearchTerm)
		var kept []PickerItem
		for _, it := range p.Items {
			if strings.Contains(strings.ToLower(it.Label), term) {
				kept = append(kept, it)
			}
		}
		p.Filtered = kept
	}
	p.SelectedIndex = 0
	p.ScrollOffset = 0
}

// Draw renders the picker overlay at the centre of the terminal.
func (p *Picker) Draw(screen tcell.Screen, th *theme.Theme, screenW, screenH int) {
	if !p.Active {
		return
	}

	boxW := int(float64(screenW) * 0.6)
	boxH := int(float64(screenH) * 0.4)
	if boxW < 40 {
		boxW = 40
	}
	if boxH < 6 {
		boxH = 6
	}
	if boxW > screenW {
		boxW = screenW
	}
	if boxH > screenH {
		boxH = screenH
	}

	x := (screenW - boxW) / 2
	y := (screenH - boxH) / 2

	style := th.GetStyle("Default")
	DrawBox(screen, x, y, boxW, boxH, style)

	// Title row: show title + count indicator
	titleStr := p.Title
	countStr := ""
	if len(p.Filtered) > 0 {
		countStr = string(rune(p.SelectedIndex+1)) + "/" + string(rune(len(p.Filtered)))
	}
	DrawText(screen, x+2, y, boxW-4, titleStr, style.Bold(true))
	if countStr != "" {
		DrawText(screen, x+boxW-1-len(countStr)-1, y, len(countStr)+1, " "+countStr, style.Dim(true))
	}

	// Search prompt
	prompt := "> " + p.SearchTerm
	DrawText(screen, x+2, y+1, boxW-4, prompt, style.Bold(true))
	cursorX := x + 2 + len(prompt)
	if cursorX < x+boxW-1 {
		DrawText(screen, cursorX, y+1, 1, "_", style.Reverse(true))
	}

	// Separator
	for col := x + 1; col < x+boxW-1; col++ {
		screen.SetContent(col, y+2, '─', nil, style)
	}

	maxItems := boxH - 4
	listY := y + 3

	// Scroll adjustments
	if p.SelectedIndex < p.ScrollOffset {
		p.ScrollOffset = p.SelectedIndex
	}
	if p.SelectedIndex >= p.ScrollOffset+maxItems {
		p.ScrollOffset = p.SelectedIndex - maxItems + 1
	}
	if p.ScrollOffset < 0 {
		p.ScrollOffset = 0
	}

	for i := 0; i < maxItems; i++ {
		itemIdx := p.ScrollOffset + i
		if itemIdx >= len(p.Filtered) {
			break
		}
		it := p.Filtered[itemIdx]

		itemStyle := style
		if itemIdx == p.SelectedIndex {
			itemStyle = style.Reverse(true)
		}

		display := it.Label
		if it.Description != "" {
			display += "  " + it.Description
		}

		txt := display
		maxTxt := boxW - 4
		if len([]rune(txt)) > maxTxt {
			runes := []rune(txt)
			txt = string(runes[:maxTxt-3]) + "..."
		}

		DrawText(screen, x+2, listY+i, boxW-4, txt, itemStyle)

		// Fill remainder so reverse-video highlight is clean
		drawn := len([]rune(txt))
		for col := x + 2 + drawn; col < x+boxW-1; col++ {
			screen.SetContent(col, listY+i, ' ', nil, itemStyle)
		}
	}
}