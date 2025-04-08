package core

import (
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
	// Sync cursor state back to Editor
	e.Cursor = e.cursorManager.GetPosition()
	// Also update selection end if selecting
	if e.selecting {
		e.selectionEnd = e.Cursor
	}

	logger.Debugf("MoveCursor: Delta(%d,%d) → NewCursor(%d,%d)",
		deltaLine, deltaCol, e.Cursor.Line, e.Cursor.Col)
}

func (e *Editor) PageMove(deltaPages int) {
	if e.cursorManager == nil {
		logger.Warnf("Editor.PageMove: cursorManager is nil")
		return
	}

	e.cursorManager.PageMove(deltaPages)
	// Sync cursor state back to Editor
	e.Cursor = e.cursorManager.GetPosition()
	if e.selecting {
		e.selectionEnd = e.Cursor
	}

	logger.Debugf("PageMove: Delta(%d) → NewCursor(%d,%d)",
		deltaPages, e.Cursor.Line, e.Cursor.Col)
}

func (e *Editor) Home() {
	if e.cursorManager == nil {
		logger.Warnf("Editor.Home: cursorManager is nil")
		return
	}

	e.cursorManager.MoveToLineStart()
	// Sync cursor state back to Editor
	e.Cursor = e.cursorManager.GetPosition()
	if e.selecting {
		e.selectionEnd = e.Cursor
	}

	logger.Debugf("Home: NewCursor(%d,%d)", e.Cursor.Line, e.Cursor.Col)
}

func (e *Editor) End() {
	if e.cursorManager == nil {
		logger.Warnf("Editor.End: cursorManager is nil")
		return
	}

	e.cursorManager.MoveToLineEnd()
	// Sync cursor state back to Editor
	e.Cursor = e.cursorManager.GetPosition()
	if e.selecting {
		e.selectionEnd = e.Cursor
	}

	logger.Debugf("End: NewCursor(%d,%d)", e.Cursor.Line, e.Cursor.Col)
}

// Find operations
func (e *Editor) Find(term string, startPos types.Position, forward bool) (types.Position, bool) {
	// Basic implementation - can be moved to a find manager later
	if term == "" {
		return types.Position{Line: -1, Col: -1}, false
	}

	// Implementation using buffer
	// This implementation is simplified and should be replaced with proper logic
	logger.Debugf("Find not fully implemented: searching for '%s'", term)
	return types.Position{Line: -1, Col: -1}, false
}

func (e *Editor) HighlightMatches(term string) {
	// Basic implementation - can be moved to a find manager later
	if term == "" {
		e.ClearHighlights()
		return
	}

	// Implementation to highlight matches in the buffer
	logger.Debugf("HighlightMatches not fully implemented: highlighting '%s'", term)
}

// SaveBuffer handles buffer saving
func (e *Editor) SaveBuffer() error {
	filePath := ""
	if bufWithFP, ok := e.buffer.(interface{ FilePath() string }); ok {
		filePath = bufWithFP.FilePath()
	}

	err := e.buffer.Save(filePath)
	if err != nil {
		return err
	}

	// Dispatch save event
	if e.eventManager != nil {
		e.eventManager.Dispatch(event.TypeBufferSaved, event.BufferSavedData{FilePath: filePath})
	}
	return nil
}
