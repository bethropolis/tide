// internal/app/app.go
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/commands"
	"github.com/bethropolis/tide/internal/config"
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/highlighter"
	"github.com/bethropolis/tide/internal/input"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/modehandler"
	"github.com/bethropolis/tide/internal/plugin"
	"github.com/bethropolis/tide/internal/statusbar"
	"github.com/bethropolis/tide/internal/theme"
	"github.com/bethropolis/tide/internal/tui"
	"github.com/bethropolis/tide/internal/types"
	"github.com/gdamore/tcell/v2"
)

// App encapsulates the core components and main loop of the editor.
type App struct {
	tuiManager         *tui.TUI
	editors            []*core.Editor
	activeEditorIndex  int
	statusBar          *statusbar.StatusBar
	eventManager       *event.Manager
	pluginManager      *plugin.Manager
	modeHandler        *modehandler.ModeHandler
	editorAPI          plugin.EditorAPI
	highlighterService *highlighter.Highlighter
	activeTheme        *theme.Theme
	themeManager       *theme.Manager
	fuzzyFinder        *tui.FuzzyFinder // Overlay
	picker             *tui.Picker     // Generic plugin-reusable overlay
	completion         *tui.CompletionOverlay // Identifier completion suggestion popup

	// Channels managed by the App
	quit          chan struct{}
	redrawRequest chan struct{}
}

