// cmd/tide/main.go
package main

import (
	"flag" // Use flags for configuration
	stlog "log" // Use standard log for FATAL errors before logger is ready
	"log/slog"
	"os"

	"github.com/bethropolis/tide/internal/app"
	"github.com/bethropolis/tide/internal/logger" // Import logger package
)

var (
	logFilePath string
	logLevel    string
	filePath    string 
)

func main() {
	// --- Argument & Flag Parsing ---
	flag.StringVar(&logFilePath, "logfile", "tide.log", "Path to write log file")
	flag.StringVar(&logLevel, "loglevel", "debug", "Log level (debug, info, warn, error)")
	// Allow specifying file as first non-flag argument
	flag.Parse()
	if flag.NArg() > 0 {
		filePath = flag.Arg(0)
	}

	// --- Logger Initialization ---
	level := slog.LevelDebug // Default
	switch logLevel {
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	case "debug":
		// already debug level
	default:
		stlog.Printf("Warning: Invalid log level '%s', defaulting to debug", logLevel)
	}

	// Open log file
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		stlog.Fatalf("Failed to open log file '%s': %v", logFilePath, err)
	}
	defer logFile.Close() // Ensure log file is closed on exit

	// Initialize our logger package
	logger.Init(level, logFile) // Pass the file writer

	logger.Infof("Starting Tide editor...") // Use the new logger
	logger.Debugf("Log level set to: %s", level.String())
	logger.Debugf("Log file: %s", logFilePath)
	if filePath != "" {
		logger.Debugf("File path specified: %s", filePath)
	} else {
		logger.Debugf("No file specified, starting empty.")
	}


	// --- Create and Run App ---
	tideApp, err := app.NewApp(filePath) // App handles internal setup
	if err != nil {
		logger.Errorf("Error initializing application: %v", err) // Use logger
		// logger.Cleanup() // If logger managed file closing
		os.Exit(1)
	}

	if err := tideApp.Run(); err != nil {
		logger.Errorf("Application exited with error: %v", err) // Use logger
		// logger.Cleanup()
		os.Exit(1)
	}

	logger.Infof("Tide editor finished.") // Use logger
	// logger.Cleanup()
	os.Exit(0)
}