// internal/plugin/manager.go
package plugin

import (
	"fmt"
	"log"
	"sync"

	// "github.com/bethropolis/tide/internal/event"
)

// Manager handles the registration, initialization, and lifecycle of plugins.
type Manager struct {
	mu      sync.RWMutex
	plugins map[string]Plugin // Store loaded plugins by name
	api     EditorAPI         // The API instance passed to plugins during init
}

// NewManager creates a new plugin manager.
func NewManager() *Manager {
	return &Manager{
		plugins: make(map[string]Plugin),
	}
}

// Register adds a plugin instance to the manager.
// This should be called before InitializePlugins.
func (m *Manager) Register(plugin Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := plugin.Name()
	if name == "" {
		return fmt.Errorf("plugin registration failed: plugin name cannot be empty")
	}
	if _, exists := m.plugins[name]; exists {
		return fmt.Errorf("plugin registration failed: plugin named '%s' already registered", name)
	}

	m.plugins[name] = plugin
	log.Printf("Plugin Manager: Registered plugin '%s'", name)
	return nil
}

// InitializePlugins iterates through registered plugins and calls their Init method.
// It requires the EditorAPI instance to be provided.
func (m *Manager) InitializePlugins(api EditorAPI) {
	m.mu.Lock() // Lock while setting API and iterating initial map
	m.api = api   // Store the API for potential later use? Might not be needed.
	pluginsToInit := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		pluginsToInit = append(pluginsToInit, p)
	}
	m.mu.Unlock() // Unlock before calling plugin Init methods

	log.Printf("Plugin Manager: Initializing %d plugins...", len(pluginsToInit))
	for _, plugin := range pluginsToInit {
		log.Printf("Plugin Manager: Initializing plugin '%s'...", plugin.Name())
		err := plugin.Initialize(api) // Pass the API
		if err != nil {
			// Log error but continue initializing other plugins? Or halt?
			// Let's log and continue for robustness.
			log.Printf("Plugin Manager: ERROR initializing plugin '%s': %v", plugin.Name(), err)
		} else {
			log.Printf("Plugin Manager: Successfully initialized plugin '%s'", plugin.Name())
		}
	}
}

// ShutdownPlugins calls Shutdown on all registered plugins.
func (m *Manager) ShutdownPlugins() {
	m.mu.RLock() // Read lock to get the list of plugins
	pluginsToShutdown := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		pluginsToShutdown = append(pluginsToShutdown, p)
	}
	m.mu.RUnlock() // Unlock before calling Shutdown

	log.Printf("Plugin Manager: Shutting down %d plugins...", len(pluginsToShutdown))
	for _, plugin := range pluginsToShutdown {
		log.Printf("Plugin Manager: Shutting down plugin '%s'...", plugin.Name())
		err := plugin.Shutdown()
		if err != nil {
			log.Printf("Plugin Manager: ERROR shutting down plugin '%s': %v", plugin.Name(), err)
		}
	}
}

// DispatchEventToPlugins sends an event to all plugins that might handle it.
// Note: If plugins subscribe via the API, this direct dispatch might become less necessary,
// as the main event manager would handle delivery. This could be used for specific
// plugin-only event types or as an alternative dispatch mechanism.
// Let's keep it simple for now and rely on plugins subscribing via the API.
/*
func (m *Manager) DispatchEventToPlugins(e event.Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.plugins {
		// Check if plugin implements an optional HandleEvent method
		if handler, ok := p.(interface{ HandleEvent(event.Event) bool }); ok {
			consumed := handler.HandleEvent(e)
			if consumed {
				break // Allow event consumption
			}
		}
	}
}
*/

// GetPlugin returns a registered plugin by name (e.g., for inter-plugin communication). Use cautiously.
func (m *Manager) GetPlugin(name string) (Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, exists := m.plugins[name]
	return p, exists
}