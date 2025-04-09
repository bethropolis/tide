package types

import sitter "github.com/smacker/go-tree-sitter"

// EditInfo encapsulates the information needed for tree-sitter's Edit function.
type EditInfo struct {
	StartIndex     uint32       // Start byte of the edit
	OldEndIndex    uint32       // End byte of the old text
	NewEndIndex    uint32       // End byte of the new text
	StartPosition  sitter.Point // Start position (row, column)
	OldEndPosition sitter.Point // Old end position
	NewEndPosition sitter.Point // New end position
}
