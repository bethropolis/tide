package utils

import (
	"sync"
	"time"
	"unicode/utf8"
)

// RuneIndexToByteOffset converts a rune index to a byte offset in a byte slice.
// Returns -1 if runeIndex is out of bounds.
func RuneIndexToByteOffset(line []byte, runeIndex int) int {
	if runeIndex <= 0 {
		return 0
	}
	byteOffset := 0
	currentRune := 0
	for byteOffset < len(line) {
		if currentRune == runeIndex {
			return byteOffset
		}
		_, size := utf8.DecodeRune(line[byteOffset:])
		byteOffset += size
		currentRune++
	}
	if currentRune == runeIndex {
		return len(line)
	} // Allow index at the very end
	return -1 // Index out of bounds
}

// ByteOffsetToRuneIndex converts a byte offset to a rune index in a byte slice.
func ByteOffsetToRuneIndex(line []byte, byteOffset int) int {
	if byteOffset <= 0 {
		return 0
	}
	if byteOffset > len(line) {
		byteOffset = len(line)
	} // Clamp offset
	runeIndex := 0
	currentOffset := 0
	for currentOffset < byteOffset {
		_, size := utf8.DecodeRune(line[currentOffset:])
		if currentOffset+size > byteOffset {
			break
		} // Don't count rune if offset is within it
		currentOffset += size
		runeIndex++
	}
	return runeIndex
}

// GetLeadingWhitespace returns the leading whitespace characters (spaces/tabs) from a byte slice.
func GetLeadingWhitespace(line []byte) []byte {
	endIndex := 0
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRune(line[i:])
		if r != ' ' && r != '\t' {
			break // Stop at first non-whitespace rune
		}
		endIndex += size
		i += size
	}
	// Return a copy to avoid modifying the original line slice indirectly
	ws := make([]byte, endIndex)
	copy(ws, line[:endIndex])
	return ws
}

// Debouncer provides a way to debounce function calls
type Debouncer struct {
	mutex      sync.Mutex
	timer      *time.Timer
	lastCalled time.Time
}

// Debounce calls the provided function after the specified duration,
// canceling any previous pending calls
func (d *Debouncer) Debounce(duration time.Duration, fn func()) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// Cancel existing timer if present
	if d.timer != nil {
		d.timer.Stop()
	}

	// Schedule new timer
	d.timer = time.AfterFunc(duration, func() {
		d.mutex.Lock()
		d.lastCalled = time.Now()
		d.timer = nil
		d.mutex.Unlock()
		fn()
	})
}
