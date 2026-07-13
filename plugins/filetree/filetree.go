// Package filetree provides a file-picker plugin that uses the UI Picker
// to let users browse and open files from the current directory.
package filetree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/plugin"
	"github.com/bethropolis/tide/internal/tui"
)

// Plugin implements plugin.Plugin.
type Plugin struct {
	api plugin.EditorAPI
}

// New returns a constructor for the filetree plugin.
func New() plugin.Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string {
	return "filetree"
}

func (p *Plugin) Initialize(api plugin.EditorAPI) error {
	p.api = api

	err := api.RegisterCommand("files", p.listFiles)
	if err != nil {
		logger.Warnf("filetree: failed to register :files command: %v", err)
	}

	err = api.RegisterCommand("pick", p.pickFile)
	if err != nil {
		logger.Warnf("filetree: failed to register :pick command: %v", err)
	}

	return nil
}

func (p *Plugin) Shutdown() error {
	return nil
}

// listFiles shows the file count in the status bar.
func (p *Plugin) listFiles(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cannot read directory %q: %w", dir, err)
	}

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		files = append(files, e.Name())
	}

	p.api.SetStatusMessage("%d files in %s", len(files), dir)
	return nil
}

// pickFile opens a Picker overlay listing files in the given directory (default ".").
func (p *Plugin) pickFile(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cannot read directory %q: %w", dir, err)
	}

	items := make([]tui.PickerItem, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		desc := "file"
		if e.IsDir() {
			desc = "directory"
		}
		items = append(items, tui.PickerItem{
			Label:       e.Name(),
			Description: desc,
			Value:       filepath.Join(dir, e.Name()),
		})
	}

	if len(items) == 0 {
		p.api.SetStatusMessage("No files found in %s", dir)
		return nil
	}

	p.api.ShowPicker("Files", items, func(val string) {
		p.api.OpenFile(val)
	}, nil)

	return nil
}
