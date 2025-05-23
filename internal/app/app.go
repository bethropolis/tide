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
	"github.com/gdamore/tcell/v2"
)

// App encapsulates the core components and main loop of the editor.
type App struct {
	tuiManager         *tui.TUI
	editor             *core.Editor
	statusBar          *statusbar.StatusBar
	eventManager       *event.Manager
	pluginManager      *plugin.Manager
	modeHandler        *modehandler.ModeHandler
	editorAPI          plugin.EditorAPI
	filePath           string
	highlighterService *highlighter.Highlighter
	activeTheme        *theme.Theme
	themeManager       *theme.Manager

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

	buf := buffer.NewSliceBuffer()
	var _ buffer.Buffer = buf

	loadErr := buf.Load(filePath)
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		logger.Warnf("Warning: error loading file '%s': %v", filePath, loadErr)
	}

	appInstance := &App{
		tuiManager:    tuiManager,
		statusBar:     statusbar.New(statusbar.DefaultConfig()),
		eventManager:  event.NewManager(),
		pluginManager: plugin.NewManager(),
		filePath:      filePath,
		themeManager:  theme.NewManager(),
		quit:          make(chan struct{}),
		redrawRequest: make(chan struct{}, 1),
	}
	appInstance.activeTheme = appInstance.themeManager.Current()

	highlighterSvc := highlighter.NewHighlighter()
	appInstance.highlighterService = highlighterSvc

	editor := core.NewEditor(buf, highlighterSvc, appInstance.requestRedraw)
	appInstance.editor = editor

	inputProcessor := input.NewInputProcessor()
	modeHandlerCfg := modehandler.Config{
		Editor:         editor,
		InputProcessor: inputProcessor,
		EventManager:   appInstance.eventManager,
		StatusBar:      appInstance.statusBar,
		QuitSignal:     appInstance.quit,
	}
	modeHandler := modehandler.New(modeHandlerCfg)
	appInstance.modeHandler = modeHandler

	editor.SetEventManager(appInstance.eventManager)

	appInstance.editorAPI = newEditorAPI(appInstance)

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

	appInstance.eventManager.Subscribe(event.TypeBufferModified, func(e event.Event) bool {
		if data, ok := e.Data.(event.BufferModifiedData); ok {
			if hm := appInstance.editor.GetHighlightManager(); hm != nil {
				logger.DebugTagf("highlight", "App: Forwarding BufferModified event to core highlight manager")
				hm.AccumulateEdit(data.Edit)
			} else {
				logger.Warnf("App: Highlight manager is nil, cannot process buffer modification.")
			}
		} else {
			logger.Warnf("App: Received BufferModified event with unexpected data type: %T", e.Data)
		}
		return false
	})

	appInstance.pluginManager.InitializePlugins(appInstance.editorAPI)

	width, height := tuiManager.Size()
	editor.SetViewSize(width, height)

	logger.DebugTagf("highlight", "App: Beginning initial synchronous syntax highlight process...")
	lang, queryBytes := appInstance.highlighterService.GetLanguage(filePath)
	if lang != nil {
		logger.DebugTagf("highlight", "App: Language detected for '%s', proceeding with highlighting", filePath)

		initialCtx := context.Background()
		bufContent := buf.Bytes()
		logger.DebugTagf("highlight", "App: Buffer size for initial highlighting: %d bytes", len(bufContent))

		logger.DebugTagf("highlight", "App: Calling highlighter.HighlightBuffer synchronously...")
		startTime := time.Now()
		initialHighlights, initialTree, err := appInstance.highlighterService.HighlightBuffer(initialCtx, buf.Bytes(), lang, queryBytes, nil) // <<< Pass buf.Bytes()
		duration := time.Since(startTime)

		if err != nil {
			logger.Warnf("App: Initial synchronous highlighting failed: %v", err)
		} else {
			highlightCount := 0
			for _, ranges := range initialHighlights {
				highlightCount += len(ranges)
			}
			logger.DebugTagf("highlight", "App: Initial sync highlighting complete in %v. Found %d highlight ranges across %d lines.",
				duration, highlightCount, len(initialHighlights))

			if hm := editor.GetHighlightManager(); hm != nil {
				hm.UpdateHighlights(initialHighlights, initialTree)
				logger.DebugTagf("highlight", "App: Core highlight manager state updated successfully.")
			} else {
				logger.Warnf("App: Highlight manager is nil after sync highlight.")
			}
		}
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
		if hm := a.editor.GetHighlightManager(); hm != nil {
			hm.Shutdown()
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
			if a.editor.GetBuffer().IsModified() {
				logger.Warnf("Exited with unsaved changes.")
				fmt.Fprintln(os.Stderr, "Warning: Exited with unsaved changes.")
			}
			return nil
		case <-a.redrawRequest:
			w, h := a.tuiManager.Size()
			a.editor.SetViewSize(w, h)
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
			needsRedraw = a.modeHandler.HandleKeyEvent(eventData)
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

	// Force an immediate redraw
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
	buffer := a.editor.GetBuffer()
	a.statusBar.SetFileInfo(buffer.FilePath(), buffer.IsModified())
	a.statusBar.SetCursorInfo(a.editor.GetCursor())

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
	if a.tuiManager == nil || a.editor == nil || a.statusBar == nil {
		logger.Warnf("drawEditor: TUI manager, editor, or status bar is not initialized")
		return
	}

	// --- Prep for Drawing ---
	screen := a.tuiManager.GetScreen()
	w, h := a.tuiManager.Size()
	a.activeTheme = a.themeManager.Current() // Ensure we have the latest theme

	// Update status bar content *before* drawing anything
	a.updateStatusBarContent() // Update the status bar with latest info

	// --- Drawing ---
	a.tuiManager.Clear()

	// Draw the buffer content
	tui.DrawBuffer(a.tuiManager, a.editor, a.activeTheme)

	// Draw the status bar
	a.statusBar.Draw(screen, w, h, a.activeTheme)

	// Draw the cursor (position is calculated relative to buffer draw)
	tui.DrawCursor(a.tuiManager, a.editor)

	// Refresh the screen to display changes
	a.tuiManager.Show()
}
