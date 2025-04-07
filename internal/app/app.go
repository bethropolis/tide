// internal/app/app.go
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time" // Still needed temporarily

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/highlighter" // Import highlighter
	"github.com/bethropolis/tide/internal/input"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/modehandler" // Import new package
	"github.com/bethropolis/tide/internal/plugin"
	"github.com/bethropolis/tide/internal/statusbar"
	"github.com/bethropolis/tide/internal/theme" // Import the theme package
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
	modeHandler         *modehandler.ModeHandler // Add ModeHandler
	editorAPI           plugin.EditorAPI
	filePath            string
	highlighter         *highlighter.Highlighter // Hold highlighter instance
	highlightingManager *HighlightingManager     // Add HighlightingManager
	activeTheme         *theme.Theme             // Store reference to the active theme

	// Channels managed by the App
	quit          chan struct{}
	redrawRequest chan struct{}

	// Status Bar State - keeping these temporarily for migration
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

	// --- Create App Instance ---
	appInstance := &App{
		tuiManager:    tuiManager,
		editor:        editor,
		statusBar:     statusBar,
		eventManager:  eventManager,
		pluginManager: pluginManager,
		modeHandler:   modeHandler,
		filePath:      filePath,
		highlighter:   highlighterSvc,
		quit:          quitChan,
		redrawRequest: make(chan struct{}, 1),
		activeTheme:   theme.GetCurrentTheme(), // Use the current theme (defaults to DefaultDark)
		// Status fields remain for migration
		statusMessage:     "",
		statusMessageTime: time.Time{},
	}

	// --- Create Highlighting Manager ---
	appInstance.highlightingManager = NewHighlightingManager(
		editor,
		highlighterSvc,
		appInstance.requestRedraw,
	)

	// --- Create Editor API adapter ---
	editorAPI := newEditorAPI(appInstance)
	appInstance.editorAPI = editorAPI

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
	pluginManager.InitializePlugins(editorAPI)

	// --- Final Setup ---
	width, height := tuiManager.Size()
	editor.SetViewSize(width, height)

	// --- Initial Syntax Highlighting (with enhanced logging) ---
	logger.Debugf("App: Beginning initial syntax highlight process...")
	lang, queryBytes := appInstance.highlighter.GetLanguage(filePath) // Get both language and query
	if lang != nil {
		logger.Debugf("App: Language detected for '%s', proceeding with highlighting", filePath)

		// Use context.Background() for initial sync parse
		initialCtx := context.Background()
		logger.Debugf("App: Getting buffer content for initial highlight...")
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
				if lineCount <= 3 { // Log first few lines as samples
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
		editor.UpdateSyntaxHighlights(make(highlighter.HighlightResult), nil) // Ensure cleared state
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
		case <-a.quit: // Wait for quit signal from ModeHandler
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

			// case *tcell.EventMouse: ...
		}

		if needsRedraw {
			a.requestRedraw()
		}
	}
}

// --- Drawing ---

// drawEditor clears screen and redraws all components.
func (a *App) drawEditor() {
	// Update status bar content (might involve modehandler state)
	a.updateStatusBarContent()

	screen := a.tuiManager.GetScreen()
	width, height := a.tuiManager.Size()

	a.tuiManager.Clear()
	// Pass the theme to DrawBuffer
	tui.DrawBuffer(a.tuiManager, a.editor, a.activeTheme)
	a.statusBar.Draw(screen, width, height, a.activeTheme) // Pass theme to status bar
	tui.DrawCursor(a.tuiManager, a.editor)
	a.tuiManager.Show()
}

// updateStatusBarContent pushes current editor state to the status bar component.
func (a *App) updateStatusBarContent() {
	buffer := a.editor.GetBuffer()
	a.statusBar.SetFileInfo(buffer.FilePath(), buffer.IsModified())
	a.statusBar.SetCursorInfo(a.editor.GetCursor())
	a.statusBar.SetEditorMode(a.modeHandler.GetCurrentModeString())

	// If in command mode, ensure the command buffer is displayed via status bar's temp message
	if a.modeHandler.GetCurrentMode() == modehandler.ModeCommand {
		a.statusBar.SetTemporaryMessage(":%s", a.modeHandler.GetCommandBuffer())
	} else if a.modeHandler.GetCurrentMode() == modehandler.ModeFind {
		// Update status bar with find buffer in find mode
		a.statusBar.SetTemporaryMessage("/%s", a.modeHandler.GetFindBuffer())
	}

	// If we have an active status message, transfer it to the status bar
	// This code will help during transition
	if !a.statusMessageTime.IsZero() && time.Since(a.statusMessageTime) <= 4*time.Second {
		a.statusBar.SetTemporaryMessage(a.statusMessage)
	}
}

// --- Event Handlers (App reacts to events) ---
func (a *App) handleCursorMovedForStatus(e event.Event) bool {
	if data, ok := e.Data.(event.CursorMovedData); ok {
		a.statusBar.SetCursorInfo(data.NewPosition)
	}
	return false
}

func (a *App) handleBufferModifiedForStatus(e event.Event) bool {
	a.updateStatusBarContent()
	return false
}

func (a *App) handleBufferSavedForStatus(e event.Event) bool {
	a.updateStatusBarContent()
	return false
}

func (a *App) handleBufferLoadedForStatus(e event.Event) bool {
	a.updateStatusBarContent()
	a.editor.TriggerSyntaxHighlight() // Re-highlight on load
	a.requestRedraw()                 // Request redraw after potential highlight changes
	return false
}

// handleBufferModifiedForHighlighting processes buffer modification events
func (a *App) handleBufferModifiedForHighlighting(e event.Event) bool {
	if data, ok := e.Data.(event.BufferModifiedData); ok {
		// Buffer was modified, accumulate the edit info
		logger.Debugf("App: Buffer modified event received, accumulating edit info.")
		a.highlightingManager.AccumulateEdit(data.Edit)
	} else {
		logger.Warnf("App: Received BufferModified event with unexpected data type: %T", e.Data)
		// Fall back to old method if edit info isn't available
		logger.Debugf("App: Falling back to non-incremental highlighting due to missing edit info")
	}
	return false // Allow other handlers for BufferModified to run
}

// SetStatusMessage updates the status message.
// Keeping temporarily for backward compatibility during transition
func (a *App) SetStatusMessage(format string, args ...interface{}) {
	a.statusMessage = fmt.Sprintf(format, args...)
	a.statusMessageTime = time.Now()
	// Forward to the new status bar
	a.statusBar.SetTemporaryMessage(a.statusMessage)
}

// requestRedraw sends a redraw signal non-blockingly.
func (a *App) requestRedraw() {
	select {
	case a.redrawRequest <- struct{}{}:
	default: // Don't block if a redraw is already pending
	}
}

// GetModeHandler allows the API adapter to access the mode handler for command registration.
func (a *App) GetModeHandler() *modehandler.ModeHandler {
	return a.modeHandler
}

// GetTheme returns the app's active theme.
func (a *App) GetTheme() *theme.Theme {
	return a.activeTheme
}

// SetTheme changes the app's active theme and triggers a redraw.
func (a *App) SetTheme(t *theme.Theme) {
	if t != nil {
		a.activeTheme = t
		theme.SetCurrentTheme(t)
		a.requestRedraw() // Redraw with new theme
	}
}