// NewApp creates and initializes a new application instance.
func NewApp(filePath string) (*App, error) {
	// --- Create Core Components ---
	tuiManager, err := tui.New()
	if err != nil {
		return nil, fmt.Errorf("TUI initialization failed: %w", err)
	}

	buf := buffer.NewPieceTable()
	var _ buffer.Buffer = buf

	loadErr := buf.Load(filePath)
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		logger.Errorf("Critical error: failed to load file '%s': %v", filePath, loadErr)
		// Close TUI before returning error since we initialized it
		tuiManager.Close()
		return nil, fmt.Errorf("failed to open file '%s': %w", filePath, loadErr)
	}

	appInstance := &App{
		tuiManager:    tuiManager,
		statusBar:     statusbar.New(statusbar.DefaultConfig()),
		eventManager:  event.NewManager(),
		pluginManager: plugin.NewManager(),
		themeManager:  theme.NewManager(),
		quit:          make(chan struct{}),
		redrawRequest: make(chan struct{}, 1),
	}

	appInstance.fuzzyFinder = tui.NewFuzzyFinder(func(selectedPath string) {
		logger.Infof("FuzzyFinder selected: %s", selectedPath)
		appInstance.OpenFile(selectedPath)
	})

	appInstance.picker = tui.NewPicker("", nil, nil)

	appInstance.completion = &tui.CompletionOverlay{
		OnAccept: func(item tui.CompletionItem) {
			ed := appInstance.getActiveEditor()
			if ed == nil {
				return
			}
			buf := ed.GetBuffer()
			cursor := ed.GetCursor()

			prefixStart := cursor.Col - item.ReplaceLen
			if prefixStart < 0 {
				prefixStart = 0
			}

			start := types.Position{Line: cursor.Line, Col: prefixStart}
			end := types.Position{Line: cursor.Line, Col: cursor.Col}

			if _, err := buf.Delete(start, end); err != nil {
				return
			}
			editInfo, err := buf.Insert(start, []byte(item.InsertText))
			if err != nil {
				return
			}

			cursor.Col = prefixStart + len([]rune(item.InsertText))
			ed.SetCursor(cursor)

			appInstance.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
			appInstance.requestRedraw()
		},
	}

	appInstance.activeTheme = appInstance.themeManager.Current()

	highlighterSvc := highlighter.NewHighlighter()
	appInstance.highlighterService = highlighterSvc

	editor := appInstance.createEditor(filePath)
	appInstance.editors = append(appInstance.editors, editor)
	appInstance.activeEditorIndex = 0

	inputProcessor := input.NewInputProcessor()
	if err := inputProcessor.LoadUserBindings(&config.Get().Keybinds); err != nil {
		logger.Warnf("Failed to load user keybindings from config: %v", err)
	}
	modeHandlerCfg := modehandler.Config{
		Editor:         editor,
		InputProcessor: inputProcessor,
		EventManager:   appInstance.eventManager,
		StatusBar:      appInstance.statusBar,
		QuitSignal:     appInstance.quit,
		OnInsertEdit: func() {
			appInstance.rebuildCompletions()
		},
	}
	modeHandler := modehandler.New(modeHandlerCfg)
	appInstance.modeHandler = modeHandler

	appInstance.editorAPI = newEditorAPI(appInstance)
	modeHandler.SetAPI(appInstance.editorAPI)

	commands.RegisterAppCommands(appInstance.editorAPI, appInstance)

	// --- Register Plugins (Call centralized function) ---
	err = registerPlugins(appInstance.pluginManager) // <<< CALL NEW FUNCTION
	if err != nil {
		logger.Errorf("Errors occurred during plugin registration: %v", err)
	}

	appInstance.eventManager.Subscribe(event.TypeCursorMoved, appInstance.handleCursorMovedForStatus)
	appInstance.eventManager.Subscribe(event.TypeBufferModified, appInstance.handleBufferModifiedForStatus)
	appInstance.eventManager.Subscribe(event.TypeBufferSaved, appInstance.handleBufferSavedForStatus)
	appInstance.eventManager.Subscribe(event.TypeBufferLoaded, appInstance.handleBufferLoadedForStatus)

	appInstance.eventManager.Subscribe(event.TypeTriggerFuzzyFind, func(e event.Event) bool {
		cwd, err := os.Getwd()
		if err == nil && appInstance.fuzzyFinder != nil {
			appInstance.fuzzyFinder.Toggle(cwd)
			appInstance.requestRedraw()
		}
		return false
	})

	appInstance.eventManager.Subscribe(event.TypeBufferModified, func(e event.Event) bool {
		if data, ok := e.Data.(event.BufferModifiedData); ok {
			if hm := appInstance.getActiveEditor().GetHighlightManager(); hm != nil {
				logger.DebugTagf("highlight", "App: Forwarding BufferModified event to core highlight manager")
				hm.AccumulateEdit(data.Edit)
			} else {
				logger.Warnf("App: Highlight manager is nil, cannot process buffer modification.")
			}
			// Mark affected lines dirty for delta rendering.
			ed := appInstance.getActiveEditor()
			startLine := int(data.Edit.StartPosition.Row)
			newEndLine := int(data.Edit.NewEndPosition.Row)
			oldEndLine := int(data.Edit.OldEndPosition.Row)
			// When the edit touches multiple lines, force a full redraw so that
			// any inserted/deleted rows below the edit point are refreshed.
			if newEndLine > startLine || oldEndLine > startLine {
				ed.MarkAllDirty()
			} else {
				ed.MarkDirty(startLine)
			}
		} else {
			logger.Warnf("App: Received BufferModified event with unexpected data type: %T", e.Data)
		}
		return false
	})

	// When the highlight manager finishes a background pass, mark all lines dirty
	// and request a redraw so the new highlights appear.
	appInstance.eventManager.Subscribe(event.TypeHighlightComplete, func(e event.Event) bool {
		if ed := appInstance.getActiveEditor(); ed != nil {
			ed.MarkAllDirty()
		}
		appInstance.requestRedraw()
		return false
	})

	appInstance.pluginManager.InitializePlugins(appInstance.editorAPI)

	width, height := tuiManager.Size()
	editor.SetViewSize(width, height-config.StatusBarHeight)

	logger.DebugTagf("highlight", "App: Beginning initial asynchronous syntax highlight process...")
	lang, queryBytes := appInstance.highlighterService.GetLanguage(filePath)
	if lang != nil {
		logger.DebugTagf("highlight", "App: Language detected for '%s', proceeding with highlighting", filePath)

		bufContent := buf.Bytes() // capture bytes to avoid race and block
		go func() {
			logger.DebugTagf("highlight", "App: Calling highlighter.HighlightBuffer asynchronously...")
			startTime := time.Now()
			// Background context
			initialCtx := context.Background()
			initialHighlights, initialTree, err := appInstance.highlighterService.HighlightBuffer(initialCtx, bufContent, lang, queryBytes, nil)
			duration := time.Since(startTime)

			if err != nil {
				logger.Warnf("App: Initial async highlighting failed: %v", err)
			} else {
				highlightCount := 0
				for _, ranges := range initialHighlights {
					highlightCount += len(ranges)
				}
				logger.DebugTagf("highlight", "App: Initial async highlighting complete in %v. Found %d highlight ranges across %d lines.",
					duration, highlightCount, len(initialHighlights))

				// Update core highlight manager and trigger a redraw
				if hm := editor.GetHighlightManager(); hm != nil {
					hm.UpdateHighlights(initialHighlights, initialTree)
					logger.DebugTagf("highlight", "App: Core highlight manager state updated successfully. Requesting redraw.")
					editor.MarkAllDirty()
					appInstance.requestRedraw()
				} else {
					logger.Warnf("App: Highlight manager is nil after async highlight.")
				}
			}
		}()
	} else {
		logger.DebugTagf("highlight", "App: No language detected for initial highlight of '%s'", filePath)
		if hm := editor.GetHighlightManager(); hm != nil {
			hm.ClearHighlights()
		}
	}

	return appInstance, nil
}

