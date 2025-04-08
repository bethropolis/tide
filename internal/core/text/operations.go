package text

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/buffer"       // Import main buffer package
	"github.com/bethropolis/tide/internal/core/history" // Add history import
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/types"
)

// Operations handles text insertion/deletion
type Operations struct {
	editor EditorInterface
}

// EditorInterface defines editor methods needed
type EditorInterface interface {
	GetBuffer() buffer.Buffer // Changed return type to concrete buffer.Buffer
	GetCursor() types.Position
	SetCursor(pos types.Position)
	GetEventManager() *event.Manager
	ClearSelection()
	HasSelection() bool
	GetSelection() (start types.Position, end types.Position, ok bool)
	ScrollToCursor()
	GetHistoryManager() *history.Manager // Add GetHistoryManager method
}

// NewOperations creates a text operations manager
func NewOperations(editor EditorInterface) *Operations {
	return &Operations{
		editor: editor,
	}
}

// InsertRune inserts a single rune at cursor
func (o *Operations) InsertRune(r rune) error {
	o.editor.ClearSelection() // Clear selection when typing

	runeBytes := make([]byte, utf8.RuneLen(r))
	utf8.EncodeRune(runeBytes, r)

	cursorBefore := o.editor.GetCursor() // Store cursor before change
	editInfo, err := o.editor.GetBuffer().Insert(cursorBefore, runeBytes)
	if err != nil {
		return err
	}

	// Calculate cursor after the insertion
	cursorAfter := cursorBefore
	if r == '\n' {
		cursorAfter.Line++
		cursorAfter.Col = 0
	} else {
		cursorAfter.Col++
	}
	o.editor.SetCursor(cursorAfter)

	// Record change for undo/redo
	histMgr := o.editor.GetHistoryManager()
	if histMgr != nil {
		change := history.Change{
			Type:          history.InsertAction,
			Text:          runeBytes,
			StartPosition: cursorBefore,
			EndPosition:   cursorAfter,
			CursorBefore:  cursorBefore,
		}
		histMgr.RecordChange(change)
	}

	// Ensure cursor remains visible after insertion/movement
	o.editor.ScrollToCursor()

	// Dispatch event WITH EditInfo
	eventManager := o.editor.GetEventManager()
	if eventManager != nil {
		eventManager.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
	}

	return nil
}

// InsertNewLine inserts a newline and scrolls
func (o *Operations) InsertNewLine() error {
	o.editor.ClearSelection() // Clear selection when typing
	// InsertRune handles the scroll now
	return o.InsertRune('\n')
}

// DeleteBackward deletes character before cursor
func (o *Operations) DeleteBackward() error {
	var editInfo types.EditInfo
	var err error
	cursorBefore := o.editor.GetCursor() // Store cursor before change
	var deletedText []byte
	var start, end types.Position

	// If selection exists, delete selection instead of single char
	if o.editor.HasSelection() {
		start, end, _ = o.editor.GetSelection()

		// Get the text being deleted for undo
		deletedText, err = o.extractTextFromRange(start, end)
		if err != nil {
			return fmt.Errorf("failed to extract selected text: %w", err)
		}

		o.editor.ClearSelection()                               // Clear selection state first
		editInfo, err = o.editor.GetBuffer().Delete(start, end) // Delete range
		if err != nil {
			return fmt.Errorf("buffer delete failed: %w", err)
		}

		o.editor.SetCursor(start) // Move cursor to start of deleted range

		// Record change for undo/redo
		histMgr := o.editor.GetHistoryManager()
		if histMgr != nil {
			change := history.Change{
				Type:          history.DeleteAction,
				Text:          deletedText,
				StartPosition: start,
				EndPosition:   end,
				CursorBefore:  cursorBefore,
			}
			histMgr.RecordChange(change)
		}

		o.editor.ScrollToCursor()

		if o.editor.GetEventManager() != nil {
			o.editor.GetEventManager().Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
		}

		return nil
	}

	// If no selection, proceed with single char delete
	currentPos := o.editor.GetCursor()
	start = currentPos
	end = currentPos

	if currentPos.Col > 0 {
		start.Col--
		// Extract character being deleted
		lineBytes, err := o.editor.GetBuffer().Line(start.Line)
		if err == nil && start.Col < utf8.RuneCount(lineBytes) {
			// Find the rune at the position
			r, _ := utf8.DecodeRune(lineBytes[utf8.RuneCountInString(string(lineBytes[:start.Col])):])
			deletedText = make([]byte, utf8.RuneLen(r))
			utf8.EncodeRune(deletedText, r)
		}
	} else if currentPos.Line > 0 {
		start.Line--
		prevLineBytes, err := o.editor.GetBuffer().Line(start.Line)
		if err != nil {
			return fmt.Errorf("cannot get previous line %d: %w", start.Line, err)
		}
		start.Col = utf8.RuneCount(prevLineBytes)
		// When deleting at beginning of line, we're deleting a newline character
		deletedText = []byte{'\n'}
	} else {
		return nil // At beginning of buffer, nothing to delete
	}

	editInfo, err = o.editor.GetBuffer().Delete(start, end)
	if err != nil {
		return fmt.Errorf("buffer delete failed: %w", err)
	}

	// Record change for undo/redo
	histMgr := o.editor.GetHistoryManager()
	if histMgr != nil && len(deletedText) > 0 {
		change := history.Change{
			Type:          history.DeleteAction,
			Text:          deletedText,
			StartPosition: start,
			EndPosition:   end,
			CursorBefore:  cursorBefore,
		}
		histMgr.RecordChange(change)
	}

	// Cursor moves to the 'start' position
	o.editor.SetCursor(start)

	// Ensure cursor is visible after deletion/movement
	o.editor.ScrollToCursor()

	if o.editor.GetEventManager() != nil {
		o.editor.GetEventManager().Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
	}

	return nil
}

