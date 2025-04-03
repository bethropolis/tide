// internal/app/app.go
package app

import (
	"errors"
	"fmt"
	"log"
	"time" // Still needed temporarily

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/input"
	"github.com/bethropolis/tide/internal/statusbar" // Import statusbar
	"github.com/bethropolis/tide/internal/tui"
	"github.com/gdamore/tcell/v2"
)

// App encapsulates the core components and main loop of the editor.
type App struct {
	tuiManager     *tui.TUI
	editor         *core.Editor
	inputProcessor *input.InputProcessor
	statusBar      *statusbar.StatusBar // Use the status bar component
	filePath       string

	// Channels managed by the App
	quit          chan struct{}
	redrawRequest chan struct{}

	// Status Bar State - keeping these temporarily for migration
	statusMessage     string
	statusMessageTime time.Time
	forceQuitPending  bool
}

// NewApp creates and initializes a new application instance.
func NewApp(filePath string) (*App, error) {
	tuiManager, err := tui.New() // TUI handles its own init
	if err != nil {
		return nil, fmt.Errorf("TUI initialization failed: %w", err)
	}

	buf := buffer.NewSliceBuffer() // Or load based on factory later
	// Ensure buffer interface satisfies needed methods (compile-time check)
	var _ buffer.Buffer = buf
	var _ interface{ IsModified() bool } = buf
	var _ interface{ FilePath() string } = buf

	// Load initial file content
	loadErr := buf.Load(filePath)
	if loadErr != nil && !errors.Is(loadErr, errors.New("file does not exist")) { // Crude check for non-existence
		// Maybe use os.IsNotExist(loadErr) if Load returns wrapped errors
		log.Printf("Warning: error loading file '%s': %v", filePath, loadErr)
	}

	editor := core.NewEditor(buf)                         // Editor initializes its state
	inputProcessor := input.NewInputProcessor()           // Input handles its defaults
	statusBar := statusbar.New(statusbar.DefaultConfig()) // Create status bar

	// Set initial view size
	width, height := tuiManager.Size()
	editor.SetViewSize(width, height)

	return &App{
		tuiManager:     tuiManager,
		editor:         editor,
		inputProcessor: inputProcessor,
		statusBar:      statusBar, // Assign status bar instance
		filePath:       filePath,  // Store initial path requested
		quit:           make(chan struct{}),
		redrawRequest:  make(chan struct{}, 1), // Buffered
		// Status fields remain for migration
		statusMessage:     "",
		statusMessageTime: time.Time{},
	}, nil
}

// Run starts the application's main event and drawing loops.
// It blocks until the application quits.
func (a *App) Run() error {
	// Ensure TUI is closed properly on exit
	defer a.tuiManager.Close()

	// Start the event loop in a separate goroutine
	go a.eventLoop()

	// Initial status message and draw
	a.statusBar.SetTemporaryMessage("Tide Editor - Ctrl+S Save | ESC Quit")
	a.requestRedraw() // Use method for consistency

	// --- Main Drawing Loop (runs in the main goroutine) ---
	for {
		select {
		case <-a.quit:
			// Final check for unsaved changes before logging exit
			if a.editor.GetBuffer().IsModified() {
				log.Println("Warning: Exited with unsaved changes.")
			}
			log.Println("Exiting application.")
			return nil // Normal exit
		case <-a.redrawRequest:
			// Reset force quit flag on redraw (means another key was pressed)
			a.forceQuitPending = false
			// Update view size before drawing, in case of resize events
			w, h := a.tuiManager.Size()
			a.editor.SetViewSize(w, h) // Update editor's view size knowledge
			// Perform drawing using functions now likely in the tui package
			a.drawEditor()
		}
	}
}

// eventLoop handles TUI events.
// eventLoop handles TUI events.
func (a *App) eventLoop() {
	// Remove defer close(a.quit) - Already done

	for {
		event := a.tuiManager.PollEvent()
		if event == nil {
			return
		} // Exit if screen closed

		needsRedraw := false // Reset for each event

		switch ev := event.(type) {
		case *tcell.EventResize:
			a.tuiManager.GetScreen().Sync()
			needsRedraw = true // Resize always needs redraw

		case *tcell.EventKey:
			actionEvent := a.inputProcessor.ProcessEvent(ev)
			actionProcessed := a.handleAction(actionEvent) // Delegate action handling

			// --- Corrected needsRedraw Logic ---
			// 1. If an action was processed successfully, we *always* need a redraw.
			if actionProcessed {
				needsRedraw = true
			} else {
				// 2. If action wasn't processed, check for specific cases needing redraw (like quit prompt)
				if actionEvent.Action == input.ActionQuit && a.forceQuitPending {
					needsRedraw = true // Need to redraw to show the status message
				}
				// Add other cases here if needed (e.g., unknown key showing a message)
			}
			// --- End Corrected Logic ---

		// case *tcell.EventMouse: ...
		} // End switch event.(type)

		if needsRedraw {
			a.requestRedraw()
		}
	} // End for loop
}

