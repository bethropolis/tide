package core

import (
	"fmt"

	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
)

// Text operation methods delegated to textOps
func (e *Editor) InsertRune(r rune) error {
	if e.textOps == nil {
		logger.Warnf("Editor.InsertRune: textOps manager is nil")
		return nil
	}
	return e.textOps.InsertRune(r)
}

func (e *Editor) InsertNewLine() error {
	if e.textOps == nil {
		logger.Warnf("Editor.InsertNewLine: textOps manager is nil")
		return nil
	}
	return e.textOps.InsertNewLine()
}

func (e *Editor) InsertTab() error {
	if e.textOps == nil {
		logger.Warnf("Editor.InsertTab: textOps manager is nil")
		return nil
	}
	return e.textOps.InsertTab()
}

func (e *Editor) DeleteBackward() error {
	if e.textOps == nil {
		logger.Warnf("Editor.DeleteBackward: textOps manager is nil")
		return nil
	}
	return e.textOps.DeleteBackward()
}

func (e *Editor) DeleteForward() error {
	if e.textOps == nil {
		logger.Warnf("Editor.DeleteForward: textOps manager is nil")
		return nil
	}
	return e.textOps.DeleteForward()
}

// Clipboard operations delegated to clipboardManager
func (e *Editor) YankSelection() (bool, error) {
	if e.clipboardManager == nil {
		logger.Warnf("Editor.YankSelection: clipboardManager is nil")
		return false, nil
	}
	return e.clipboardManager.YankSelection()
}

func (e *Editor) Paste() (bool, error) {
	if e.clipboardManager == nil {
		logger.Warnf("Editor.Paste: clipboardManager is nil")
		return false, nil
	}
	return e.clipboardManager.Paste()
}

// Cursor operations delegated to cursorManager
func (e *Editor) MoveCursor(deltaLine, deltaCol int) {
	if e.cursorManager == nil {
		logger.Warnf("Editor.MoveCursor: cursorManager is nil")
		return
	}

	e.cursorManager.Move(deltaLine, deltaCol)

	// Update selection end if we're selecting
	if e.selectionManager != nil && e.selectionManager.IsSelecting() {
		e.selectionManager.UpdateSelectionEnd()
	}

	logger.DebugTagf("core", "MoveCursor: Delta(%d,%d) → NewCursor(%d,%d)",
		deltaLine, deltaCol, e.GetCursor().Line, e.GetCursor().Col)
}

func (e *Editor) PageMove(deltaPages int) {
	if e.cursorManager == nil {
		logger.Warnf("Editor.PageMove: cursorManager is nil")
		return
	}

	e.cursorManager.PageMove(deltaPages)

	// Update selection end if we're selecting
	if e.selectionManager != nil && e.selectionManager.IsSelecting() {
		e.selectionManager.UpdateSelectionEnd()
	}

	logger.DebugTagf("core", "PageMove: Delta(%d) → NewCursor(%d,%d)",
		deltaPages, e.GetCursor().Line, e.GetCursor().Col)
}

func (e *Editor) Home() {
	if e.cursorManager == nil {
		logger.Warnf("Editor.Home: cursorManager is nil")
		return
	}

	e.cursorManager.MoveToLineStart()

	// Update selection end if we're selecting
	if e.selectionManager != nil && e.selectionManager.IsSelecting() {
		e.selectionManager.UpdateSelectionEnd()
	}

	logger.DebugTagf("core", "Home: NewCursor(%d,%d)", e.GetCursor().Line, e.GetCursor().Col)
}

func (e *Editor) End() {
	if e.cursorManager == nil {
		logger.Warnf("Editor.End: cursorManager is nil")
		return
	}

	e.cursorManager.MoveToLineEnd()

	// Update selection end if we're selecting
	if e.selectionManager != nil && e.selectionManager.IsSelecting() {
		e.selectionManager.UpdateSelectionEnd()
	}

	logger.DebugTagf("core", "End: NewCursor(%d,%d)", e.GetCursor().Line, e.GetCursor().Col)
}

// Find operations delegated to findManager
func (e *Editor) Find(term string, startPos types.Position, forward bool) (types.Position, bool) {
	if e.findManager == nil {
		logger.Warnf("Editor.Find: findManager is nil")
		return types.Position{}, false
	}
	err := e.findManager.HighlightMatches(term)
	if err != nil {
		return types.Position{}, false
	}
	matchPos, found, _ := e.findManager.FindNext(forward) // Ignore wrapped status
	if found {
		return matchPos, true
	}
	return types.Position{}, false
}

func (e *Editor) HighlightMatches(term string) error {
	if e.findManager == nil {
		logger.Warnf("Editor.HighlightMatches: findManager is nil")
		return fmt.Errorf("find manager not initialized")
	}
	return e.findManager.HighlightMatches(term)
}

// Replace performs a find and replace operation using findManager
func (e *Editor) Replace(pattern, replacement string, global bool) (int, error) {
	if e.findManager == nil {
		logger.Warnf("Editor.Replace: findManager is nil")
		return 0, fmt.Errorf("find manager not initialized")
	}
	return e.findManager.Replace(pattern, replacement, global)
}

// SaveBuffer handles buffer saving, accepting an optional override path.
func (e *Editor) SaveBuffer(filePath ...string) error { // Use variadic string
	savePath := ""
	if len(filePath) > 0 {
		savePath = filePath[0] // Use first provided path if given
	}
	// Delegate to buffer's save method
	err := e.buffer.Save(savePath)
	if err != nil {
		return err // Propagate error
	}
	// Dispatch save event with the ACTUAL path saved to
	if e.eventManager != nil {
		// Get the potentially updated path from the buffer
		actualPath := ""
		if bufWithFP, ok := e.buffer.(interface{ FilePath() string }); ok {
			actualPath = bufWithFP.FilePath()
		}
		e.eventManager.Dispatch(event.TypeBufferSaved, event.BufferSavedData{FilePath: actualPath})
	}
	return nil
}