// Run starts the application's main event and drawing loops.
func (a *App) Run() error {
	defer func() {
		if ed := a.getActiveEditor(); ed != nil {
			if hm := ed.GetHighlightManager(); hm != nil {
				hm.Shutdown()
			}
		}
		a.pluginManager.ShutdownPlugins()
		a.tuiManager.Close()
		logger.Infof("Tide editor shut down.")
	}()

	go a.eventLoop()

	a.eventManager.Dispatch(event.TypeAppReady, event.AppReadyData{})
	a.statusBar.SetTemporaryMessage("Tide Editor - Ctrl+S Save | :q Quit | ,: Command | ,/ Find")
	a.requestRedraw()

	for {
		select {
		case <-a.quit:
			logger.Infof("Quit signal received.")
			a.eventManager.Dispatch(event.TypeAppQuit, event.AppQuitData{})
			if ed := a.getActiveEditor(); ed != nil && ed.GetBuffer().IsModified() {
				logger.Warnf("Exited with unsaved changes.")
				fmt.Fprintln(os.Stderr, "Warning: Exited with unsaved changes.")
			}
			return nil
		case <-a.redrawRequest:
			w, h := a.tuiManager.Size()
			if ed := a.getActiveEditor(); ed != nil {
				ed.SetViewSize(w, h-config.StatusBarHeight)
			}
			a.drawEditor()
		}
	}
}

// eventLoop handles TUI events, delegating key events to ModeHandler.
func (a *App) eventLoop() {
	for {
		ev := a.tuiManager.PollEvent()
		if ev == nil {
			logger.Infof("TUI PollEvent returned nil, exiting event loop.")
			return
		}

		needsRedraw := false

		switch eventData := ev.(type) {
		case *tcell.EventResize:
			a.tuiManager.GetScreen().Sync()
			needsRedraw = true

		case *tcell.EventKey:
			if a.fuzzyFinder != nil && a.fuzzyFinder.IsActive() {
				needsRedraw = a.fuzzyFinder.HandleKeyEvent(eventData)
			} else if a.picker != nil && a.picker.IsActive() {
				needsRedraw = a.picker.HandleKeyEvent(eventData)
			} else if a.completion != nil && a.completion.IsActive() {
				// Completion overlay: Tab/Enter accept, Up/Down
				// navigate, Esc cancels; everything else passes through
				// to the editor so the user can keep typing.
				switch eventData.Key() {
				case tcell.KeyEscape, tcell.KeyCtrlC:
					a.completion.Cancel()
					needsRedraw = true
				case tcell.KeyEnter, tcell.KeyTab:
					needsRedraw = a.completion.HandleKeyEvent(eventData)
				case tcell.KeyUp, tcell.KeyDown, tcell.KeyCtrlK, tcell.KeyCtrlJ:
					needsRedraw = a.completion.HandleKeyEvent(eventData)
				default:
					needsRedraw = a.modeHandler.HandleKeyEvent(eventData)
				}
			} else {
				needsRedraw = a.modeHandler.HandleKeyEvent(eventData)
			}

		case *tcell.EventMouse:
			needsRedraw = a.modeHandler.HandleMouseEvent(eventData)
		}

		if needsRedraw {
			a.requestRedraw()
		}
	}
}

