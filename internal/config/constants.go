package config

import (
	"math"
	"time"
)

// GutterWidth calculates the width of the line-number gutter for a given line
// count and screen width. Returns 0 when there is not enough room.
func GutterWidth(lineCount, screenWidth int) int {
	if lineCount <= 0 {
		lineCount = 1
	}
	maxDigits := int(math.Log10(float64(lineCount))) + 1
	gw := maxDigits + 1 // +1 padding space after digits
	if gw >= screenWidth {
		return 0
	}
	return gw
}

// Base application details
const AppName = "tide"
const ConfigDirName = "tide"
const ThemesDirName = "themes"
const DefaultThemeFileName = "theme.toml"   // Active theme file
const DefaultConfigFileName = "config.toml" // Main config file
const DefaultLogFileName = "tide.log"

// UI Layout
const StatusBarHeight = 1

// Input Behavior
const DefaultLeaderKey = ','
const LeaderTimeout = 500 * time.Millisecond

// Status Bar
const MessageTimeout = 4 * time.Second

// These could be moved to NewDefaultConfig(), keeping here for now
const DefaultTabWidth = 4
const DefaultScrollOff = 3
const SystemClipboard = true
