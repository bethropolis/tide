// Package logger provides configurable logging capabilities
package logger

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Config holds all settings for the logger.
type Config struct {
	// LogLevel specifies the minimum level to log (e.g., "debug", "info", "warn", "error").
	LogLevel string

	// LogFilePath is the path to the output log file. Use empty or "-" for stderr.
	LogFilePath string

	// --- Filtering Options ---

	// EnabledTags only logs messages with these tags (if non-empty).
	EnabledTags []string
	// DisabledTags prevents logging messages with these tags. Overrides EnabledTags.
	DisabledTags []string

	// EnabledPackages only logs messages originating from these packages (if non-empty).
	// Package name is the immediate directory name (e.g., "core", "theme", "app").
	EnabledPackages []string
	// DisabledPackages prevents logging from these packages. Overrides EnabledPackages.
	DisabledPackages []string

	// EnabledFiles only logs messages originating from these filenames (if non-empty).
	// Filename is the base name (e.g., "editor.go", "manager.go").
	EnabledFiles []string
	// DisabledFiles prevents logging from these filenames. Overrides EnabledFiles.
	DisabledFiles []string

	// --- Internal processed fields ---
	level               slog.Leveler
	enabledTagsSet      map[string]struct{}
	disabledTagsSet     map[string]struct{}
	enabledPackagesSet  map[string]struct{}
	disabledPackagesSet map[string]struct{}
	enabledFilesSet     map[string]struct{}
	disabledFilesSet    map[string]struct{}
}

// NewConfig creates a new Config with default values
func NewConfig() Config {
	return Config{
		LogLevel:    "info",
		LogFilePath: "",
	}
}

// process parses string levels/lists into efficient internal formats.
func (c *Config) process() {
	// Default level
	c.level = slog.LevelInfo
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		c.level = slog.LevelDebug
	case "info":
		c.level = slog.LevelInfo
	case "warn", "warning":
		c.level = slog.LevelWarn
	case "error", "err":
		c.level = slog.LevelError
	}

	// Print diagnostic information if debug enabled
	if debugFilter {
		fmt.Fprintf(os.Stderr, "[CONFIG PROCESS] Before conversion - DisabledPackages: %v\n", c.DisabledPackages)
		fmt.Fprintf(os.Stderr, "[CONFIG PROCESS] Before conversion - EnabledPackages: %v\n", c.EnabledPackages)
	}

	// Convert filter lists to sets for efficient lookup
	c.enabledTagsSet = sliceToSet(c.EnabledTags)
	c.disabledTagsSet = sliceToSet(c.DisabledTags)
	c.enabledPackagesSet = sliceToSet(c.EnabledPackages)
	c.disabledPackagesSet = sliceToSet(c.DisabledPackages)
	c.enabledFilesSet = sliceToSet(c.EnabledFiles)
	c.disabledFilesSet = sliceToSet(c.DisabledFiles)

	// Print diagnostic information after conversion if debug enabled
	if debugFilter {
		fmt.Fprintf(os.Stderr, "[CONFIG PROCESS] After conversion - disabledPackagesSet: %v\n", c.disabledPackagesSet)
		fmt.Fprintf(os.Stderr, "[CONFIG PROCESS] After conversion - enabledPackagesSet: %v\n", c.enabledPackagesSet)
	}
}

// helper function to convert slice to set
func sliceToSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item != "" { // Ignore empty strings
			set[strings.ToLower(item)] = struct{}{} // Use lowercase for case-insensitive matching
		}
	}
	if len(set) == 0 {
		return nil // Use nil map if empty, simplifies checks later
	}
	return set
}