// GetModeHandler allows the API adapter to access the mode handler.
func (a *App) GetModeHandler() *modehandler.ModeHandler {
	return a.modeHandler
}

// GetThemeManager returns the app's theme manager
func (a *App) GetThemeManager() *theme.Manager {
	return a.themeManager
}

// GetTheme returns the app's active theme.
func (a *App) GetTheme() *theme.Theme {
	a.activeTheme = a.themeManager.Current()
	return a.activeTheme
}

// SetTheme changes the app's active theme and triggers a redraw.
func (a *App) SetTheme(name string) error {
	// Use the manager to set the theme
	err := a.themeManager.SetTheme(name)
	if err != nil {
		return err // Propagate error (e.g., theme not found)
	}

	// Update app's cached theme
	newTheme := a.themeManager.Current()
	oldThemeName := "unknown"
	if a.activeTheme != nil {
		oldThemeName = a.activeTheme.Name
	}
	a.activeTheme = newTheme

	// Update the TUI screen's default style with detailed logging
	if a.tuiManager != nil {
		screen := a.tuiManager.GetScreen()
		if screen != nil {
			styleToSet := newTheme.GetStyle("Default") // Get the Default style

			// Decompose and log its components
			fg, bg, attr := styleToSet.Decompose()
			logger.Debugf("App: Theme '%s' - Default style Decomposed: FG=%#v (%T), BG=%#v (%T), Attr=%#v",
				newTheme.Name, fg, fg, bg, bg, attr) // Log type and value

			// Check if background is default/reset explicitly
			if bg == tcell.ColorDefault {
				logger.Warnf("App: Theme '%s' - Default style BG resolved to tcell.ColorDefault!", newTheme.Name)
			} else if bg == tcell.ColorReset {
				logger.Warnf("App: Theme '%s' - Default style BG resolved to tcell.ColorReset!", newTheme.Name)
			} else {
				r, g, bVal := bg.RGB() // Get RGB values if it's a specific color
				logger.Debugf("App: Theme '%s' - Default style BG has specific RGB: #%02x%02x%02x", newTheme.Name, r, g, bVal)
			}

			// The actual call
			screen.SetStyle(styleToSet)
			logger.Debugf("App: Called screen.SetStyle for theme '%s'.", newTheme.Name)
		}
	}

	logger.Debugf("App: Theme changed from '%s' to '%s', requesting redraw",
		oldThemeName, newTheme.Name)

	// Dispatch theme changed event
	a.eventManager.Dispatch(event.TypeThemeChanged, event.ThemeChangedData{
		OldThemeName: oldThemeName,
		NewThemeName: newTheme.Name,
	})

	// Force a full redraw (all lines may have new styling under the new theme)
	a.getActiveEditor().MarkAllDirty()
	a.requestRedraw()
	return nil
}

// ListThemes returns available theme names.
func (a *App) ListThemes() []string {
	return a.themeManager.ListThemes()
}

// SetStatusMessage sets a temporary message.
func (a *App) SetStatusMessage(format string, args ...interface{}) {
	a.statusBar.SetTemporaryMessage(format, args...)
	a.requestRedraw()
}

