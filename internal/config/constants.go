package config

import "time"

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
