// internal/tui/completion.go
package tui

import (
	"sort"
	"strings"

	"github.com/bethropolis/tide/internal/theme"
	"github.com/gdamore/tcell/v2"
)

// CompletionItem represents one autocomplete suggestion rendered next to
// the cursor.  InsertText is the literal string inserted (after the
// existing prefix), InsertLen is the number of extra characters added.
type CompletionItem struct {
	InsertText string // full identifier to insert
	ReplaceLen int    // characters from the partial word already on screen that are replaced
	Label      string // display label (defaults to InsertText)
}

// CompletionOverlay is a small floating suggestion list shown in insert
// mode when an identifier prefix has a set of matches.  It implements the
// Overlay interface so the main event loop can route keys to it.
type CompletionOverlay struct {
	Active      bool
	Items       []CompletionItem
	Prefix      string // typed prefix that produced these items
	AnchorLine  int    // buffer line where the popup sits
	AnchorCol   int    // buffer column where the popup starts
	SelectedIdx int
	OnAccept    func(item CompletionItem)
}

// IsActive satisfies Overlay.
func (c *CompletionOverlay) IsActive() bool { return c.Active }

// Activate opens the overlay.  Items must already be sorted by the caller.
func (c *CompletionOverlay) Activate(prefix string, anchorLine, anchorCol int, items []CompletionItem) {
	c.Prefix = prefix
	c.AnchorLine = anchorLine
	c.AnchorCol = anchorCol
	c.Items = items
	c.SelectedIdx = 0
	c.Active = true
}

// Cancel hides the overlay without accepting.
func (c *CompletionOverlay) Cancel() { c.Active = false }

// Update replaces the current item list depending on the latest prefix.
func (c *CompletionOverlay) Update(prefix string, anchorLine, anchorCol int, items []CompletionItem) {
	c.Prefix = prefix
	c.AnchorLine = anchorLine
	c.AnchorCol = anchorCol
	c.Items = items
	if c.SelectedIdx >= len(items) {
		c.SelectedIdx = 0
	}
}

// HandleKeyEvent satisfies Overlay.  Returns true while active so the
// editor stops processing the same keys.
func (c *CompletionOverlay) HandleKeyEvent(ev *tcell.EventKey) bool {
	if !c.Active {
		return false
	}
	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyCtrlC:
		c.Cancel()
		return true
	case tcell.KeyEnter, tcell.KeyTab:
		c.accept()
		return true
	case tcell.KeyUp, tcell.KeyCtrlK:
		if c.SelectedIdx > 0 {
			c.SelectedIdx--
		}
		return true
	case tcell.KeyDown, tcell.KeyCtrlJ:
		if c.SelectedIdx < len(c.Items)-1 {
			c.SelectedIdx++
		}
		return true
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		// Backspace is consumed but not handled inside the completion list —
		// the caller will rebuild the list after the editor applies the
		// delete.  We just keep the overlay open.
		return true
	case tcell.KeyRune:
		// Letter/digit keys are also passed through — the editor will
		// rebuild the list after the rune is inserted.  We just keep the
		// overlay open here.
		return true
	}
	return true
}

// accept commits the currently-selected item.
func (c *CompletionOverlay) accept() {
	if len(c.Items) == 0 {
		c.Active = false
		return
	}
	item := c.Items[c.SelectedIdx]
	c.Active = false
	if c.OnAccept != nil {
		c.OnAccept(item)
	}
}

// Draw paints the overlay as a small floating box anchored at the given
// (anchorCol, anchorLine) text position.
func (c *CompletionOverlay) Draw(screen tcell.Screen, th *theme.Theme, screenW, screenH int) {
	if !c.Active || len(c.Items) == 0 {
		return
	}

	maxRows := 8
	if maxRows > len(c.Items) {
		maxRows = len(c.Items)
	}

	maxLabelLen := 0
	for _, it := range c.Items {
		label := it.Label
		if label == "" {
			label = it.InsertText
		}
		if len(label) > maxLabelLen {
			maxLabelLen = len(label)
		}
	}
	boxW := maxLabelLen + 4
	if boxW < 8 {
		boxW = 8
	}
	if boxW > screenW {
		boxW = screenW
	}
	boxH := maxRows + 2

	x := c.AnchorCol + 1
	y := c.AnchorLine + 1
	if x+boxW >= screenW {
		x = screenW - boxW - 1
	}
	if y+boxH >= screenH {
		y = screenH - boxH - 1
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	style := th.GetStyle("Default")
	DrawBox(screen, x, y, boxW, boxH, style)

	for i := 0; i < maxRows; i++ {
		it := c.Items[i]
		label := it.Label
		if label == "" {
			label = it.InsertText
		}
		itemStyle := style
		if i == c.SelectedIdx {
			itemStyle = style.Reverse(true)
		}
		DrawText(screen, x+1, y+i+1, boxW-2, label, itemStyle)
		drawn := len([]rune(label))
		for col := x + 1 + drawn; col < x+boxW-1; col++ {
			screen.SetContent(col, y+i+1, ' ', nil, itemStyle)
		}
	}
}

// FilterSymbols applies a case-insensitive prefix filter on candidates and
// returns a sorted slice of CompletionItem.  replaceLen is the number of
// characters in the typed prefix that will be overwritten by the inserted
// candidate — usually the original prefix length.
func FilterSymbols(prefix string, candidates []string, replaceLen int) []CompletionItem {
	prefixLower := strings.ToLower(prefix)
	seen := make(map[string]struct{})
	out := make([]CompletionItem, 0, len(candidates))
	for _, c := range candidates {
		if strings.Contains(strings.ToLower(c), prefixLower) {
			if _, dup := seen[c]; dup {
				continue
			}
			seen[c] = struct{}{}
			label := c
			if strings.HasPrefix(strings.ToLower(c), prefixLower) {
				// Only show the suffix as the highlighted detail when the
				// prefix is already on screen.
				if len(c) > replaceLen {
					label = c[replaceLen:]
				} else {
					label = c
				}
			}
			out = append(out, CompletionItem{
				InsertText: c,
				ReplaceLen: replaceLen,
				Label:      label,
			})
		}
	}
	sort.SliceStable(out, func(i, k int) bool {
		iExact := strings.HasPrefix(strings.ToLower(out[i].InsertText), prefixLower)
		kExact := strings.HasPrefix(strings.ToLower(out[k].InsertText), prefixLower)
		if iExact != kExact {
			return iExact
		}
		return out[i].InsertText < out[k].InsertText
	})
	return out
}