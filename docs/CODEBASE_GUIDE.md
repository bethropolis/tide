# Tide Editor: Codebase Guide

Welcome to the Tide editor project! This guide provides an overview of the codebase structure, core concepts, and how different components interact. Whether you're looking to contribute or just understand how it works, this document should be your starting point.

**Core Philosophy:**

*   **Plugin-First:** The editor core aims to be minimal, with most features implemented as plugins (even built-in ones eventually).
*   **Modularity:** Functionality is separated into distinct packages with clear responsibilities and interfaces.
*   **Clean Code:** Emphasis on readability, maintainability, and standard Go practices.

## High-Level Architecture

Tide operates on a modular event-driven architecture. Here's a conceptual flow for user input:

1.  **Input Event (TUI):** The `tui` package captures raw keyboard/mouse events from the terminal using the `tcell` library.
2.  **Event Loop (App):** The main application (`app`) receives the raw event.
3.  **Mode Handling (ModeHandler):** The raw key event is passed to the `modehandler`. Based on the current mode (e.g., Normal, Command), it interprets the event.
4.  **Input Processing (Input):** The `modehandler` uses the `input` package to translate the raw key into a defined `Action` (e.g., `ActionMoveUp`, `ActionInsertRune`, `ActionEnterCommandMode`).
5.  **Action Execution (ModeHandler/Editor):**
    *   If it's a command input action (in Command Mode), `modehandler` updates its internal command buffer.
    *   If it's an editor action (in Normal Mode), `modehandler` calls methods on the `core.Editor` (e.g., `editor.MoveCursor`, `editor.InsertRune`).
    *   If it's a command execution (Enter in Command Mode), `modehandler` looks up and executes the registered command function.
6.  **Editor Logic (Core):** The `core.Editor` updates its state (cursor position, viewport) and interacts with the `buffer`.
7.  **Buffer Operation (Buffer):** If text is modified, the `core.Editor` calls methods on the active `buffer.Buffer` implementation (e.g., `slice_buffer`) to insert or delete text.
8.  **Event Dispatch (Event/App):** After actions are performed, the `app` (often triggered by `core.Editor` methods or `modehandler`) dispatches events (e.g., `TypeCursorMoved`, `TypeBufferModified`) via the `event.Manager`.
9.  **Event Handling (App/Plugins/StatusBar):** Components subscribed to events react. For example:
    *   `app` updates the `statusbar` component with new cursor/file info.
    *   Plugins might react to buffer changes or key presses.
10. **Redraw Request (App):** Any action that changes the visible state requests a redraw via a channel.
11. **Drawing (App/TUI/StatusBar):** The main loop in `app` receives the redraw request, updates component states (like status bar info), and calls drawing functions:
    *   `tui.DrawBuffer` renders the text content based on `core.Editor` viewport.
    *   `statusbar.Draw` renders the status line based on its state.
    *   `tui.DrawCursor` places the cursor.
    *   `tui.Show` updates the physical terminal screen.

<!-- TODO: Add a simple ASCII flow diagram here -->

## Workspace Overview

```
tide/
├── cmd/tide/         # Main application entry point (very minimal)
│   └── main.go
├── internal/         # Core internal packages (not meant for external use)
│   ├── app/          # Application orchestrator, lifecycle, API implementation
│   ├── buffer/       # Text buffer abstraction and implementation
│   ├── core/         # Core editor logic and state (cursor, viewport)
│   ├── event/        # Event bus system (types, manager)
│   ├── input/        # Raw input to Action translation (keymaps)
│   ├── modehandler/  # Input mode logic, command execution
│   ├── plugin/       # Plugin interfaces (Plugin, EditorAPI), manager
│   ├── statusbar/    # Status bar UI component logic and drawing
│   ├── tui/          # Terminal UI abstraction (tcell wrapper, drawing utils)
│   ├── types/        # Shared basic types (Position) to avoid cycles
│   └── util/         # Common utility functions (currently empty)
├── plugins/          # Location for specific built-in plugin implementations
│   └── wordcount/    # Example word count plugin
├── pkg/              # Reserved for potentially exportable libraries (currently empty)
├── go.mod            # Go module definition
├── go.sum            # Dependency checksums
├── guide.md          # Initial planning guide (this file replaces/augments it)
├── CODEBASE_GUIDE.md # This file!
└── README.md         # Project README
```

## Deep Dive: `internal` Packages

### `internal/app`

*   **Purpose:** Orchestrates the entire application. Connects all the different components.
*   **Key Types:**
    *   `App`: The main application struct. Holds instances of TUI, Editor, ModeHandler, StatusBar, EventManager, PluginManager, etc.
    *   `appEditorAPI`: The concrete implementation of the `plugin.EditorAPI` interface, providing controlled access for plugins to the App's components.
