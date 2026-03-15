package app

import (
	"context"
	"fmt"
	"os"

	"github.com/bethropolis/tide/internal/buffer"
	"github.com/bethropolis/tide/internal/config"
	"github.com/bethropolis/tide/internal/core"
	"github.com/bethropolis/tide/internal/logger"
)

func (a *App) getActiveEditor() *core.Editor {
	if len(a.editors) == 0 {
		return nil
	}
	return a.editors[a.activeEditorIndex]
}

func (a *App) createEditor(filePath string) *core.Editor {
	buf := buffer.NewPieceTable()

	if filePath != "" {
		err := buf.Load(filePath)
		if err != nil && !os.IsNotExist(err) {
			logger.Warnf("Warning: error loading file '%s': %v", filePath, err)
		}
	}

	// Use the event manager so the highlight manager can dispatch TypeHighlightComplete
	// when a background highlighting pass finishes; the app subscribes to that event
	// to call MarkAllDirty() + requestRedraw().
	editor := core.NewEditor(buf, a.highlighterService, a.eventManager)

	lang, queryBytes := a.highlighterService.GetLanguage(filePath)
	if lang != nil {
		initialCtx := context.Background()
		initialHighlights, initialTree, _ := a.highlighterService.HighlightBuffer(initialCtx, buf.Bytes(), lang, queryBytes, nil)
		if hm := editor.GetHighlightManager(); hm != nil {
			hm.UpdateHighlights(initialHighlights, initialTree)
		}
	}

	w, h := a.tuiManager.Size()
	editor.SetViewSize(w, h-config.StatusBarHeight)
	return editor
}

// OpenFile opens a file in a new buffer or switches to it if already open
func (a *App) OpenFile(filePath string) {
	// Check if already open
	for i, ed := range a.editors {
		if ed.GetBuffer().FilePath() == filePath {
			a.activeEditorIndex = i
			if a.modeHandler != nil {
				a.modeHandler.SetEditor(a.getActiveEditor())
			}
			a.statusBar.SetTemporaryMessage("Switched to %s", filePath)
			a.requestRedraw()
			return
		}
	}

	// Create new editor
	newEd := a.createEditor(filePath)
	a.editors = append(a.editors, newEd)
	a.activeEditorIndex = len(a.editors) - 1
	if a.modeHandler != nil {
		a.modeHandler.SetEditor(a.getActiveEditor())
	}
	a.statusBar.SetTemporaryMessage("Opened %s", filePath)
	a.requestRedraw()
}

// NextBuffer switches to the next buffer
func (a *App) NextBuffer() {
	if len(a.editors) <= 1 {
		return
	}
	a.activeEditorIndex = (a.activeEditorIndex + 1) % len(a.editors)
	if a.modeHandler != nil {
		a.modeHandler.SetEditor(a.getActiveEditor())
	}
	a.getActiveEditor().MarkAllDirty() // Force full redraw for the newly active buffer
	a.statusBar.SetTemporaryMessage("Buffer %d/%d", a.activeEditorIndex+1, len(a.editors))
	a.requestRedraw()
}

// PrevBuffer switches to the previous buffer
func (a *App) PrevBuffer() {
	if len(a.editors) <= 1 {
		return
	}
	a.activeEditorIndex = (a.activeEditorIndex - 1 + len(a.editors)) % len(a.editors)
	if a.modeHandler != nil {
		a.modeHandler.SetEditor(a.getActiveEditor())
	}
	a.getActiveEditor().MarkAllDirty() // Force full redraw for the newly active buffer
	a.statusBar.SetTemporaryMessage("Buffer %d/%d", a.activeEditorIndex+1, len(a.editors))
	a.requestRedraw()
}

// CloseBuffer closes the active buffer
func (a *App) CloseBuffer() error {
	active := a.getActiveEditor()
	if active == nil {
		return nil
	}
	if active.GetBuffer().IsModified() {
		return fmt.Errorf("buffer has unsaved changes (use :bd! to force)")
	}

	a.ForceCloseBuffer()
	return nil
}

// ForceCloseBuffer closes the active buffer without checking for modifications
func (a *App) ForceCloseBuffer() {
	if len(a.editors) <= 1 {
		a.quit <- struct{}{}
		return
	}

	// Remove from slice
	a.editors = append(a.editors[:a.activeEditorIndex], a.editors[a.activeEditorIndex+1:]...)
	if a.activeEditorIndex >= len(a.editors) {
		a.activeEditorIndex = len(a.editors) - 1
	}

	if a.modeHandler != nil {
		a.modeHandler.SetEditor(a.getActiveEditor())
	}
	a.getActiveEditor().MarkAllDirty()
	a.statusBar.SetTemporaryMessage("Buffer closed")
	a.requestRedraw()
}
