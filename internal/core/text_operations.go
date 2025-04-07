package core

import (
	"fmt"
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/types"
)

func (e *Editor) InsertRune(r rune) error {
	e.ClearSelection() // Clear selection when typing

	runeBytes := make([]byte, utf8.RuneLen(r))
	utf8.EncodeRune(runeBytes, r)

	currentPos := e.Cursor
	editInfo, err := e.buffer.Insert(currentPos, runeBytes)
	if err != nil {
		return err
	}

	// Move cursor forward
	if r == '\n' {
		e.Cursor.Line++
		e.Cursor.Col = 0
	} else {
		e.Cursor.Col++
	}

	// Ensure cursor remains visible after insertion/movement
	e.ScrollToCursor()

	// Dispatch event WITH EditInfo
	if e.eventManager != nil {
		e.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
	}

	return nil
}

// InsertNewLine inserts a newline and scrolls.
func (e *Editor) InsertNewLine() error {
	e.ClearSelection() // Clear selection when typing
	// InsertRune handles the scroll now
	return e.InsertRune('\n')
}

func (e *Editor) DeleteBackward() error {
	var editInfo types.EditInfo
	var err error

	// If selection exists, delete selection instead of single char
	if e.HasSelection() {
		start, end, _ := e.GetSelection()
		e.ClearSelection() // Clear selection state first

		editInfo, err = e.buffer.Delete(start, end) // Delete range
		if err != nil {
			return fmt.Errorf("buffer delete failed: %w", err)
		}

		e.Cursor = start // Move cursor to start of deleted range
		e.ScrollToCursor()

		if e.eventManager != nil {
			e.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
		}

		return nil
	}

	// If no selection, proceed with single char delete
	start := e.Cursor
	end := e.Cursor

	if e.Cursor.Col > 0 {
		start.Col--
	} else if e.Cursor.Line > 0 {
		start.Line--
		prevLineBytes, err := e.buffer.Line(start.Line)
		if err != nil {
			return fmt.Errorf("cannot get previous line %d: %w", start.Line, err)
		}
		start.Col = utf8.RuneCount(prevLineBytes)
	} else {
		return nil
	}

	editInfo, err = e.buffer.Delete(start, end)
	if err != nil {
		return fmt.Errorf("buffer delete failed: %w", err)
	}

	// Cursor moves to the 'start' position
	e.Cursor = start

	// Ensure cursor is visible after deletion/movement
	e.ScrollToCursor()

	if e.eventManager != nil && (editInfo != types.EditInfo{}) {
		e.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
	}

	return nil
}

// DeleteForward deletes and scrolls if needed.
func (e *Editor) DeleteForward() error {
	var editInfo types.EditInfo
	var err error

	// If selection exists, delete selection
	if e.HasSelection() {
		start, end, _ := e.GetSelection()
		e.ClearSelection()

		editInfo, err = e.buffer.Delete(start, end)
		if err != nil {
			return fmt.Errorf("buffer delete failed: %w", err)
		}

		e.Cursor = start
		e.ScrollToCursor()

		if e.eventManager != nil {
			e.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
		}

		return nil
	}

	// If no selection, proceed with single char delete
	start := e.Cursor
	end := e.Cursor

	lineBytes, err := e.buffer.Line(e.Cursor.Line)
	if err != nil {
		return fmt.Errorf("cannot get current line %d: %w", e.Cursor.Line, err)
	}
	lineRuneCount := utf8.RuneCount(lineBytes)

	if e.Cursor.Col < lineRuneCount {
		end.Col++
	} else if e.Cursor.Line < e.buffer.LineCount()-1 {
		end.Line++
		end.Col = 0
	} else {
		return nil
	}

	editInfo, err = e.buffer.Delete(start, end)
	if err != nil {
		return fmt.Errorf("buffer delete failed: %w", err)
	}

	// Cursor position remains at 'start'
	e.Cursor = start

	// Let's be safe and scroll anyway
	e.ScrollToCursor()

	if e.eventManager != nil && (editInfo != types.EditInfo{}) {
		e.eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
	}

	return nil
}
