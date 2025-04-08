// cmd/tide/main.go
package main

import (
	"flag"
	"os"
	"strings"

	"github.com/bethropolis/tide/internal/app"
	"github.com/bethropolis/tide/internal/logger"
)

var (
	logFilePathFlag  string
	logLevelFlag     string
	filePathArg      string
	enableTagsFlag   string
	disableTagsFlag  string
	enablePkgsFlag   string
	disablePkgsFlag  string
	enableFilesFlag  string
	disableFilesFlag string
)

func main() {
	// --- Define Flags ---
	flag.StringVar(&logLevelFlag, "loglevel", "info", "Log level (debug, info, warn, error)")
	flag.StringVar(&logFilePathFlag, "logfile", "", "Path to write log file (use '-' for stderr)")
	flag.StringVar(&enableTagsFlag, "log-tags", "", "Comma-separated list of tags to enable")
	flag.StringVar(&disableTagsFlag, "log-disable-tags", "", "Comma-separated list of tags to disable")
	flag.StringVar(&enablePkgsFlag, "log-packages", "", "Comma-separated list of packages to enable")
	flag.StringVar(&disablePkgsFlag, "log-disable-packages", "", "Comma-separated list of packages to disable")
	flag.StringVar(&enableFilesFlag, "log-files", "", "Comma-separated list of files to enable")
	flag.StringVar(&disableFilesFlag, "log-disable-files", "", "Comma-separated list of files to disable")

	// Add debug flag
	var debugLogFlag bool
	flag.BoolVar(&debugLogFlag, "debug-log", false, "Enable debug logging for the logger system")

	// --- Parse Flags and Arguments ---
	flag.Parse()

	if flag.NArg() > 0 {
		filePathArg = flag.Arg(0)
	}

	// --- Create Logger Config ---
	logConfig := logger.Config{
		LogLevel:         logLevelFlag,
		LogFilePath:      logFilePathFlag,
		EnabledTags:      splitCommaList(enableTagsFlag),
		DisabledTags:     splitCommaList(disableTagsFlag),
		EnabledPackages:  splitCommaList(enablePkgsFlag),
		DisabledPackages: splitCommaList(disablePkgsFlag),
		EnabledFiles:     splitCommaList(enableFilesFlag),
		DisabledFiles:    splitCommaList(disableFilesFlag),
	}

	// Initialize logger with the config
	logger.Init(logConfig)

	// Enable filter debugging if requested
	logger.EnableFilterDebug(debugLogFlag)

	// enable debug filter if specified
	if logLevelFlag == "filter" {
		logger.EnableFilterDebug(true)
		logger.Debugf("Debug filter enabled.")
	}

	// --- Log Startup Info ---
	logger.Infof("Starting Tide editor...")
	logger.DebugTagf("config", "Effective Log level set to: %s", logConfig.LogLevel)
	logger.DebugTagf("config", "Effective Log file: %s", logConfig.LogFilePath)

	if len(logConfig.DisabledPackages) > 0 {
		logger.DebugTagf("filter", "Disabled Packages: %v", logConfig.DisabledPackages)
	}
	if len(logConfig.EnabledPackages) > 0 {
		logger.DebugTagf("filter", "Enabled Packages: %v", logConfig.EnabledPackages)
	}

	if filePathArg != "" {
		logger.Infof("File path specified: %s", filePathArg)
	} else {
		logger.Infof("No file specified, starting empty.")
	}

	// --- Create and Run App ---
	tideApp, err := app.NewApp(filePathArg)
	if err != nil {
		logger.Errorf("Error initializing application: %v", err)
		os.Exit(1)
	}

	if err := tideApp.Run(); err != nil {
		logger.Errorf("Application exited with error: %v", err)
		os.Exit(1)
	}

	logger.Infof("Tide editor finished.")
	os.Exit(0)
}

// Helper function to split comma-separated list
func splitCommaList(list string) []string {
	if list == "" {
		return nil
	}
	items := strings.Split(list, ",")
	// Trim spaces from each item
	for i, item := range items {
		items[i] = strings.TrimSpace(item)
	}
	return items
}