// rebuildCompletions is called after each insert-mode edit.  It queries the
// current buffer's Tree-sitter tree for local identifier symbols and
// updates the floating completion overlay if the cursor sits at the end of
// a word prefix.
func (a *App) rebuildCompletions() {
	if a.completion == nil {
		return
	}
	ed := a.getActiveEditor()
	if ed == nil {
		return
	}
	if a.modeHandler.GetCurrentMode() != modehandler.ModeInsert {
		a.completion.Cancel()
		return
	}

	cursor := ed.GetCursor()
	prefix, err := a.wordPrefix(ed, cursor)
	if err != nil || len(prefix) < 2 {
		a.completion.Cancel()
		return
	}

	hm := ed.GetHighlightManager()
	if hm == nil {
		return
	}
	symbols := hm.GetLocalSymbols()
	if len(symbols) == 0 {
		a.completion.Cancel()
		return
	}

	items := tui.FilterSymbols(prefix, symbols, len(prefix))
	if len(items) == 0 {
		a.completion.Cancel()
		return
	}

	if a.completion.IsActive() {
		a.completion.Update(prefix, cursor.Line, cursor.Col, items)
	} else {
		a.completion.Activate(prefix, cursor.Line, cursor.Col, items)
	}
	a.requestRedraw()
}

// wordPrefix returns the identifier-like text immediately before the cursor
// on the same line.  Returns an empty string if the cursor is at the start
// of a line or preceded by non-identifier characters.
func (a *App) wordPrefix(ed *core.Editor, cursor types.Position) (string, error) {
	line, err := ed.GetBuffer().Line(cursor.Line)
	if err != nil {
		return "", err
	}
	if cursor.Col <= 0 || cursor.Col > len(line) {
		return "", nil
	}

	// Walk backward from cursor to find the start of the word
	runeLine := []rune(string(line))
	start := cursor.Col
	if start > len(runeLine) {
		start = len(runeLine)
	}
	if start <= 0 {
		return "", nil
	}

	i := start - 1
	for i >= 0 {
		r := runeLine[i]
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			break
		}
		i--
	}
	if i >= start-1 {
		return "", nil
	}
	return string(runeLine[i+1 : start]), nil
}

// requestRedraw sends a redraw signal non-blockingly.
func (a *App) requestRedraw() {
	select {
	case a.redrawRequest <- struct{}{}:
	default:
		logger.DebugTagf("draw", "Redraw request skipped, already pending.")
	}
}

// updateStatusBarContent pushes current editor state to the status bar component.
func (a *App) updateStatusBarContent() {
	ed := a.getActiveEditor()
	if ed == nil {
		return
	}
	buffer := ed.GetBuffer()
	a.statusBar.SetFileInfo(buffer.FilePath(), buffer.IsModified())
	a.statusBar.SetCursorInfo(ed.GetCursor())

	// Get mode string and potentially command/find buffer from ModeHandler
	modeStr := a.modeHandler.GetCurrentModeString()
	a.statusBar.SetEditorMode(modeStr) // Update the mode display

	// Check if in Command or Find mode to display the buffer in the status bar
	// Use SetTemporaryMessage to override the default status line
	currentMode := a.modeHandler.GetCurrentMode()
	if currentMode == modehandler.ModeCommand {
		a.statusBar.SetTemporaryMessage(":%s", a.modeHandler.GetCommandBuffer())
	} else if currentMode == modehandler.ModeFind {
		a.statusBar.SetTemporaryMessage("/%s", a.modeHandler.GetFindBuffer())
	}
	// Note: If not in Command/Find mode, SetTemporaryMessage called elsewhere (e.g., by commands)
	// will still take effect, or the status bar will show its default content if no temp message is active.
}

