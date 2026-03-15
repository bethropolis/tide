// Package history provides undo/redo functionality via a change history stack.
package history

import "github.com/bethropolis/tide/internal/types"

// ActionType indicates whether text was inserted or deleted, or a group transaction.
type ActionType int

const (
	InsertAction      ActionType = iota
	DeleteAction                 // Text deleted from the buffer
	TransactionAction            // A grouped transaction of multiple changes
)

// Change represents a single, reversible text operation, or a group transaction.
type Change struct {
	Type          ActionType
	Text          []byte         // Text inserted or text deleted (unused for TransactionAction)
	StartPosition types.Position // Where the change began (unused for TransactionAction)
	EndPosition   types.Position // Position after inserted text, or end position of deleted text
	CursorBefore  types.Position // Cursor position *before* this change was applied

	// For TransactionAction: the grouped sub-changes applied in order.
	Children []Change
}
