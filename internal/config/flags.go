// internal/config/flags.go
package config

import (
	"flag"
	"fmt"
	"strings"

	"github.com/bethropolis/tide/internal/logger"
)

// Flags holds values parsed from command-line flags.
// Use pointers to distinguish between unset flags and zero-value flags.
type Flags struct {
	ConfigFilePath *string
	Version 	   *bool
	LogLevel       *string
	LogFilePath    *string
	TabWidth       *int
	ScrollOff      *int
	// Add flags for logger filters
	EnableTags      *string
	DisableTags     *string
	EnablePkgs      *string
	DisablePkgs     *string
	EnableFiles     *string
	DisableFiles    *string
	DebugLog        *bool
	SystemClipboard *bool
}

// DefineFlags sets up the command-line flags and associates them with the Flags struct fields.
func (f *Flags) DefineFlags() {
	f.ConfigFilePath = flag.String("config", "", fmt.Sprintf("Path to TOML configuration file (default ~/.config/%s/%s)", AppName, DefaultConfigFileName))
	f.Version = flag.Bool("version", false, "Show version information and exit")
	f.LogLevel = flag.String("loglevel", "", "Log level (debug, info, warn, error) - Overrides config file")
	f.LogFilePath = flag.String("logfile", "", "Path to write log file (use '-' for stderr) - Overrides config file")
	f.TabWidth = flag.Int("tabwidth", 0, "Number of spaces per tab - Overrides config file")               // Use 0 to indicate unset
	f.ScrollOff = flag.Int("scrolloff", -1, "Lines of context above/below cursor - Overrides config file") // Use -1 to indicate unset
	f.EnableTags = flag.String("log-tags", "", "Comma-separated list of tags to enable - Overrides config file")
	f.DisableTags = flag.String("log-disable-tags", "", "Comma-separated list of tags to disable - Overrides config file")
	f.EnablePkgs = flag.String("log-packages", "", "Comma-separated list of packages to enable - Overrides config file")
	f.DisablePkgs = flag.String("log-disable-packages", "", "Comma-separated list of packages to disable - Overrides config file")
	f.EnableFiles = flag.String("log-files", "", "Comma-separated list of files to enable - Overrides config file")
	f.DisableFiles = flag.String("log-disable-files", "", "Comma-separated list of files to disable - Overrides config file")
	f.DebugLog = flag.Bool("debug-log", false, "Enable verbose debug logging for the logger filtering system")
	f.SystemClipboard = flag.Bool("system-clipboard", false, "Use system clipboard instead of internal clipboard")
}

// ParseFlags parses the defined command-line flags into the Flags struct.
// It returns the remaining non-flag arguments (e.g., the file path).
func (f *Flags) ParseFlags() []string {
	f.DefineFlags()
	flag.Parse()
	return flag.Args() // Return non-flag arguments
}

// ApplyOverrides updates the Config struct with values from flags *if* they were set.
func (f *Flags) ApplyOverrides(cfg *Config, verbose bool) {
	// Visit only processes flags that were actually set
	flag.Visit(func(fl *flag.Flag) {
		if verbose {
			logger.DebugTagf("config", "Applying flag override: %s", fl.Name)
		}
		switch fl.Name {
		case "loglevel":
			if f.LogLevel != nil && *f.LogLevel != "" {
				if verbose {
					logger.DebugTagf("config", "Setting log level from flag: %s", *f.LogLevel)
				}
				cfg.Logger.LogLevel = *f.LogLevel
			}
		case "logfile":
			if f.LogFilePath != nil { // Empty string is valid ("-")
				if verbose {
					logger.DebugTagf("config", "Setting log file path from flag: %s", *f.LogFilePath)
				}
				cfg.Logger.LogFilePath = *f.LogFilePath
			}
		case "tabwidth":
			if f.TabWidth != nil && *f.TabWidth > 0 {
				if verbose {
					logger.DebugTagf("config", "Setting tab width from flag: %d", *f.TabWidth)
				}
				cfg.Editor.TabWidth = *f.TabWidth // Only override if positive
			}
		case "scrolloff":
			if f.ScrollOff != nil && *f.ScrollOff >= 0 {
				if verbose {
					logger.DebugTagf("config", "Setting scroll off from flag: %d", *f.ScrollOff)
				}
				cfg.Editor.ScrollOff = *f.ScrollOff // Only override if non-negative
			}
		case "system-clipboard":
			if f.SystemClipboard != nil {
				if verbose {
					logger.DebugTagf("config", "Setting system clipboard from flag: %v", *f.SystemClipboard)
				}
				cfg.Editor.SystemClipboard = *f.SystemClipboard
			}
		case "log-tags":
			if f.EnableTags != nil && *f.EnableTags != "" {
				tags := splitCommaList(*f.EnableTags)
				if verbose {
					logger.DebugTagf("config", "Setting enabled tags from flag: %v", tags)
				}
				cfg.Logger.EnabledTags = tags
			}
		case "log-disable-tags":
			if f.DisableTags != nil && *f.DisableTags != "" {
				tags := splitCommaList(*f.DisableTags)
				if verbose {
					logger.DebugTagf("config", "Setting disabled tags from flag: %v", tags)
				}
				cfg.Logger.DisabledTags = tags
			}
		case "log-packages":
			if f.EnablePkgs != nil && *f.EnablePkgs != "" {
				pkgs := splitCommaList(*f.EnablePkgs)
				if verbose {
					logger.DebugTagf("config", "Setting enabled packages from flag: %v", pkgs)
				}
				cfg.Logger.EnabledPackages = pkgs
			}
		case "log-disable-packages":
			if f.DisablePkgs != nil && *f.DisablePkgs != "" {
				pkgs := splitCommaList(*f.DisablePkgs)
				if verbose {
					logger.DebugTagf("config", "Setting disabled packages from flag: %v", pkgs)
				}
				cfg.Logger.DisabledPackages = pkgs
			}
		case "log-files":
			if f.EnableFiles != nil && *f.EnableFiles != "" {
				files := splitCommaList(*f.EnableFiles)
				if verbose {
					logger.DebugTagf("config", "Setting enabled files from flag: %v", files)
				}
				cfg.Logger.EnabledFiles = files
			}
		case "log-disable-files":
			if f.DisableFiles != nil && *f.DisableFiles != "" {
				files := splitCommaList(*f.DisableFiles)
				if verbose {
					logger.DebugTagf("config", "Setting disabled files from flag: %v", files)
				}
				cfg.Logger.DisabledFiles = files
			}
		}
	})
}

// Helper function to split comma-separated list (can be moved to util)
func splitCommaList(list string) []string {
	if list == "" {
		return nil
	}
	items := strings.Split(list, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
