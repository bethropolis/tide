package app

import (
	"fmt" // For error wrapping

	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/plugin"

	// Import desired plugin packages here
	"github.com/bethropolis/tide/plugins/autosave"
	"github.com/bethropolis/tide/plugins/wordcount"
	// Import other plugins as they are created
	// "github.com/bethropolis/tide/plugins/anotherplugin"
)

// registerPlugins initializes and registers all known plugins with the manager.
func registerPlugins(pm *plugin.Manager) error {
	if pm == nil {
		return fmt.Errorf("plugin manager is nil")
	}

	// List of plugin constructors
	// Adding a new plugin means adding its constructor here.
	pluginConstructors := []func() plugin.Plugin{
		wordcount.New,
		autosave.New,
		// anotherplugin.New,
	}

	var finalErr error
	for _, newPlugin := range pluginConstructors {
		p := newPlugin()
		pluginName := p.Name() // Get name for logging

		logger.Debugf("Registering plugin: %s", pluginName)
		err := pm.Register(p)
		if err != nil {
			// Log the error but continue registering others
			wrappedErr := fmt.Errorf("failed to register plugin '%s': %w", pluginName, err)
			logger.Errorf(wrappedErr.Error())
			// Collect errors if needed, or just log and continue
			if finalErr == nil {
				finalErr = wrappedErr // Store the first error encountered
			}
		}
	}

	return finalErr // Return the first error encountered, or nil
}
