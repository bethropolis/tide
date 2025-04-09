package clipboard

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/buffer"       // Import the main buffer package
	"github.com/bethropolis/tide/internal/core/history" // Add history import
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
	"github.com/bethropolis/tide/internal/utils" // Import utility package as utils
	// Import for grapheme cluster support
)

// Manager handles clipboard operations
type Manager struct {
	editor    EditorInterface
	clipboard []byte
}

// EditorInterface defines methods needed from editor
type EditorInterface interface {
	GetBuffer() buffer.Buffer // Changed return type to concrete buffer.Buffer
	GetCursor() types.Position
	SetCursor(pos types.Position)
	GetSelection() (start types.Position, end types.Position, ok bool)
	ClearSelection()
	GetEventManager() *event.Manager
	ScrollToCursor()
	MoveCursor(deltaLine, deltaCol int)
	GetHistoryManager() *history.Manager // Add GetHistoryManager method
}

// NewManager creates a new clipboard manager
func NewManager(editor EditorInterface) *Manager {
	return &Manager{
		editor:    editor,
		clipboard: nil,
	}
}

// YankSelection copies selected text to clipboard
func (m *Manager) YankSelection() (bool, error) {
	start, end, ok := m.editor.GetSelection()
	if !ok {
		// No selection active
		return false, nil // Not an error, just nothing to yank
	}

	// Extract the selected text from the buffer
	content, err := m.extractTextFromRange(start, end)
	if err != nil {
		return false, fmt.Errorf("failed to extract selected text for yank: %w", err)
	}

	m.clipboard = content
	logger.Debugf("ClipboardManager: Yanked %d bytes", len(m.clipboard))

	// Clear selection after yank
	m.editor.ClearSelection()

	return true, nil
}

// extractTextFromRange extracts text from a given range in the buffer
func (m *Manager) extractTextFromRange(start, end types.Position) ([]byte, error) {
	var content bytes.Buffer
	buffer := m.editor.GetBuffer()

	// For single line selection
	if start.Line == end.Line {
		lineBytes, err := buffer.Line(start.Line)
		if err != nil {
			return nil, fmt.Errorf("cannot get line %d: %w", start.Line, err)
		}

		// Convert rune indices to byte indices using utilities
		startByteOffset := utils.RuneIndexToByteOffset(lineBytes, start.Col)
		endByteOffset := utils.RuneIndexToByteOffset(lineBytes, end.Col)

		// Make sure indices are valid
		if startByteOffset >= 0 && endByteOffset >= 0 && startByteOffset <= endByteOffset && endByteOffset <= len(lineBytes) {
			content.Write(lineBytes[startByteOffset:endByteOffset])
		} else {
			return nil, fmt.Errorf("invalid byte offsets calculated (%d, %d) for line %d, cols %d-%d",
				startByteOffset, endByteOffset, start.Line, start.Col, end.Col)
		}
		return content.Bytes(), nil
	}

	// For multi-line selection
	for lineIdx := start.Line; lineIdx <= end.Line; lineIdx++ {
		lineBytes, err := buffer.Line(lineIdx)
		if err != nil {
			return nil, fmt.Errorf("cannot get line %d: %w", lineIdx, err)
		}

		if lineIdx == start.Line {
			// First line - from start.Col to end of line
			startByteOffset := utils.RuneIndexToByteOffset(lineBytes, start.Col)
			if startByteOffset >= 0 && startByteOffset <= len(lineBytes) {
				content.Write(lineBytes[startByteOffset:])
				content.WriteByte('\n') // Add newline after first line
			} else {
				return nil, fmt.Errorf("invalid start byte offset %d for line %d, col %d",
					startByteOffset, start.Line, start.Col)
			}
		} else if lineIdx == end.Line {
			// Last line - from beginning to end.Col
			endByteOffset := utils.RuneIndexToByteOffset(lineBytes, end.Col)
			if endByteOffset >= 0 && endByteOffset <= len(lineBytes) {
				content.Write(lineBytes[:endByteOffset])
			} else {
				return nil, fmt.Errorf("invalid end byte offset %d for line %d, col %d",
					endByteOffset, end.Line, end.Col)
			}
		} else {
			// Middle lines - entire line plus newline
			content.Write(lineBytes)
			content.WriteByte('\n')
		}
	}

	return content.Bytes(), nil
}

// Paste inserts clipboard content at cursor
func (m *Manager) Paste() (bool, error) {
	if len(m.clipboard) == 0 {
		// Nothing in clipboard
		return false, nil
	}

	buffer := m.editor.GetBuffer()
	eventMgr := m.editor.GetEventManager()
	cursorBefore := m.editor.GetCursor() // Store cursor before change
	var pastePos types.Position
	var selectedText []byte
	var err error

	// If there's a selection, delete it first
	if start, end, ok := m.editor.GetSelection(); ok {
		// Extract the selected text for history
		selectedText, err = m.extractTextFromRange(start, end)
		if err != nil {
			return false, fmt.Errorf("failed to extract selected text: %w", err)
		}

		m.editor.ClearSelection()
		editInfo, err := buffer.Delete(start, end)
		if err != nil {
			return false, fmt.Errorf("failed to delete selection before paste: %w", err)
		}

		// Record the deletion in history
		histMgr := m.editor.GetHistoryManager()
		if histMgr != nil && len(selectedText) > 0 {
			deleteChange := history.Change{
				Type:          history.DeleteAction,
				Text:          selectedText,
				StartPosition: start,
				EndPosition:   end,
				CursorBefore:  cursorBefore,
			}
			histMgr.RecordChange(deleteChange)
		}

		m.editor.SetCursor(start)
		pastePos = start

		if eventMgr != nil {
			eventMgr.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
		}
	} else {
		pastePos = m.editor.GetCursor()
	}

	clipboardContent := m.clipboard

	// Insert content
	editInfo, err := buffer.Insert(pastePos, clipboardContent)
	if err != nil {
		return false, fmt.Errorf("buffer insert failed during paste: %w", err)
	}

	// Calculate new cursor position based on pasted content
	numLines := bytes.Count(clipboardContent, []byte("\n"))
	lastLine := clipboardContent
	if numLines > 0 {
		lastNewLineIndex := bytes.LastIndexByte(clipboardContent, '\n')
		lastLine = clipboardContent[lastNewLineIndex+1:]
	}
	lastLineRuneCount := utf8.RuneCount(lastLine)

	// Move cursor to the end of the pasted content
	newPos := types.Position{
		Line: pastePos.Line + numLines,
		Col:  0,
	}

	if numLines > 0 {
		newPos.Col = lastLineRuneCount
	} else {
		newPos.Col = pastePos.Col + lastLineRuneCount
	}

	// Record the paste in history
	histMgr := m.editor.GetHistoryManager()
	if histMgr != nil {
		pasteChange := history.Change{
			Type:          history.InsertAction,
			Text:          clipboardContent,
			StartPosition: pastePos,
			EndPosition:   newPos,
			CursorBefore:  cursorBefore,
		}
		histMgr.RecordChange(pasteChange)
	}

	m.editor.SetCursor(newPos)
	m.editor.ScrollToCursor()

	logger.Debugf("ClipboardManager: Pasted %d bytes", len(clipboardContent))
	if eventMgr != nil {
		eventMgr.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
	}

	return true, nil
}
