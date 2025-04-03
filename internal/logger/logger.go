// internal/logger/logger.go
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime" // <<< Need runtime for caller info
	"sync"
	"time"
)

var (
	defaultLogger *slog.Logger
	logLevel      *slog.LevelVar
	initOnce      sync.Once
	logOutput     io.Writer = io.Discard
)

// Init initializes the logger package.
func Init(level slog.Level, output io.Writer) {
	initOnce.Do(func() {
		if output == nil { output = io.Discard }
		logOutput = output
		logLevel = new(slog.LevelVar)
		logLevel.Set(level)

		opts := slog.HandlerOptions{
			Level:     logLevel,
			AddSource: true, // Keep this enabled
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.SourceKey {
					source := a.Value.Any().(*slog.Source)
					source.File = filepath.Base(source.File) // Base filename is good
				}
				if a.Key == slog.TimeKey {
					a.Value = slog.StringValue(a.Value.Time().Format(time.TimeOnly))
				}
				return a
			},
		}
		handler := slog.NewTextHandler(output, &opts)
		defaultLogger = slog.New(handler)
		// Log initialization message using the underlying handler directly
		// to avoid source location issues with the wrappers during init.
        r := slog.NewRecord(time.Now(), slog.LevelInfo, "Logger initialized", 0) // PC=0 means no source info calculated here
		r.AddAttrs(slog.String("level", level.String()))
        _ = handler.Handle(context.Background(), r) // Use handler directly for init message
	})
}


// Ensure logger is initialized, providing a safe default if Init wasn't called.
// This default logger won't output anywhere unless Init configures it.
func ensureInitialized() {
	initOnce.Do(func() {
		// Initialize with default settings (level Info, output Discard) if Init() wasn't called explicitly
		logLevel = new(slog.LevelVar)
		logLevel.Set(slog.LevelInfo) // Default level if not explicitly initialized
		handler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: logLevel})
		defaultLogger = slog.New(handler)
		// Log a warning if this implicit initialization happens?
		// Maybe not, could be noisy if user *intends* no logs.
	})
}

// logAtLevel creates and logs a record at the specified level, capturing the correct caller source.
func logAtLevel(level slog.Level, format string, args ...interface{}) {
    ensureInitialized()
    // Check level early to avoid overhead of Callers and Sprintf if disabled
    if !defaultLogger.Enabled(context.Background(), level) {
        return
    }

    // --- Capture Caller ---
    var pcs [1]uintptr
    // Skip 3 frames:
    // 1. runtime.Callers itself
    // 2. logAtLevel (this function)
    // 3. The wrapper function (Debugf, Infof, etc.)
    runtime.Callers(3, pcs[:]) // Capture the caller of Debugf/Infof etc.

    // --- Create Record ---
    r := slog.NewRecord(time.Now(), level, fmt.Sprintf(format, args...), pcs[0]) // Pass PC for source info

    // --- Handle the Record ---
    // Use context.Background() for now, could pass context down if needed later
    _ = defaultLogger.Handler().Handle(context.Background(), r)
}


// --- Wrapper Functions (Use the new helper) ---

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
	// Log the message using the helper to get correct source location
	logAtLevel(slog.LevelError, format, args...)

	// Ensure log might be flushed (best effort, depends on underlying writer)
	// If logOutput is os.File, Close() call in main's defer handles flushing.
	// If it's os.Stderr, it's typically unbuffered.

	os.Exit(1)
}


// Get retrieves the configured logger instance.
func Get() *slog.Logger {
	ensureInitialized()
	return defaultLogger
}