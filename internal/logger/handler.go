package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const tagKey = "tag" // The slog attribute key used for filtering tags

// filteringHandler wraps a base slog.Handler to add custom filtering.
type filteringHandler struct {
	baseHandler slog.Handler
	cfg         *Config // Reference to processed config
}

// newFilteringHandler creates a handler with filtering capabilities.
func newFilteringHandler(base slog.Handler, cfg *Config) *filteringHandler {
	return &filteringHandler{
		baseHandler: base,
		cfg:         cfg,
	}
}

// Enabled checks if the level is enabled by the base handler.
func (h *filteringHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.baseHandler.Enabled(ctx, level)
}

// Helper function for set lookup
func foundInSet(set map[string]struct{}, key string) bool {
	if set == nil {
		return false
	}
	_, found := set[key]
	return found
}

// Handle applies filtering logic before passing the record to the base handler.
func (h *filteringHandler) Handle(ctx context.Context, r slog.Record) error {
	// Skip filtering if no config is available
	if h.cfg == nil {
		return h.baseHandler.Handle(ctx, r)
	}

	// Debug logging (controlled by debugFilter flag)
	if debugFilter {
		fmt.Fprintf(os.Stderr, "[FILTER] Message: Level=%s, Msg=%s\n", r.Level, r.Message)
	}

	// --- Extract Source Information ---
	var pkg, file string
	var sourceFound bool

	// First try to get source info from the Source attribute (most reliable)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == slog.SourceKey {
			if source, ok := a.Value.Any().(*slog.Source); ok && source != nil {
				file = filepath.Base(source.File)
				if file != "" {
					dirPath := filepath.Dir(source.File)
					pkg = filepath.Base(dirPath)
					sourceFound = true
				}
				return false // Stop iteration once we find source
			}
		}
		return true // Continue iteration
	})

	// If Source attribute not found, try using the PC
	if !sourceFound && r.PC != 0 {
		frames := runtime.CallersFrames([]uintptr{r.PC})
		frame, more := frames.Next()
		if more {
			file = filepath.Base(frame.File)
			dirPath := filepath.Dir(frame.File)
			pkg = filepath.Base(dirPath)
			sourceFound = true
		}
	}

	// Debug source information
	if debugFilter {
		if sourceFound {
			fmt.Fprintf(os.Stderr, "[FILTER] Source found: Package=%s, File=%s\n", pkg, file)
		} else {
			fmt.Fprintln(os.Stderr, "[FILTER] No source information found")
		}
	}

	// --- Apply Package Filtering ---
	if sourceFound && pkg != "" {
		pkgLower := strings.ToLower(pkg)

		// Check if this package should be disabled
		if h.cfg.disabledPackagesSet != nil {
			if _, found := h.cfg.disabledPackagesSet[pkgLower]; found {
				if debugFilter {
					fmt.Fprintf(os.Stderr, "[FILTER] FILTERED OUT: Message from disabled package '%s'\n", pkg)
				}
				return nil // Drop the message
			}
		}

		// Check if we have an enabled packages list and this one isn't on it
		if h.cfg.enabledPackagesSet != nil {
			if _, found := h.cfg.enabledPackagesSet[pkgLower]; !found {
				if debugFilter {
					fmt.Fprintf(os.Stderr, "[FILTER] FILTERED OUT: Package '%s' not in enabled list\n", pkg)
				}
				return nil // Drop the message
			}
		}
	}

	// --- Apply File Filtering ---
	if sourceFound && file != "" {
		fileLower := strings.ToLower(file)

		// Check if this file should be disabled
		if h.cfg.disabledFilesSet != nil {
			if _, found := h.cfg.disabledFilesSet[fileLower]; found {
				if debugFilter {
					fmt.Fprintf(os.Stderr, "[FILTER] FILTERED OUT: Message from disabled file '%s'\n", file)
				}
				return nil // Drop the message
			}
		}

		// Check if we have an enabled files list and this one isn't on it
		if h.cfg.enabledFilesSet != nil {
			if _, found := h.cfg.enabledFilesSet[fileLower]; !found {
				if debugFilter {
					fmt.Fprintf(os.Stderr, "[FILTER] FILTERED OUT: File '%s' not in enabled list\n", file)
				}
				return nil // Drop the message
			}
		}
	}

	// --- Apply Tag Filtering ---
	var tagValue string
	var tagFound bool

	r.Attrs(func(a slog.Attr) bool {
		if a.Key == tagKey {
			tagValue = strings.ToLower(a.Value.String())
			tagFound = true
			return false // Stop iteration
		}
		return true // Continue iteration
	})

	if debugFilter && tagFound {
		fmt.Fprintf(os.Stderr, "[FILTER] Message has tag: %s\n", tagValue)
	}

	if tagFound {
		// Check if this tag should be disabled
		if h.cfg.disabledTagsSet != nil {
			if _, found := h.cfg.disabledTagsSet[tagValue]; found {
				if debugFilter {
					fmt.Fprintf(os.Stderr, "[FILTER] FILTERED OUT: Message with disabled tag '%s'\n", tagValue)
				}
				return nil // Drop the message
			}
		}

		// Check if we have an enabled tags list and this one isn't on it
		if h.cfg.enabledTagsSet != nil {
			if _, found := h.cfg.enabledTagsSet[tagValue]; !found {
				if debugFilter {
					fmt.Fprintf(os.Stderr, "[FILTER] FILTERED OUT: Tag '%s' not in enabled list\n", tagValue)
				}
				return nil // Drop the message
			}
		}
	} else if h.cfg.enabledTagsSet != nil {
		// If we're filtering for specific tags but this message has none, filter it out
		if debugFilter {
			fmt.Fprintln(os.Stderr, "[FILTER] FILTERED OUT: Message has no tag but specific tags are enabled")
		}
		return nil // Drop the message
	}

	// If we get here, the message passed all filters
	if debugFilter {
		fmt.Fprintln(os.Stderr, "[FILTER] PASSED: Message passed all filters")
	}

	// Pass the record to the base handler
	return h.baseHandler.Handle(ctx, r)
}

// WithAttrs returns a new handler with attributes added.
func (h *filteringHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return newFilteringHandler(h.baseHandler.WithAttrs(attrs), h.cfg)
}

// WithGroup returns a new handler with a group added.
func (h *filteringHandler) WithGroup(name string) slog.Handler {
	return newFilteringHandler(h.baseHandler.WithGroup(name), h.cfg)
}
