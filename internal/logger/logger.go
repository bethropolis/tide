// internal/logger/logger.go
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var (
	defaultLogger *slog.Logger
	loggerConfig  *Config // Store the processed config
	initOnce      sync.Once
	logOutput     io.Writer = io.Discard
	debugFilter   bool      = false // Set to true to see filtering debug info
)

// EnableFilterDebug turns on debug output for filtering decisions
func EnableFilterDebug(enable bool) {
	debugFilter = enable
}

// Init initializes the logger package with the given configuration.
func Init(cfg Config) {
	initOnce.Do(func() {
		// Process the config
		cfg.process()
		loggerConfig = &cfg // Store processed config

		// --- Diagnostic Log (only shown when debug is enabled) ---
		if debugFilter {
			fmt.Fprintf(os.Stderr, "[Logger Init] Configured Level String: %s\n", cfg.LogLevel)
			if cfg.level != nil {
				fmt.Fprintf(os.Stderr, "[Logger Init] Parsed Level: %s (%v)\n", cfg.level.Level().String(), cfg.level.Level())
			} else {
				fmt.Fprintf(os.Stderr, "[Logger Init] Parsed Level: <nil>\n")
			}
			fmt.Fprintf(os.Stderr, "[Logger Init] Disabled Packages Set: %v\n", cfg.disabledPackagesSet)
			fmt.Fprintf(os.Stderr, "[Logger Init] Disabled Files Set: %v\n", cfg.disabledFilesSet)
			fmt.Fprintf(os.Stderr, "[Logger Init] Disabled Tags Set: %v\n", cfg.disabledTagsSet)
		}

		// Set output writer
		var err error
		if cfg.LogFilePath == "-" {
			logOutput = os.Stderr
		} else if cfg.LogFilePath == "" {
			// Use ~/.config/tide/tide.log as default
			usr, err := user.Current()
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: Failed to get current user: %v\n", err)
				logOutput = os.Stderr
			} else {
				configDir := filepath.Join(usr.HomeDir, ".config", "tide")

				// Create directory if it doesn't exist
				if err := os.MkdirAll(configDir, 0755); err != nil {
					fmt.Fprintf(os.Stderr, "ERROR: Failed to create directory '%s': %v\n", configDir, err)
					logOutput = os.Stderr
				} else {
					logFilePath := filepath.Join(configDir, "tide.log")
					var file *os.File
					file, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
					if err != nil {
						logOutput = os.Stderr
						fmt.Fprintf(os.Stderr, "ERROR: Failed to open log file '%s', falling back to stderr: %v\n", logFilePath, err)
					} else {
						logOutput = file
						// Note: Closing the log file should be handled in main
					}
				}
			}
		} else {
			var file *os.File
			file, err = os.OpenFile(cfg.LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err != nil {
				// Fallback to stderr if file opening fails
				logOutput = os.Stderr
				fmt.Fprintf(os.Stderr, "ERROR: Failed to open log file '%s', falling back to stderr: %v\n", cfg.LogFilePath, err)
			} else {
				logOutput = file
				// Note: Closing the log file should be handled in main
			}
		}

		// Create handler with our options
		opts := slog.HandlerOptions{
			Level:     cfg.level,
			AddSource: true, // This adds the source information
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.SourceKey {
					source := a.Value.Any().(*slog.Source)
					// Extract just the package and filename
					dir := filepath.Base(filepath.Dir(source.File))
					file := filepath.Base(source.File)
					// Replace with a simplified format: "package/file.go"
					a.Value = slog.StringValue(fmt.Sprintf("%s/%s", dir, file))
				}
				if a.Key == slog.TimeKey {
					a.Value = slog.StringValue(a.Value.Time().Format(time.TimeOnly))
				}
				return a
			},
		}

		// Create a text handler that preserves the source information
		baseHandler := slog.NewTextHandler(logOutput, &opts)
		filteringHandler := newFilteringHandler(baseHandler, loggerConfig)
		defaultLogger = slog.New(filteringHandler)

		// Print minimal diagnostic info about filter settings to stderr
		if len(cfg.DisabledPackages) > 0 {
			if debugFilter {
				fmt.Fprintf(os.Stderr, "INFO: Logger initialized with disabled packages: %v\n", cfg.DisabledPackages)
			}
		}
		if len(cfg.EnabledPackages) > 0 {
			if debugFilter {
				fmt.Fprintf(os.Stderr, "INFO: Logger initialized with enabled packages: %v\n", cfg.EnabledPackages)
			}
		}

		// IMPORTANT: We log directly to the handler to avoid the deadlock
		r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf("Logger initialized. Final Effective Level: %s", cfg.level.Level().String()), 0)
		_ = defaultLogger.Handler().Handle(context.Background(), r)

		if len(cfg.EnabledTags) > 0 {
			r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf("Enabled Tags: %v", cfg.EnabledTags), 0)
			_ = defaultLogger.Handler().Handle(context.Background(), r)
		}
		if len(cfg.DisabledTags) > 0 {
			r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf("Disabled Tags: %v", cfg.DisabledTags), 0)
			_ = defaultLogger.Handler().Handle(context.Background(), r)
		}
	})
}

