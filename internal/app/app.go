// internal/app/app.go
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/commands"
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/highlight"
	"github.com/bethropolis/tide/internal/highlighter"
	"github.com/bethropolis/tide/internal/input"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/modehandler"
	"github.com/bethropolis/tide/internal/plugin"
	"github.com/bethropolis/tide/internal/statusbar"
	"github.com/bethropolis/tide/internal/theme"
	"github.com/bethropolis/tide/internal/tui"
	"github.com/bethropolis/tide/plugins/wordcount"
	"github.com/gdamore/tcell/v2"
)

// App encapsulates the core components and main loop of the editor.
type App struct {
	tuiManager          *tui.TUI
	editor              *core.Editor
	statusBar           *statusbar.StatusBar
	eventManager        *event.Manager
	pluginManager       *plugin.Manager
	modeHandler         *modehandler.ModeHandler
	editorAPI           plugin.EditorAPI
	filePath            string
	highlighter         *highlighter.Highlighter
	highlightingManager *highlight.Manager
	activeTheme         *theme.Theme
	themeManager        *theme.Manager

	// Channels managed by the App
	quit          chan struct{}
	redrawRequest chan struct{}

	// Status Bar State - kept temporarily for migration
	statusMessage     string
	statusMessageTime time.Time
}

// NewApp creates and initializes a new application instance.
func NewApp(filePath string) (*App, error) {
	// --- Create Core Components ---
	tuiManager, err := tui.New()
	if err != nil {
		return nil, fmt.Errorf("TUI initialization failed: %w", err)
	}

	buf := buffer.NewSliceBuffer()
	// Ensure buffer interface satisfies needed methods (compile-time check)
	var _ buffer.Buffer = buf
	var _ interface{ IsModified() bool } = buf
	var _ interface{ FilePath() string } = buf

	loadErr := buf.Load(filePath)
	if loadErr != nil && !errors.Is(loadErr, errors.New("file does not exist")) {
		logger.Debugf("Warning: error loading file '%s': %v", filePath, loadErr)
	}

	editor := core.NewEditor(buf)

	// Create highlighter service
	highlighterSvc := highlighter.NewHighlighter()
	editor.SetHighlighter(highlighterSvc)

	inputProcessor := input.NewInputProcessor()
	statusBar := statusbar.New(statusbar.DefaultConfig())
	eventManager := event.NewManager()
	pluginManager := plugin.NewManager()
	quitChan := make(chan struct{})

	// Set event manager in editor so it can dispatch events
	editor.SetEventManager(eventManager)

	// --- Create Mode Handler ---
	modeHandlerCfg := modehandler.Config{
		Editor:         editor,
		InputProcessor: inputProcessor,
		EventManager:   eventManager,
		StatusBar:      statusBar,
		QuitSignal:     quitChan,
	}
	modeHandler := modehandler.New(modeHandlerCfg)

	// Initialize the theme manager
	themeManager := theme.NewManager()
	activeTheme := themeManager.Current() // Get current theme from manager

	// --- Create App Instance ---
	appInstance := &App{
		tuiManager:        tuiManager,
		editor:            editor,
		statusBar:         statusBar,
		eventManager:      eventManager,
		pluginManager:     pluginManager,
		modeHandler:       modeHandler,
		filePath:          filePath,
		highlighter:       highlighterSvc,
		themeManager:      themeManager,
		activeTheme:       activeTheme,
		quit:              quitChan,
		redrawRequest:     make(chan struct{}, 1),
		statusMessage:     "",
		statusMessageTime: time.Time{},
	}

	// --- Create Highlighting Manager ---
	appInstance.highlightingManager = highlight.NewManager(
		editor,
		highlighterSvc,
		appInstance.requestRedraw,
	)

	// --- Create Editor API adapter ---
	appInstance.editorAPI = newEditorAPI(appInstance)

	// Register Built-in App Commands (like :theme)
	// Fix: Use commands.ThemeAPI to avoid import cycles
	commands.RegisterAppCommands(appInstance.editorAPI, appInstance.editorAPI.(commands.ThemeAPI))

	// --- Register Built-in Plugins ---
	wcPlugin := wordcount.New()
	if err := pluginManager.Register(wcPlugin); err != nil {
		logger.Debugf("Failed to register WordCount plugin: %v", err)
	}
	// Register other plugins here...

	// --- Subscribe Core Components (App level wiring) ---
	eventManager.Subscribe(event.TypeCursorMoved, appInstance.handleCursorMovedForStatus)
	eventManager.Subscribe(event.TypeBufferModified, appInstance.handleBufferModifiedForStatus)
	eventManager.Subscribe(event.TypeBufferSaved, appInstance.handleBufferSavedForStatus)
	eventManager.Subscribe(event.TypeBufferLoaded, appInstance.handleBufferLoadedForStatus)

	// --- Subscribe to Buffer Modifications for Highlighting ---
	eventManager.Subscribe(event.TypeBufferModified, appInstance.handleBufferModifiedForHighlighting)

	// --- Initialize Plugins (triggers RegisterCommand via API) ---
	pluginManager.InitializePlugins(appInstance.editorAPI)

	// --- Final Setup ---
	width, height := tuiManager.Size()
	editor.SetViewSize(width, height)

	// --- Initial Syntax Highlighting ---
	logger.Debugf("App: Beginning initial syntax highlight process...")
	lang, queryBytes := appInstance.highlighter.GetLanguage(filePath)
	if lang != nil {
		logger.Debugf("App: Language detected for '%s', proceeding with highlighting", filePath)

		initialCtx := context.Background()
		bufContent := buf.Bytes()
		logger.Debugf("App: Buffer size for highlighting: %d bytes", len(bufContent))

		logger.Debugf("App: Calling highlighter.HighlightBuffer synchronously...")
		startTime := time.Now()
		initialHighlights, initialTree, err := appInstance.highlighter.HighlightBuffer(initialCtx, buf, lang, queryBytes, nil)
		duration := time.Since(startTime)

		if err != nil {
			logger.Warnf("App: Initial highlighting failed: %v", err)
		} else {
			highlightCount := 0
			lineCount := 0
			for lineNum, ranges := range initialHighlights {
				lineCount++
				highlightCount += len(ranges)
				if lineCount <= 3 {
					logger.Debugf("App: Line %d has %d highlight ranges", lineNum, len(ranges))
				}
			}

			logger.Debugf("App: Initial highlighting complete in %v. Found %d highlight ranges across %d lines.",
				duration, highlightCount, lineCount)

			logger.Debugf("App: Updating editor with initial highlights...")
			editor.UpdateSyntaxHighlights(initialHighlights, initialTree)
			logger.Debugf("App: Editor highlighting state updated successfully")
		}
	} else {
		logger.Debugf("App: No language detected for initial highlight of '%s'", filePath)
		editor.UpdateSyntaxHighlights(make(highlighter.HighlightResult), nil)
	}

	return appInstance, nil
}

