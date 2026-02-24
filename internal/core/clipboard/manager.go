package clipboard

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"github.com/atotto/clipboard"                       // <<< Import clipboard library
	"github.com/bethropolis/tide/internal/buffer"       // Import the main buffer package
	"github.com/bethropolis/tide/internal/core/history" // Add history import
	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/types"
	// Import for grapheme cluster support
)

// Manager handles clipboard operations
type Manager struct {
	editor             EditorInterface
	internalClipboard  []byte // Renamed for clarity
	useSystemClipboard bool   // <<< Add flag
}

// EditorInterface defines methods needed from editor
type EditorInterface interface {
	GetBuffer() buffer.Buffer // Changed return type to concrete buffer.Buffer
	GetCursor() types.Position
	SetCursor(pos types.Position)
	GetSelection() (start types.Position, end types.Position, ok bool)
	ClearSelection()
	IsLinewise() bool
	GetEventManager() *event.Manager
	ScrollToCursor()
	MoveCursor(deltaLine, deltaCol int)
	GetHistoryManager() *history.Manager // Add GetHistoryManager method
}

// NewManager creates a new clipboard manager, accepting the config flag
func NewManager(editor EditorInterface, useSystem bool) *Manager { // <<< Add useSystem bool
	return &Manager{
		editor:             editor,
		internalClipboard:  nil,
		useSystemClipboard: useSystem, // <<< Store the flag
	}
}

// getEffectiveSelection extends the selection if linewise is true
func (m *Manager) getEffectiveSelection() (types.Position, types.Position, bool) {
	start, end, ok := m.editor.GetSelection()
	if !ok {
		return start, end, false
	}

	if m.editor.IsLinewise() {
		// Linewise selection: Start from column 0 of start line, end at end of end line (or beginning of next line)
		start.Col = 0

		// To include the newline of the last line, we select up to line + 1, col 0
		end.Line++
		end.Col = 0

		buf := m.editor.GetBuffer()
		if end.Line > buf.LineCount() {
			end.Line = buf.LineCount()
			// If it's the very last line without a newline, we might just select to the end of the line
			if lastLine, err := buf.Line(buf.LineCount() - 1); err == nil {
				end.Line = buf.LineCount() - 1
				end.Col = utf8.RuneCount(lastLine)
			}
		}
	}
	return start, end, true
}

// YankSelection copies selected text to clipboard
func (m *Manager) YankSelection() (bool, error) {
	start, end, ok := m.getEffectiveSelection()
	if !ok {
		// No selection active
		return false, nil // Not an error, just nothing to yank
	}

	// Extract the selected text from the buffer
	content, err := m.extractTextFromRange(start, end)
	if err != nil {
		return false, fmt.Errorf("failed to extract selected text for yank: %w", err)
	}

	if m.useSystemClipboard {
		err = clipboard.WriteAll(string(content))
		if err != nil {
			return false, fmt.Errorf("failed to write to system clipboard: %w", err)
		}
		logger.Debugf("ClipboardManager: Yanked %d bytes to system clipboard", len(content))
	} else {
		m.internalClipboard = content
		logger.Debugf("ClipboardManager: Yanked %d bytes to internal clipboard", len(m.internalClipboard))
	}

	// Clear selection after yank
	m.editor.ClearSelection()

	return true, nil
}

// extractTextFromRange extracts text from a given range in the buffer
func (m *Manager) extractTextFromRange(start, end types.Position) ([]byte, error) {
	return []byte(m.editor.GetBuffer().GetText(start, end)), nil
}