*   **Responsibilities:**
    *   Initialization (`NewApp`): Creates and wires up all core components. Registers built-in plugins. Initializes the editor API. Sets up event subscriptions between core components.
    *   Lifecycle Management (`Run`): Starts the event loop goroutine, manages the main drawing loop, handles the quit signal, and ensures cleanup (TUI close, plugin shutdown).
    *   Event Loop (`eventLoop`): Receives raw events from TUI and delegates key events to `ModeHandler`. Triggers redraws.
    *   Drawing Trigger (`drawEditor`): Called on redraw requests. Updates component states (like status bar info) and calls the specific drawing functions in `tui` and `statusbar`.
    *   Core Event Handling: Contains handler methods (e.g., `handleCursorMovedForStatus`) that react to events dispatched by the `event.Manager` to update core UI components like the status bar.

### `internal/buffer`

*   **Purpose:** Provides an abstraction for text storage and manipulation.
*   **Key Types:**
    *   `Buffer` (interface): Defines the contract for text buffers (`Load`, `Save`, `Lines`, `Line`, `Insert`, `Delete`, `Bytes`, `IsModified`, `FilePath`).
    *   `SliceBuffer` (struct): A concrete implementation using `[][]byte`.
*   **Responsibilities:**
    *   Loading text from and saving text to files.
    *   Providing access to lines and the entire content.
    *   Implementing the core text insertion and deletion logic.
    *   Tracking the modified status.

### `internal/core`

*   **Purpose:** Represents the logical state of the editor, independent of the UI.
*   **Key Types:**
    *   `Editor`: Holds the current `buffer.Buffer`, cursor position (`types.Position`), viewport position (`ViewportX`, `ViewportY`), scroll-off setting, and cached view dimensions.
*   **Responsibilities:**
    *   Managing cursor position and movement logic (`MoveCursor`, `Home`, `End`, `PageMove`). Includes clamping cursor to valid buffer locations and handling line wrapping for left/right movement.
    *   Managing the viewport state (`ScrollToCursor`, `SetViewSize`) to keep the cursor visible, incorporating scroll-off margins.
    *   Providing high-level editing actions (`InsertRune`, `InsertNewLine`, `DeleteBackward`, `DeleteForward`) which typically delegate text manipulation to the `Buffer`.
    *   Initiating buffer save operations (`SaveBuffer`).

### `internal/event`

*   **Purpose:** Decouples components by providing a publish/subscribe event system.
*   **Key Types:**
    *   `Type` (enum): Defines different kinds of events (e.g., `TypeBufferModified`).
    *   `Event` (struct): The data structure passed on the bus, containing `Type` and `Data`.
    *   `Data Structs` (e.g., `CursorMovedData`): Specific payloads for events.
    *   `Manager`: Handles subscriptions (`Subscribe`) and dispatching (`Dispatch`).
*   **Responsibilities:**
    *   Allows components to announce state changes (`Dispatch`) without knowing who is listening.
    *   Allows components to react to state changes (`Subscribe`) without needing direct references to the announcer.

### `internal/input`

*   **Purpose:** Translates low-level terminal key events into semantic editor actions.
*   **Key Types:**
    *   `Action` (enum): Defines semantic actions (e.g., `ActionMoveUp`, `ActionSave`).
    *   `ActionEvent` (struct): Bundles an `Action` and any associated data (like a `Rune`).
    *   `InputProcessor`: Contains keymaps (`Keymap`, `RuneKeymap`, `ModKeymap`).
*   **Responsibilities:**
    *   Defining default keybindings (`loadDefaultBindings`).
    *   Processing `tcell.EventKey` events (`ProcessEvent`) and mapping them to `ActionEvent` based on the keymaps. It does *not* know about the editor's current mode.

### `internal/modehandler`

*   **Purpose:** Manages the editor's input modes (Normal, Command) and handles input/actions accordingly. Executes commands.
*   **Key Types:**
    *   `InputMode` (enum): Defines `ModeNormal`, `ModeCommand`.
    *   `ModeHandler`: Holds references to core components (Editor, StatusBar, EventManager, etc.). Manages `currentMode`, `cmdBuffer`, the command registry (`commands`), and `forceQuitPending` state.
*   **Responsibilities:**
    *   Receiving key events (`HandleKeyEvent`) from the `app.eventLoop`.
    *   Interpreting `ActionEvent`s based on the `currentMode`.
    *   Switching between modes (e.g., on `:` or `ESC`).
    *   In `ModeNormal`, delegating editor actions to `core.Editor`.
    *   In `ModeCommand`, managing the `cmdBuffer` input and updating the status bar display.
    *   Executing commands (`executeCommand`) by looking up the name in its `commands` map and calling the registered function.
    *   Registering commands (`RegisterCommand`) provided by plugins (called via the `EditorAPI`).
    *   Signaling the application to quit via the `quitSignal` channel.

