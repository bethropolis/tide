package lua

import (
	"os"
	"path/filepath"

	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/plugin"
)

// LoadLuaPlugins loads all .lua files from the specified directory and registers them.
func LoadLuaPlugins(pm *plugin.Manager, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debugf("Lua plugin directory '%s' does not exist, skipping", dir)
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".lua" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		p, err := NewLuaPlugin(path)
		if err != nil {
			logger.Errorf("Failed to load lua plugin '%s': %v", path, err)
			continue
		}

		if err := pm.Register(p); err != nil {
			logger.Errorf("Failed to register lua plugin '%s': %v", path, err)
		} else {
			logger.Infof("Loaded Lua plugin: %s", p.Name())
		}
	}

	return nil
}