// handleAction processes the mapped editor action. Returns true if redraw is needed.
func (a *App) handleAction(actionEvent input.ActionEvent) bool {
	actionProcessed := true // Assume success unless error occurs or no action taken

	switch actionEvent.Action {
	case input.ActionQuit:
		if a.editor.GetBuffer().IsModified() && !a.forceQuitPending {
			a.statusBar.SetTemporaryMessage("Unsaved changes! Press ESC again or Ctrl+Q to force quit.")
			a.forceQuitPending = true
			actionProcessed = false // Don't mark as processed yet
		} else {
			// Signal quit by closing channel *here*
			close(a.quit)
			actionProcessed = false // No further redraw needed
		}
	case input.ActionForceQuit:
		// Signal quit by closing channel *here*
		close(a.quit)
		actionProcessed = false

	case input.ActionSave:
		err := a.editor.SaveBuffer() // Editor handles buffer interaction
		savedPath := a.editor.GetBuffer().FilePath()
		if savedPath == "" {
			savedPath = a.filePath
		} // Fallback if somehow empty
		if savedPath == "" {
			savedPath = "[No Name]"
		}
		if err != nil {
			a.statusBar.SetTemporaryMessage("Save FAILED: %v", err)
		} else {
			a.statusBar.SetTemporaryMessage("Buffer saved successfully to %s", savedPath)
		}

	// --- Cursor Movement ---
	case input.ActionMoveUp:
		a.editor.MoveCursor(-1, 0)
	case input.ActionMoveDown:
		a.editor.MoveCursor(1, 0)
	case input.ActionMoveLeft:
		a.editor.MoveCursor(0, -1)
	case input.ActionMoveRight:
		a.editor.MoveCursor(0, 1)
	case input.ActionMovePageUp:
		a.editor.PageMove(-1)
	case input.ActionMovePageDown:
		a.editor.PageMove(1)
	case input.ActionMoveHome:
		a.editor.Home()
	case input.ActionMoveEnd:
		a.editor.End()

	// --- Text Manipulation ---
	case input.ActionInsertRune:
		err := a.editor.InsertRune(actionEvent.Rune)
		if err != nil {
			log.Printf("Err InsertRune: %v", err)
			actionProcessed = false
		}
	case input.ActionInsertNewLine:
		err := a.editor.InsertNewLine()
		if err != nil {
			log.Printf("Err InsertNewLine: %v", err)
			actionProcessed = false
		}
	case input.ActionDeleteCharBackward:
		err := a.editor.DeleteBackward()
		if err != nil {
			log.Printf("Err DeleteBackward: %v", err)
			actionProcessed = false
		}
	case input.ActionDeleteCharForward:
		err := a.editor.DeleteForward()
		if err != nil {
			log.Printf("Err DeleteForward: %v", err)
			actionProcessed = false
		}

	case input.ActionUnknown:
		actionProcessed = false
	}

	// Reset force quit flag if any *other* action was processed successfully
	if actionEvent.Action != input.ActionQuit && actionEvent.Action != input.ActionUnknown && actionProcessed {
		a.forceQuitPending = false
	}

	return actionProcessed
}

// --- Drawing and Status Management (App methods) ---

// drawEditor clears screen and redraws all components.
func (a *App) drawEditor() {
	// Update status bar content before drawing
	a.updateStatusBarContent()

	// Get screen and dimensions for drawing functions
	screen := a.tuiManager.GetScreen()
	width, height := a.tuiManager.Size()

	// Clear and draw main components
	a.tuiManager.Clear()
	tui.DrawBuffer(a.tuiManager, a.editor)  // tui draws buffer
	a.statusBar.Draw(screen, width, height) // status bar draws itself
	tui.DrawCursor(a.tuiManager, a.editor)  // tui draws cursor

	// Show composite view
	a.tuiManager.Show()
}

// updateStatusBarContent pushes current editor state to the status bar component.
func (a *App) updateStatusBarContent() {
	buffer := a.editor.GetBuffer()
	a.statusBar.SetFileInfo(buffer.FilePath(), buffer.IsModified())
	a.statusBar.SetCursorInfo(a.editor.GetCursor())

	// If we have an active status message, transfer it to the status bar
	// This code will help during transition
	if !a.statusMessageTime.IsZero() && time.Since(a.statusMessageTime) <= 4*time.Second {
		a.statusBar.SetTemporaryMessage(a.statusMessage)
	}
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