// Ensure logger is initialized with default config if Init wasn't called.
func ensureInitialized() {
	initOnce.Do(func() {
		// Initialize with default settings if Init() wasn't called explicitly
		defaultConfig := NewConfig()
		fmt.Fprintln(os.Stderr, "WARN: Logger not explicitly initialized, using defaults (Level: Info)")
		Init(defaultConfig)
	})
}

// logAtLevel creates and logs a record at the specified level, capturing the correct caller source.
func logAtLevel(level slog.Level, format string, args ...interface{}) {
	// Check if logger is initialized
	if defaultLogger == nil {
		fmt.Fprintf(os.Stderr, "ERROR: Logger accessed before initialization completed for message: "+format+"\n", args...)
		ensureInitialized()
		if defaultLogger == nil {
			fmt.Fprintf(os.Stderr, "FATAL: Logger initialization failed critically.\n")
			os.Exit(1)
		}
	}

	// Check level early to avoid overhead if disabled
	if !defaultLogger.Enabled(context.Background(), level) {
		return
	}

	// Capture caller info - more reliable with 2 frames to get the actual calling location
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		pc = 0
	}

	// Create record with source information via PC - let the handler extract source details
	r := slog.NewRecord(time.Now(), level, fmt.Sprintf(format, args...), pc)

	// Handle the record - the ReplaceAttr in the handler will format the source correctly
	_ = defaultLogger.Handler().Handle(context.Background(), r)
}

// logAtLevelWithTag adds a tag attribute before logging.
func logAtLevelWithTag(level slog.Level, tag string, format string, args ...interface{}) {
	// Check if logger is initialized
	if defaultLogger == nil {
		fmt.Fprintf(os.Stderr, "ERROR: Logger accessed before initialization completed for tagged message: "+format+"\n", args...)
		ensureInitialized()
		if defaultLogger == nil {
			fmt.Fprintf(os.Stderr, "FATAL: Logger initialization failed critically.\n")
			os.Exit(1)
		}
	}

	if !defaultLogger.Enabled(context.Background(), level) {
		return
	}

	// Capture caller info - more reliable with 2 frames to get the actual calling location
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		pc = 0
	}

	// Create record with PC - the handler will extract source info
	r := slog.NewRecord(time.Now(), level, fmt.Sprintf(format, args...), pc)

	// Add just the tag attribute
	r.AddAttrs(slog.String(tagKey, tag))

	// Handle the record
	_ = defaultLogger.Handler().Handle(context.Background(), r)
}

// --- Standard logging functions ---

// Debugf logs a debug message using Printf-style formatting.
func Debugf(format string, args ...interface{}) {
	logAtLevel(slog.LevelDebug, format, args...)
}

// Infof logs an info message using Printf-style formatting.
func Infof(format string, args ...interface{}) {
	logAtLevel(slog.LevelInfo, format, args...)
}

// Warnf logs a warning message using Printf-style formatting.
func Warnf(format string, args ...interface{}) {
	logAtLevel(slog.LevelWarn, format, args...)
}

// Errorf logs an error message using Printf-style formatting.
func Errorf(format string, args ...interface{}) {
	logAtLevel(slog.LevelError, format, args...)
}

// Fatalf logs an error message then exits.
func Fatalf(format string, args ...interface{}) {
	logAtLevel(slog.LevelError, format, args...)
	os.Exit(1)
}

// --- Tagged logging functions ---

// DebugTagf logs a debug message with a tag using Printf-style formatting.
func DebugTagf(tag, format string, args ...interface{}) {
	logAtLevelWithTag(slog.LevelDebug, tag, format, args...)
}

// InfoTagf logs an info message with a tag using Printf-style formatting.
func InfoTagf(tag, format string, args ...interface{}) {
	logAtLevelWithTag(slog.LevelInfo, tag, format, args...)
}

// WarnTagf logs a warning message with a tag using Printf-style formatting.
func WarnTagf(tag, format string, args ...interface{}) {
	logAtLevelWithTag(slog.LevelWarn, tag, format, args...)
}

// ErrorTagf logs an error message with a tag using Printf-style formatting.
func ErrorTagf(tag, format string, args ...interface{}) {
	logAtLevelWithTag(slog.LevelError, tag, format, args...)
}

// Get retrieves the configured logger instance.
func Get() *slog.Logger {
	ensureInitialized() // KEEP ensureInitialized HERE in Get()
	return defaultLogger
}
