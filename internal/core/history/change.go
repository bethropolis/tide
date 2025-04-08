// Package history provides undo/redo functionality via a change history stack.
package history

import "github.com/bethropolis/tide/internal/types"

// ActionType indicates whether text was inserted or deleted.
type ActionType int

const (
	InsertAction ActionType = iota
	DeleteAction
)

// Change represents a single, reversible text operation.
type Change struct {
	Type          ActionType
	Text          []byte         // Text inserted or text deleted
	StartPosition types.Position // Where the change began
	EndPosition   types.Position // Position after inserted text, or end position of deleted text
	CursorBefore  types.Position // Cursor position *before* this change was applied
}
