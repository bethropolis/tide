// cmd/tide/main.go
package main

import (
	"fmt" // Keep fmt for error printing
	"os"

	"github.com/bethropolis/tide/internal/app"
	"github.com/bethropolis/tide/internal/config"
	"github.com/bethropolis/tide/internal/logger"
)


var (
	Version   = "v.0.1.0" 
	Commit    = "n/a"
	BuildDate = "n/a"
)

func main() {
	// 1. Define and Parse Flags
	flags := &config.Flags{}
	args := flags.ParseFlags() // Parses flags and gets non-flag args

	if *flags.Version {
		printVersion()
		os.Exit(0)
	}

	filePathArg := ""
	if len(args) > 0 {
		filePathArg = args[0] // File to open is the first non-flag arg
	}

	// 2. Load Configuration (Defaults -> File -> Flag Overrides)
	// Pass the flag struct pointer so LoadConfig can apply overrides
	cfg, loadErr := config.LoadConfig(*flags.ConfigFilePath, flags)
	if loadErr != nil {
		// Log warning about file load error, but continue with defaults/flags
		// Need a temporary logger setup or print directly
		fmt.Fprintf(os.Stderr, "WARN: Error loading config file: %v\n", loadErr)
	}

	// 3. Initialize Logger (using the final loaded config)
	logger.Init(cfg.Logger)

	// Enable filter debugging if requested
	if flags.DebugLog != nil && *flags.DebugLog {
		logger.EnableFilterDebug(true)
	}

	// Special case for "filter" log level
	if cfg.Logger.LogLevel == "filter" {
		logger.EnableFilterDebug(true)
		logger.Debugf("Debug filter enabled.")
	}

	// --- Now use the logger ---
	logger.Infof("Starting Tide editor...")
	logger.DebugTagf("config", "Effective Log level set to: %s", cfg.Logger.LogLevel)
	logger.DebugTagf("config", "Effective Log file: %s", cfg.Logger.LogFilePath)
	logger.DebugTagf("config", "Tab Width: %d", cfg.Editor.TabWidth)
	logger.DebugTagf("config", "Scroll Off: %d", cfg.Editor.ScrollOff)
	logger.DebugTagf("config", "System Clipboard: %v", cfg.Editor.SystemClipboard)

	if len(cfg.Logger.DisabledPackages) > 0 {
		logger.DebugTagf("filter", "Disabled Packages: %v", cfg.Logger.DisabledPackages)
	}
	if len(cfg.Logger.EnabledPackages) > 0 {
		logger.DebugTagf("filter", "Enabled Packages: %v", cfg.Logger.EnabledPackages)
	}

	if filePathArg != "" {
		logger.Infof("File path specified: %s", filePathArg)
	} else {
		logger.Infof("No file specified, starting empty.")
	}

	// --- Create and Run App ---
	// Define an app.Config struct if needed
	appConfig := filePathArg // Currently just passing filepath, but could expand to a struct

	tideApp, err := app.NewApp(appConfig)
	if err != nil {
		logger.Fatalf("Error initializing application: %v", err) // Use Fatalf
	}

	if err := tideApp.Run(); err != nil {
		logger.Fatalf("Application exited with error: %v", err) // Use Fatalf
	}

	logger.Infof("Tide editor finished.")
	os.Exit(0) // Explicit exit
}


func printVersion() {
	fmt.Printf("Tide Editor\n")
	fmt.Printf(" Version:   %s\n", Version)
	fmt.Printf(" Commit:    %s\n", Commit)
	fmt.Printf(" Built:     %s\n", BuildDate)
}