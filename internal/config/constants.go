package config

import "time"

// Default values - Make these configurable later

// UI Layout
const StatusBarHeight = 1 // Height of the status bar in lines

// Editor Behavior
const DefaultScrollOff = 3    // Default vertical scroll-off margin
const DefaultTabWidth = 4     // Default width of a tab character
const SystemClipboard = false // Use internal clipboard by default

// Input Behavior
const DefaultLeaderKey = ',' // Default leader key rune
const LeaderTimeout = 500 * time.Millisecond

// Status Bar
const MessageTimeout = 4 * time.Second // Default duration for status messages