// Run starts the application's main event and drawing loops.
func (a *App) Run() error {
	defer a.tuiManager.Close()
	defer a.pluginManager.ShutdownPlugins()

	go a.eventLoop() // Start event loop

	// Initial setup
	a.eventManager.Dispatch(event.TypeAppReady, event.AppReadyData{})
	a.statusBar.SetTemporaryMessage("Tide Editor - Ctrl+S Save | ESC Quit")
	a.requestRedraw()

	// --- Main Drawing Loop ---
	for {
		select {
		case <-a.quit:
			a.eventManager.Dispatch(event.TypeAppQuit, event.AppQuitData{})
			if a.editor.GetBuffer().IsModified() {
				log.Println("Warning: Exited with unsaved changes.")
			}
			log.Println("Exiting application.")
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
			return
		}

		needsRedraw := false

		switch eventData := ev.(type) {
		case *tcell.EventResize:
			a.tuiManager.GetScreen().Sync()
			needsRedraw = true

		case *tcell.EventKey:
			// Delegate ALL key handling to ModeHandler
			needsRedraw = a.modeHandler.HandleKeyEvent(eventData)
		}

		if needsRedraw {
			a.requestRedraw()
		}
	}
}

// GetModeHandler allows the API adapter to access the mode handler for command registration.
func (a *App) GetModeHandler() *modehandler.ModeHandler {
	return a.modeHandler
}

// GetThemeManager returns the app's theme manager
func (a *App) GetThemeManager() *theme.Manager {
	return a.themeManager
}

// GetTheme returns the app's active theme.
func (a *App) GetTheme() *theme.Theme {
	return a.activeTheme
}

// SetTheme changes the app's active theme and triggers a redraw.
func (a *App) SetTheme(t *theme.Theme) {
	if t != nil {
		oldTheme := a.activeTheme

		// Update the app's active theme reference
		a.activeTheme = t

		// Update the theme manager's active theme
		if err := a.themeManager.SetTheme(t.Name); err != nil {
			logger.Warnf("Failed to set theme in manager: %v", err)
		}

		logger.Debugf("App: Theme changed from '%s' to '%s', requesting redraw",
			oldTheme.Name, t.Name)

		// Dispatch theme changed event
		a.eventManager.Dispatch(event.TypeThemeChanged, t.Name)

		// Force an immediate redraw
		a.requestRedraw()
	}
}
