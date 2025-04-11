package autosave

import (
	"sync"
	"time"

	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/plugin"
)

// Ensure AutoSave implements plugin.Plugin
var _ plugin.Plugin = (*AutoSave)(nil)

const (
	// Default configuration values
	defaultEnabled  = false
	defaultInterval = 1 * time.Minute
)

// AutoSave plugin automatically saves modified buffers.
type AutoSave struct {
	api plugin.EditorAPI // To interact with the editor

	// Configuration
	mutex    sync.RWMutex // Protects access to config fields below
	enabled  bool
	interval time.Duration

	// Runtime state
	stopChan chan struct{}  // Signals the saver goroutine to stop
	wg       sync.WaitGroup // Waits for the goroutine to finish
}

// New creates a new instance of the AutoSave plugin.
func New() plugin.Plugin {
	return &AutoSave{
		// Initialize with defaults, config will override in Initialize
		enabled:  defaultEnabled,
		interval: defaultInterval,
	}
}

// Name returns the unique name of the plugin.
func (p *AutoSave) Name() string {
	return "autosave"
}

// Initialize reads configuration and starts the auto-save loop if enabled.
func (p *AutoSave) Initialize(api plugin.EditorAPI) error {
	p.api = api
	pluginName := p.Name()

	logger.Debugf("%s: Initializing...", pluginName)

	// --- Read Configuration ---
	p.mutex.Lock() // Lock for writing config initially

	// Read 'enabled' flag
	if enabledVal, ok := api.GetPluginConfigValue(pluginName, "enabled"); ok {
		if boolVal, isBool := enabledVal.(bool); isBool {
			p.enabled = boolVal
		} else {
			logger.Warnf("%s: Invalid type for 'enabled' config (%T), using default (%v)", pluginName, enabledVal, p.enabled)
		}
	} else {
		logger.Debugf("%s: Config 'enabled' not found, using default (%v)", pluginName, p.enabled)
	}

	// Read 'interval' duration string
	if intervalVal, ok := api.GetPluginConfigValue(pluginName, "interval"); ok {
		if strVal, isStr := intervalVal.(string); isStr {
			parsedInterval, err := time.ParseDuration(strVal)
			if err != nil {
				logger.Warnf("%s: Invalid format for 'interval' config ('%s'): %v. Using default (%v)", pluginName, strVal, err, p.interval)
			} else if parsedInterval <= 0 {
				logger.Warnf("%s: 'interval' config must be positive ('%s'). Using default (%v)", pluginName, strVal, p.interval)
			} else {
				p.interval = parsedInterval
			}
		} else {
			logger.Warnf("%s: Invalid type for 'interval' config (%T), using default (%v)", pluginName, intervalVal, p.interval)
		}
	} else {
		logger.Debugf("%s: Config 'interval' not found, using default (%v)", pluginName, p.interval)
	}

	isEnabled := p.enabled // Read locked value
	interval := p.interval
	p.mutex.Unlock() // Unlock after reading/setting config

	logger.Infof("%s initialized. Enabled: %v, Interval: %v", pluginName, isEnabled, interval)

	// --- Start Saver Goroutine ---
	if isEnabled {
		p.stopChan = make(chan struct{})
		p.wg.Add(1) // Increment wait group counter
		go p.saverLoop(interval)
		logger.Debugf("%s: Saver goroutine started.", pluginName)
	}

	return nil
}

// Shutdown signals the saver goroutine to stop and waits for it.
func (p *AutoSave) Shutdown() error {
	p.mutex.RLock()
	isEnabled := p.enabled // Check if it was ever enabled
	p.mutex.RUnlock()

	if isEnabled && p.stopChan != nil {
		logger.Debugf("%s: Shutting down...", p.Name())
		close(p.stopChan) // Signal the goroutine to stop
		p.wg.Wait()       // Wait for the goroutine to finish
		logger.Debugf("%s: Saver goroutine stopped.", p.Name())
	}
	return nil
}

// saverLoop is the main loop for the auto-save functionality.
func (p *AutoSave) saverLoop(interval time.Duration) {
	defer p.wg.Done() // Decrement wait group counter when goroutine exits

	// Use a ticker for periodic checks
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Debugf("%s: Entering saver loop with interval %v.", p.Name(), interval)

	for {
		select {
		case <-ticker.C:
			// Timer ticked, check if save is needed
			p.saveIfModified()
		case <-p.stopChan:
			// Shutdown signal received
			logger.Debugf("%s: Received stop signal, exiting saver loop.", p.Name())
			return
		}
	}
}

// saveIfModified checks if the buffer is modified and saves it.
func (p *AutoSave) saveIfModified() {
	// No need to lock mutex here as config is read-only after init for now
	// If live config reload was added, we would need RLock here.

	if !p.enabled { // Double-check in case it was disabled live (future feature)
		return
	}

	if p.api == nil {
		logger.Errorf("%s: API is nil in saveIfModified!", p.Name())
		return
	}

	// Check if the current buffer is modified
	isModified := p.api.IsBufferModified()

	if isModified {
		filePath := p.api.GetBufferFilePath()
		if filePath == "" {
			logger.Debugf("%s: Buffer is modified but has no name, skipping auto-save.", p.Name())
			// Optionally: Notify user they need to save manually first?
			// p.api.SetStatusMessage("%s: Please save the new buffer first", p.Name())
			return
		}

		logger.Infof("%s: Auto-saving modified buffer: %s", p.Name(), filePath)
		err := p.api.SaveBuffer() // Save using the buffer's current path
		if err != nil {
			logger.Errorf("%s: Auto-save failed for '%s': %v", p.Name(), filePath, err)
			// Optionally notify the user via status bar, but could be noisy
			// p.api.SetStatusMessage("%s: Auto-save failed: %v", p.Name(), err)
		} else {
			// Optionally notify user of success? Might be too verbose.
			// p.api.SetStatusMessage("%s: Auto-saved '%s'", p.Name(), filePath)
			logger.Debugf("%s: Auto-save successful for '%s'", p.Name(), filePath)
		}
	} else {
		logger.Debugf("%s: Buffer not modified, skipping auto-save.", p.Name())
	}
}
