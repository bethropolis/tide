package utils

import (
	"unicode/utf8"

	"github.com/bethropolis/tide/internal/logger"
)

// ByteOffsetToRuneIndex converts a byte offset to a rune index in a byte slice
func ByteOffsetToRuneIndex(line []byte, byteOffset int) int {
	if byteOffset <= 0 {
		return 0
	}
	if byteOffset > len(line) {
		byteOffset = len(line)
	}

	runeIndex := 0
	currentOffset := 0
	for currentOffset < byteOffset {
		_, size := utf8.DecodeRune(line[currentOffset:])
		// Log potential issues
		if size == 0 && currentOffset < len(line) {
			logger.Warnf("ByteOffsetToRuneIndex: Zero rune size at offset %d of %d", currentOffset, len(line))
			break
		}
		if currentOffset+size > byteOffset {
			break
		}
		currentOffset += size
		runeIndex++
	}
	return runeIndex
}

// CaptureNameToStyleName maps tree-sitter capture names to theme style names
func CaptureNameToStyleName(captureName string) string {
	// Strip the leading '@' if present
	if len(captureName) > 0 && captureName[0] == '@' {
		captureName = captureName[1:]
	}

	return captureName // Return full name for maximum specificity
}