### `internal/plugin`

*   **Purpose:** Defines the interfaces and manager for the plugin system.
*   **Key Types:**
    *   `Plugin` (interface): The contract plugins must implement (`Name`, `Initialize`, `Shutdown`).
    *   `EditorAPI` (interface): The contract the editor provides *to* plugins, defining how they can safely interact with the core (get buffer info, move cursor, dispatch events, register commands, etc.).
    *   `Manager`: Registers, initializes (passing the `EditorAPI`), and shuts down plugins.
*   **Responsibilities:**
    *   Defining the plugin architecture.
    *   Managing the lifecycle of plugins.

### `internal/statusbar`

*   **Purpose:** Encapsulates the state, logic, and drawing for the status bar UI component.
*   **Key Types:**
    *   `StatusBar`: Holds configuration (`Config`), current state (file info, cursor pos, modified status, temporary message), and provides methods to update state (`SetFileInfo`, `SetCursorInfo`, `SetTemporaryMessage`).
    *   `Config`: Defines styles and behavior (e.g., message timeout).
*   **Responsibilities:**
    *   Managing its internal state based on updates from the `app`.
    *   Handling the display and timeout of temporary messages.
    *   Rendering itself onto the `tcell.Screen` (`Draw`).

### `internal/tui`

*   **Purpose:** Provides a higher-level abstraction over the `tcell` library for terminal interaction and drawing primitives.
*   **Key Types:**
    *   `TUI`: Wraps the `tcell.Screen` object.
*   **Responsibilities:**
    *   Initializing and closing the `tcell` screen (`New`, `Close`).
    *   Providing basic screen operations (`Clear`, `Show`, `Size`, `PollEvent`).
    *   (`drawing.go`): Contains specific functions to render parts of the editor UI (`DrawBuffer`, `DrawCursor`) by interacting with the `tcell.Screen`.

### `internal/types`

*   **Purpose:** Holds basic data structures shared by multiple packages to prevent import cycles.
*   **Key Types:**
    *   `Position`: Represents a line/column position.
*   **Responsibilities:**
    *   Defining fundamental, shared data structures.

### `internal/util`

*   **Purpose:** A place for common, general-purpose utility functions that don't belong to a specific domain package. (Currently empty).

## Plugin System (`plugins/...`)

*   Plugins reside in the top-level `plugins/` directory (e.g., `plugins/wordcount`).
*   Each plugin implements the `plugin.Plugin` interface.
*   Plugins are instantiated and registered with the `plugin.Manager` in `app.NewApp`.
*   During `pluginManager.InitializePlugins`, each plugin's `Initialize` method is called, receiving the `plugin.EditorAPI`.
*   Plugins use the `EditorAPI` to:
    *   Subscribe to events (`api.SubscribeEvent(...)`).
    *   Register commands (`api.RegisterCommand(...)`).
    *   Access editor state (e.g., `api.GetCursor()`, `api.GetBufferBytes()`).
    *   Modify editor state (carefully!) (e.g., `api.InsertText(...)`).
    *   Display information (`api.SetStatusMessage(...)`).
*   The `wordcount` plugin serves as a simple example.

## How to Contribute

1.  **Understand the Architecture:** Read this guide thoroughly.
2.  **Set up:** Clone the repository, ensure you have Go installed (`go version`). Run `go build ./cmd/tide/` to build.
3.  **Run:** Execute `./tide [optional_file_path]`.
4.  **Identify an Area:** Look at TODOs in the code, address FIXMEs, implement features from the original `guide.md`, or pick an existing package to improve.
5.  **Follow Patterns:** Adhere to the existing modular structure. If adding a major feature, consider if it should be a new package or a plugin.
6.  **Testing:** (Future) Add unit tests for buffer logic, mode handling, etc. Add integration tests.
7.  **Commit Messages:** Write clear and concise commit messages.
8.  **Pull Requests:** Submit PRs for review.

## Future Directions

*   Implement more editing features (selection, yank/paste, find/replace).
*   Implement more built-in plugins (syntax highlighting, file tree, Git integration).
*   Load configuration from files (keybindings, themes, settings).
*   Add more robust error handling and user feedback.
*   Implement proper Unicode width handling (`rivo/uniseg`).
*   Add comprehensive tests.

This guide should provide a solid foundation for understanding the Tide editor's codebase. Happy coding!