// CutSelection copies and deletes selected text to clipboard
func (m *Manager) CutSelection() (bool, error) {
	start, end, ok := m.getEffectiveSelection()
	if !ok {
		return false, nil
	}

	// Extract the selected text from the buffer
	content, err := m.extractTextFromRange(start, end)
	if err != nil {
		return false, fmt.Errorf("failed to extract selected text for cut: %w", err)
	}

	if m.useSystemClipboard {
		err = clipboard.WriteAll(string(content))
		if err != nil {
			return false, fmt.Errorf("failed to write to system clipboard: %w", err)
		}
		logger.Debugf("ClipboardManager: Cut %d bytes to system clipboard", len(content))
	} else {
		m.internalClipboard = content
		logger.Debugf("ClipboardManager: Cut %d bytes to internal clipboard", len(m.internalClipboard))
	}

	cursorBefore := m.editor.GetCursor()

	// Delete selection
	m.editor.ClearSelection()
	editInfo, err := m.editor.GetBuffer().Delete(start, end)
	if err != nil {
		return false, fmt.Errorf("failed to delete selection during cut: %w", err)
	}

	// Record change for undo/redo
	histMgr := m.editor.GetHistoryManager()
	if histMgr != nil {
		change := history.Change{
			Type:          history.DeleteAction,
			Text:          content,
			StartPosition: start,
			EndPosition:   end,
			CursorBefore:  cursorBefore,
		}
		histMgr.RecordChange(change)
	}

	m.editor.SetCursor(start)
	m.editor.ScrollToCursor()

	if eventMgr := m.editor.GetEventManager(); eventMgr != nil {
		eventMgr.Dispatch(event.TypeBufferModified, event.BufferModifiedData{Edit: editInfo})
	}

	return true, nil
}

// Paste inserts clipboard content at cursor. If after is true, it pastes after the cursor (like vim 'p').
// If after is false, it pastes before the cursor (like vim 'P').
func (m *Manager) Paste(after bool) (bool, error) {
	var clipboardContent []byte
	var err error

	if m.useSystemClipboard {
		content, err := clipboard.ReadAll()
		if err != nil {
			return false, fmt.Errorf("failed to read from system clipboard: %w", err)
		}
		clipboardContent = []byte(content)
		logger.Debugf("ClipboardManager: Read %d bytes from system clipboard", len(clipboardContent))
	} else {
		if len(m.internalClipboard) == 0 {
			// Nothing in clipboard
			return false, nil
		}
		clipboardContent = m.internalClipboard
		logger.Debugf("ClipboardManager: Read %d bytes from internal clipboard", len(clipboardContent))
	}

	buffer := m.editor.GetBuffer()
	eventMgr := m.editor.GetEventManager()
	cursorBefore := m.editor.GetCursor() // Store cursor before change
	var pastePos types.Position
	var selectedText []byte

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

		// Handle Vim-style linewise paste
		isLinewise := len(clipboardContent) > 0 && clipboardContent[len(clipboardContent)-1] == '\n'

		if isLinewise {
			if after {
				// Paste below current line
				pastePos.Line++
				pastePos.Col = 0

				// Make sure the line exists
				if pastePos.Line >= buffer.LineCount() {
					// We need to add a newline at the end of the file first
					lastLineIdx := buffer.LineCount() - 1
					if lastLine, err := buffer.Line(lastLineIdx); err == nil {
						endPos := types.Position{Line: lastLineIdx, Col: utf8.RuneCount(lastLine)}
						buffer.Insert(endPos, []byte("\n"))
					}
					pastePos.Line = buffer.LineCount() - 1
				}
			} else {
				// Paste above current line
				pastePos.Col = 0
			}
		} else {
			if after {
				// Paste after current character
				lineBytes, _ := buffer.Line(pastePos.Line)
				if pastePos.Col < utf8.RuneCount(lineBytes) {
					pastePos.Col++
				}
			}
		}
	}

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

	// Move cursor to the start of the pasted content if linewise, or end if character-wise
	var newPos types.Position
	isLinewise := len(clipboardContent) > 0 && clipboardContent[len(clipboardContent)-1] == '\n'
	if isLinewise {
		newPos = types.Position{Line: pastePos.Line, Col: 0} // Move to start of first pasted line
	} else {
		newPos = types.Position{
			Line: pastePos.Line + numLines,
			Col:  0,
		}

		if numLines > 0 {
			newPos.Col = lastLineRuneCount
		} else {
			// Check if we paste at the end, so we place the cursor on the last character instead of past it
			newPos.Col = pastePos.Col + lastLineRuneCount - 1
			if newPos.Col < 0 {
				newPos.Col = 0
			}
		}
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
