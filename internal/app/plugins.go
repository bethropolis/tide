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
	"github.com/bethropolis/tide/internal/plugin/lua"
	"os"
	"path/filepath"
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

	// Load Lua plugins from common directories
	// 1. Current directory ./plugins/lua
	if err := lua.LoadLuaPlugins(pm, "plugins/lua"); err != nil {
		logger.Debugf("Error loading local lua plugins: %v", err)
	}

	// 2. User config directory ~/.config/tide/plugins/lua
	if configDir, err := os.UserConfigDir(); err == nil {
		userPluginDir := filepath.Join(configDir, "tide", "plugins", "lua")
		if err := lua.LoadLuaPlugins(pm, userPluginDir); err != nil {
			logger.Debugf("Error loading user lua plugins: %v", err)
		}
	}

	return finalErr // Return the first error encountered, or nil
}
