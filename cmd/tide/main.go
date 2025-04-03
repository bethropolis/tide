// cmd/tide/main.go
package main

import (
	"log"
	"os"

	"github.com/bethropolis/tide/internal/app" // Import the new app package
)

func main() {
	// 1. Argument Parsing
	filePath := ""
	if len(os.Args) > 1 {
		filePath = os.Args[1]
		log.Printf("File path specified: %s", filePath)
	} else {
		log.Println("No file specified, starting empty.")
	}

	// 2. Create the Application instance
	tideApp, err := app.NewApp(filePath) // App handles internal setup
	if err != nil {
		log.Fatalf("Error initializing application: %v", err)
		// Note: TUI cleanup might not happen here if NewApp fails early.
		// Consider if NewApp needs more robust resource cleanup on partial failure.
		os.Exit(1)
	}

	// 3. Run the Application
	log.Println("Starting Tide editor...")
	if err := tideApp.Run(); err != nil {
		// App.Run() blocks until exit. Log errors returned from Run.
		log.Fatalf("Application exited with error: %v", err)
		os.Exit(1)
	}

	// Run finishes normally (e.g., user quit)
	log.Println("Tide editor finished.")
	os.Exit(0)
}