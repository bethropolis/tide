// internal/app/app.go
package app

import (
	"errors"
	"fmt"
	"log"
	"time" // Still needed temporarily

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/input"
	"github.com/bethropolis/tide/internal/modehandler" // Import new package
	"github.com/bethropolis/tide/internal/plugin"
	"github.com/bethropolis/tide/internal/statusbar"
	"github.com/bethropolis/tide/internal/tui"
	"github.com/bethropolis/tide/plugins/wordcount"
	"github.com/gdamore/tcell/v2"
)

// App encapsulates the core components and main loop of the editor.
type App struct {
	tuiManager    *tui.TUI
	editor        *core.Editor
	statusBar     *statusbar.StatusBar
	eventManager  *event.Manager
	pluginManager *plugin.Manager
	modeHandler   *modehandler.ModeHandler // Add ModeHandler
	editorAPI     plugin.EditorAPI
	filePath      string

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
		log.Printf("Warning: error loading file '%s': %v", filePath, loadErr)
	}

	editor := core.NewEditor(buf)
	inputProcessor := input.NewInputProcessor()
	statusBar := statusbar.New(statusbar.DefaultConfig())
	eventManager := event.NewManager()
	pluginManager := plugin.NewManager()
	quitChan := make(chan struct{})

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
		quit:          quitChan,
		redrawRequest: make(chan struct{}, 1),
		// Status fields remain for migration
		statusMessage:     "",
		statusMessageTime: time.Time{},
	}

	// --- Create Editor API adapter ---
	editorAPI := newEditorAPI(appInstance)
	appInstance.editorAPI = editorAPI

	// --- Register Built-in Plugins ---
	wcPlugin := wordcount.New()
	if err := pluginManager.Register(wcPlugin); err != nil {
		log.Printf("Failed to register WordCount plugin: %v", err)
	}
	// Register other plugins here...

	// --- Subscribe Core Components (App level wiring) ---
	eventManager.Subscribe(event.TypeCursorMoved, appInstance.handleCursorMovedForStatus)
	eventManager.Subscribe(event.TypeBufferModified, appInstance.handleBufferModifiedForStatus)
	eventManager.Subscribe(event.TypeBufferSaved, appInstance.handleBufferSavedForStatus)
	eventManager.Subscribe(event.TypeBufferLoaded, appInstance.handleBufferLoadedForStatus)

	// --- Initialize Plugins (triggers RegisterCommand via API) ---
	pluginManager.InitializePlugins(editorAPI)

	// --- Final Setup ---
	width, height := tuiManager.Size()
	editor.SetViewSize(width, height)

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
	tui.DrawBuffer(a.tuiManager, a.editor)
	a.statusBar.Draw(screen, width, height) // status bar draws itself
	tui.DrawCursor(a.tuiManager, a.editor)
	a.tuiManager.Show()
}

// updateStatusBarContent pushes current editor state to the status bar component.
func (a *App) updateStatusBarContent() {
	buffer := a.editor.GetBuffer()
	a.statusBar.SetFileInfo(buffer.FilePath(), buffer.IsModified())
	a.statusBar.SetCursorInfo(a.editor.GetCursor())

	// If in command mode, ensure the command buffer is displayed via status bar's temp message
	if a.modeHandler.GetCurrentMode() == modehandler.ModeCommand {
		a.statusBar.SetTemporaryMessage(":%s", a.modeHandler.GetCommandBuffer())
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
	return false
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