// DeleteForward deletes character after cursor
func (o *Operations) DeleteForward() error {
	var editInfo types.EditInfo
	var err error
	cursorBefore := o.editor.GetCursor() // Store cursor before change
	var deletedText []byte
	var start, end types.Position

	// If selection exists, delete selection instead of single char
	if o.editor.HasSelection() {
		start, end, _ = o.editor.GetSelection()

		// Get the text being deleted for undo
		deletedText, err = o.extractTextFromRange(start, end)
		if err != nil {
			return fmt.Errorf("failed to extract selected text: %w", err)
		}

		o.editor.ClearSelection() // Clear selection state
		editInfo, err = o.editor.GetBuffer().Delete(start, end)
		if err != nil {
			return fmt.Errorf("buffer delete failed: %w", err)
		}

		// Record change for undo/redo
		histMgr := o.editor.GetHistoryManager()
		if histMgr != nil {
			change := history.Change{
				Type:          history.DeleteAction,
				Text:          deletedText,
				StartPosition: start,
				EndPosition:   end,
				CursorBefore:  cursorBefore,
			}
			histMgr.RecordChange(change)
		}

		o.editor.SetCursor(start)
		o.editor.ScrollToCursor()

		if o.editor.GetEventManager() != nil {
			o.editor.GetEventManager().Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
		}

		return nil
	}

	// If no selection, proceed with single char delete
	start = cursorBefore
	end = cursorBefore

	lineBytes, err := o.editor.GetBuffer().Line(start.Line)
	if err != nil {
		return fmt.Errorf("cannot get current line %d: %w", start.Line, err)
	}
	lineRuneCount := utf8.RuneCount(lineBytes)

	if start.Col < lineRuneCount {
		// Deleting within the current line
		end.Col++

		// Extract the character being deleted
		if start.Col < len(lineBytes) {
			// Find the rune at the position
			r, _ := utf8.DecodeRune(lineBytes[utf8.RuneCountInString(string(lineBytes[:start.Col])):])
			deletedText = make([]byte, utf8.RuneLen(r))
			utf8.EncodeRune(deletedText, r)
		}
	} else if start.Line < o.editor.GetBuffer().LineCount()-1 {
		// Deleting at end of line (newline)
		end.Line++
		end.Col = 0
		deletedText = []byte{'\n'} // Newline character
	} else {
		return nil // At end of buffer, nothing to delete
	}

	editInfo, err = o.editor.GetBuffer().Delete(start, end)
	if err != nil {
		return fmt.Errorf("buffer delete failed: %w", err)
	}

	// Record change for undo/redo
	histMgr := o.editor.GetHistoryManager()
	if histMgr != nil && len(deletedText) > 0 {
		change := history.Change{
			Type:          history.DeleteAction,
			Text:          deletedText,
			StartPosition: start,
			EndPosition:   end,
			CursorBefore:  cursorBefore,
		}
		histMgr.RecordChange(change)
	}

	// Cursor position remains at 'start'
	o.editor.SetCursor(start)

	// Ensure cursor is visible
	o.editor.ScrollToCursor()

	if o.editor.GetEventManager() != nil {
		o.editor.GetEventManager().Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
	}

	return nil
}

// extractTextFromRange gets the text content between start and end positions
func (o *Operations) extractTextFromRange(start, end types.Position) ([]byte, error) {
	buf := o.editor.GetBuffer()
	var content bytes.Buffer

	// For single line selection
	if start.Line == end.Line {
		lineBytes, err := buf.Line(start.Line)
		if err != nil {
			return nil, fmt.Errorf("cannot get line %d: %w", start.Line, err)
		}

		// Convert rune indices to byte indices
		startIdx := utf8.RuneCountInString(string(lineBytes[:start.Col]))
		endIdx := utf8.RuneCountInString(string(lineBytes[:end.Col]))

		// Make sure indices are valid
		if startIdx <= len(lineBytes) && endIdx <= len(lineBytes) && startIdx <= endIdx {
			content.Write(lineBytes[startIdx:endIdx])
		}
		return content.Bytes(), nil
	}

	// For multi-line selection
	for lineIdx := start.Line; lineIdx <= end.Line; lineIdx++ {
		lineBytes, err := buf.Line(lineIdx)
		if err != nil {
			return nil, fmt.Errorf("cannot get line %d: %w", lineIdx, err)
		}

		if lineIdx == start.Line {
			// First line - from start.Col to end of line
			startIdx := utf8.RuneCountInString(string(lineBytes[:start.Col]))
			if startIdx <= len(lineBytes) {
				content.Write(lineBytes[startIdx:])
			}
			content.WriteByte('\n') // Add newline except for the last line
		} else if lineIdx == end.Line {
			// Last line - from beginning to end.Col
			endIdx := utf8.RuneCountInString(string(lineBytes[:end.Col]))
			if endIdx <= len(lineBytes) {
				content.Write(lineBytes[:endIdx])
			}
		} else {
			// Middle lines - entire line
			content.Write(lineBytes)
			content.WriteByte('\n')
		}
	}

	return content.Bytes(), nil
}
