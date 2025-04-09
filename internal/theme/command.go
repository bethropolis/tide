package theme

// CommandFunc defines the signature for theme commands
type CommandFunc = func(args []string) error

// ThemeAPI interface for theme operations
type ThemeAPI interface {
	GetTheme() *Theme
	SetTheme(name string) error
	ListThemes() []string
	SetStatusMessage(format string, args ...interface{})
}
