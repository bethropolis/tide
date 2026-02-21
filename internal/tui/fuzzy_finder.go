package tui

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/bethropolis/tide/internal/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/sahilm/fuzzy"
)

// FuzzyFinder is an overlay for finding files
type FuzzyFinder struct {
	active       bool
	searchTerm   string
	allFiles     []string
	matches      fuzzy.Matches
	selectedIndex int
	onSelect     func(filePath string)
	mu           sync.Mutex
	isIndexing   bool
}

// NewFuzzyFinder creates a new instance
func NewFuzzyFinder(onSelect func(filePath string)) *FuzzyFinder {
	f := &FuzzyFinder{
		onSelect: onSelect,
		allFiles: make([]string, 0),
	}
	return f
}

// Toggle activates or deactivates the finder
func (f *FuzzyFinder) Toggle(rootPath string) {
	f.active = !f.active
	if f.active {
		f.searchTerm = ""
		f.selectedIndex = 0
		f.indexFiles(rootPath)
		f.updateMatches()
	}
}

func (f *FuzzyFinder) indexFiles(root string) {
	if f.isIndexing {
		return
	}
	f.isIndexing = true
	go func() {
		defer func() { f.isIndexing = false }()
		var files []string
		
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			
			// Skip .git and node_modules by default
			if info.IsDir() && (info.Name() == ".git" || info.Name() == "node_modules" || info.Name() == "vendor") {
				return filepath.SkipDir
			}
			
			if !info.IsDir() {
				relPath, _ := filepath.Rel(root, path)
				files = append(files, relPath)
			}
			return nil
		})
		
		f.mu.Lock()
		f.allFiles = files
		f.updateMatches()
		f.mu.Unlock()
	}()
}

func (f *FuzzyFinder) updateMatches() {
	if f.searchTerm == "" {
		// Just show the first few files
		f.matches = nil
		f.selectedIndex = 0
		return
	}
	f.matches = fuzzy.Find(f.searchTerm, f.allFiles)
	if f.selectedIndex >= len(f.matches) {
		f.selectedIndex = len(f.matches) - 1
		if f.selectedIndex < 0 {
			f.selectedIndex = 0
		}
	}
}

func (f *FuzzyFinder) IsActive() bool {
	return f.active
}

func (f *FuzzyFinder) HandleKeyEvent(ev *tcell.EventKey) bool {
	if !f.active {
		return false
	}
	
	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyCtrlC:
		f.active = false
		return true
		
	case tcell.KeyEnter:
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.matches) > 0 && f.selectedIndex < len(f.matches) {
			path := f.matches[f.selectedIndex].Str
			f.active = false
			if f.onSelect != nil {
				f.onSelect(path)
			}
		} else if f.searchTerm == "" && len(f.allFiles) > 0 && f.selectedIndex < len(f.allFiles) {
			path := f.allFiles[f.selectedIndex]
			f.active = false
			if f.onSelect != nil {
				f.onSelect(path)
			}
		}
		return true
		
	case tcell.KeyUp, tcell.KeyCtrlK:
		f.selectedIndex--
		if f.selectedIndex < 0 {
			f.selectedIndex = 0
		}
		return true
		
	case tcell.KeyDown, tcell.KeyCtrlJ:
		f.selectedIndex++
		max := len(f.matches)
		if f.searchTerm == "" {
			max = len(f.allFiles)
		}
		if max > 0 && f.selectedIndex >= max {
			f.selectedIndex = max - 1
		}
		return true
		
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if len(f.searchTerm) > 0 {
			f.searchTerm = f.searchTerm[:len(f.searchTerm)-1]
			f.mu.Lock()
			f.updateMatches()
			f.mu.Unlock()
		}
		return true
		
	case tcell.KeyRune:
		f.searchTerm += string(ev.Rune())
		f.mu.Lock()
		f.updateMatches()
		f.mu.Unlock()
		return true
	}
	
	return true // consume everything while active
}

func (f *FuzzyFinder) Draw(screen tcell.Screen, th *theme.Theme, screenW, screenH int) {
	if !f.active {
		return
	}
	
	f.mu.Lock()
	defer f.mu.Unlock()
	
	// Layout logic (centered box, 60% width, 50% height)
	boxW := int(float64(screenW) * 0.6)
	boxH := int(float64(screenH) * 0.5)
	if boxW < 40 { boxW = 40 }
	if boxH < 10 { boxH = 10 }
	if boxW > screenW { boxW = screenW }
	if boxH > screenH { boxH = screenH }
	
	x := (screenW - boxW) / 2
	y := (screenH - boxH) / 2
	
	style := th.GetStyle("Default")
	DrawBox(screen, x, y, boxW, boxH, style)
	
	// Draw search prompt
	prompt := "> " + f.searchTerm
	DrawText(screen, x+2, y+1, boxW-4, prompt, style.Bold(true))
	DrawText(screen, x+2+len(prompt), y+1, 1, "_", style.Reverse(true)) // cursor
	
	// Draw separator
	for col := x+1; col < x+boxW-1; col++ {
		screen.SetContent(col, y+2, '─', nil, style)
	}
	
	// Draw items
	maxItems := boxH - 4
	listY := y + 3
	
	items := make([]string, 0)
	if f.searchTerm == "" {
		for i := 0; i < maxItems && i < len(f.allFiles); i++ {
			items = append(items, f.allFiles[i])
		}
	} else {
		for i := 0; i < maxItems && i < len(f.matches); i++ {
			items = append(items, f.matches[i].Str)
		}
	}
	
	for i, item := range items {
		itemStyle := style
		if i == f.selectedIndex {
			itemStyle = style.Reverse(true)
		}
		
		displayTxt := item
		if len(displayTxt) > boxW-4 {
			displayTxt = displayTxt[:boxW-7] + "..."
		}
		
		DrawText(screen, x+2, listY+i, boxW-4, displayTxt, itemStyle)
		
		// Fill rest of line with space so reverse style is clean
		for col := x+2+len(displayTxt); col < x+boxW-1; col++ {
			screen.SetContent(col, listY+i, ' ', nil, itemStyle)
		}
	}
	
	// Indexing indicator
	if f.isIndexing {
		DrawText(screen, x+boxW-12, y+boxH-1, 10, "Indexing...", style.Foreground(tcell.ColorYellow))
	}
}