// drawEditor handles rendering the editor's content, including the status bar.
func (a *App) drawEditor() {
	// Ensure the TUI manager and editor are initialized
	ed := a.getActiveEditor()
	if a.tuiManager == nil || ed == nil || a.statusBar == nil {
		logger.Warnf("drawEditor: TUI manager, editor, or status bar is not initialized")
		return
	}

	// --- Prep for Drawing ---
	screen := a.tuiManager.GetScreen()
	w, h := a.tuiManager.Size()
	a.activeTheme = a.themeManager.Current() // Ensure we have the latest theme

	// Determine if we should draw a tab bar (only with 2+ buffers)
	multiBuffer := len(a.editors) > 1
	totalBarHeight := config.StatusBarHeight
	if multiBuffer {
		totalBarHeight++ // Extra row for tab bar
	}

	// Ensure the editor view accounts for all UI rows
	ed.SetViewSize(w, h-totalBarHeight)

	// Update status bar content *before* drawing anything
	a.updateStatusBarContent() // Update the status bar with latest info

	// --- Drawing ---
	// For a full redraw (e.g. first frame, resize, theme change) clear everything.
	// For incremental redraws, DrawBuffer handles clearing only dirty rows.
	if ed.NeedsFullRedraw() {
		a.tuiManager.Clear()
	}

	// Draw the buffer content (uses dirty-line tracking internally)
	tui.DrawBuffer(a.tuiManager, ed, a.activeTheme)

	// Draw tab bar if multiple buffers are open
	if multiBuffer {
		a.drawTabBar(screen, w, h-totalBarHeight)
	}

	// Draw the status bar
	a.statusBar.Draw(screen, w, h, a.activeTheme)

	// Draw the cursor (position is calculated relative to buffer draw)
	// Only draw cursor if not in an overlay that hides it
	showCursor := true
	if a.fuzzyFinder != nil && a.fuzzyFinder.IsActive() {
		showCursor = false
		a.fuzzyFinder.Draw(screen, a.activeTheme, w, h)
	}
	if a.picker != nil && a.picker.IsActive() {
		showCursor = false
		a.picker.Draw(screen, a.activeTheme, w, h)
	}
	if a.completion != nil && a.completion.IsActive() {
		a.completion.Draw(screen, a.activeTheme, w, h)
	}

	if showCursor {
		tui.DrawCursor(a.tuiManager, ed)
	}

	// Refresh the screen to display changes
	a.tuiManager.Show()
}

// drawTabBar renders a row of buffer tabs just above the status bar.
// tabY is the row on which the tab bar is drawn.
func (a *App) drawTabBar(screen tcell.Screen, w, tabY int) {
	th := a.activeTheme
	activeStyle := th.GetStyle("StatusBar.Mode.Normal").Reverse(false) // active tab: highlighted
	inactiveStyle := th.GetStyle("StatusBar")                          // inactive tab: base style
	modifiedStyle := th.GetStyle("StatusBar.Modified")                 // modified marker

	// Fill the row with base style first
	for x := 0; x < w; x++ {
		screen.SetContent(x, tabY, ' ', nil, inactiveStyle)
	}

	x := 0
	for i, ed := range a.editors {
		name := ed.GetBuffer().FilePath()
		if name == "" {
			name = "[No Name]"
		} else {
			// Shorten to basename
			for j := len(name) - 1; j >= 0; j-- {
				if name[j] == '/' || name[j] == '\\' {
					name = name[j+1:]
					break
				}
			}
		}

		label := " " + name + " "
		if ed.GetBuffer().IsModified() {
			label = " " + name + "* "
		}

		style := inactiveStyle
		if i == a.activeEditorIndex {
			style = activeStyle
		}

		_ = modifiedStyle // used inline via label suffix above

		// Draw the tab label
		for _, r := range label {
			if x >= w {
				break
			}
			screen.SetContent(x, tabY, r, nil, style)
			x++
		}

		// Separator between tabs
		if i < len(a.editors)-1 && x < w {
			screen.SetContent(x, tabY, '│', nil, inactiveStyle)
			x++
		}
	}
}
