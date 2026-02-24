package lua

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sync"

	"github.com/bethropolis/tide/internal/event"
	"github.com/bethropolis/tide/internal/logger"
	"github.com/bethropolis/tide/internal/plugin"
	"github.com/bethropolis/tide/internal/types"
	lua "github.com/yuin/gopher-lua"
)

// LuaPlugin wraps a Lua script to implement the plugin.Plugin interface.
type LuaPlugin struct {
	name   string
	script string
	api    plugin.EditorAPI
	L      *lua.LState
	mu     sync.Mutex
}

// NewLuaPlugin creates a new Lua plugin from a script path.
func NewLuaPlugin(path string) (*LuaPlugin, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read lua script: %w", err)
	}

	// Use filename without extension as plugin name for now
	name := filepath.Base(path)
	name = name[:len(name)-len(filepath.Ext(name))]

	return &LuaPlugin{
		name:   name,
		script: string(content),
	}, nil
}

func (p *LuaPlugin) Name() string {
	return "lua_" + p.name
}

func (p *LuaPlugin) Initialize(api plugin.EditorAPI) error {
	p.api = api
	p.L = lua.NewState()

	// Register the Tide API in Lua
	p.registerAPI()

	// Execute the script
	p.mu.Lock()
	err := p.L.DoString(p.script)
	p.mu.Unlock()
	if err != nil {
		return fmt.Errorf("lua execution error: %w", err)
	}

	return nil
}

func (p *LuaPlugin) Shutdown() error {
	if p.L != nil {
		p.L.Close()
	}
	return nil
}

func (p *LuaPlugin) registerAPI() {
	// Create the `tide` table
	tideTable := p.L.NewTable()

	// tide.set_status_message(msg)
	p.L.SetField(tideTable, "set_status_message", p.L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		p.api.SetStatusMessage(msg)
		return 0
	}))

	// tide.register_command(name, callback)
	p.L.SetField(tideTable, "register_command", p.L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		callback := L.CheckFunction(2)

		err := p.api.RegisterCommand(name, func(args []string) error {
			p.mu.Lock()
			defer p.mu.Unlock()

			// Call the Lua function
			p.L.Push(callback)

			// Pass arguments as a Lua table
			argTable := p.L.NewTable()
			for i, arg := range args {
				p.L.RawSetInt(argTable, i+1, lua.LString(arg))
			}
			p.L.Push(argTable)

			if err := p.L.PCall(1, 0, nil); err != nil {
				logger.Errorf("Lua command '%s' error: %v", name, err)
				p.api.SetStatusMessage("Lua error: %v", err)
				return fmt.Errorf("lua command error: %w", err)
			}
			return nil
		})

		if err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}

		L.Push(lua.LNil)
		return 1
	}))

	// tide.get_cursor() -> line, col
	p.L.SetField(tideTable, "get_cursor", p.L.NewFunction(func(L *lua.LState) int {
		pos := p.api.GetCursor()
		L.Push(lua.LNumber(pos.Line))
		L.Push(lua.LNumber(pos.Col))
		return 2
	}))

	// tide.set_cursor(line, col)
	p.L.SetField(tideTable, "set_cursor", p.L.NewFunction(func(L *lua.LState) int {
		line := L.CheckInt(1)
		col := L.CheckInt(2)
		p.api.SetCursor(types.Position{Line: line, Col: col})
		return 0
	}))

	// tide.get_buffer_lines(start_line, end_line)
	p.L.SetField(tideTable, "get_buffer_lines", p.L.NewFunction(func(L *lua.LState) int {
		startLine := L.CheckInt(1)
		endLine := L.CheckInt(2)
		lines, err := p.api.GetBufferLines(startLine, endLine)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		luaTable := L.NewTable()
		for i, line := range lines {
			L.RawSetInt(luaTable, i+1, lua.LString(string(line)))
		}
		L.Push(luaTable)
		return 1
	}))

	// tide.insert_text(line, col, text)
	p.L.SetField(tideTable, "insert_text", p.L.NewFunction(func(L *lua.LState) int {
		line := L.CheckInt(1)
		col := L.CheckInt(2)
		text := L.CheckString(3)
		err := p.api.InsertText(types.Position{Line: line, Col: col}, []byte(text))
		if err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))

	// tide.delete_range(start_line, start_col, end_line, end_col)
	p.L.SetField(tideTable, "delete_range", p.L.NewFunction(func(L *lua.LState) int {
		sl := L.CheckInt(1)
		sc := L.CheckInt(2)
		el := L.CheckInt(3)
		ec := L.CheckInt(4)
		err := p.api.DeleteRange(types.Position{Line: sl, Col: sc}, types.Position{Line: el, Col: ec})
		if err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))

	// tide.get_buffer_file_path()
	p.L.SetField(tideTable, "get_buffer_file_path", p.L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(p.api.GetBufferFilePath()))
		return 1
	}))

	// tide.open_file(filepath)
	p.L.SetField(tideTable, "open_file", p.L.NewFunction(func(L *lua.LState) int {
		filepath := L.CheckString(1)
		p.api.OpenFile(filepath)
		return 0
	}))

	// tide.next_buffer()
	p.L.SetField(tideTable, "next_buffer", p.L.NewFunction(func(L *lua.LState) int {
		p.api.NextBuffer()
		return 0
	}))

	// tide.prev_buffer()
	p.L.SetField(tideTable, "prev_buffer", p.L.NewFunction(func(L *lua.LState) int {
		p.api.PrevBuffer()
		return 0
	}))

	// tide.close_buffer()
	p.L.SetField(tideTable, "close_buffer", p.L.NewFunction(func(L *lua.LState) int {
		err := p.api.CloseBuffer()
		if err != nil {
			L.Push(lua.LString(err.Error()))
			return 1
		}
		L.Push(lua.LNil)
		return 1
	}))

	// tide.force_close_buffer()
	p.L.SetField(tideTable, "force_close_buffer", p.L.NewFunction(func(L *lua.LState) int {
		p.api.ForceCloseBuffer()
		return 0
	}))

	// tide.subscribe(event_name, callback)
	p.L.SetField(tideTable, "subscribe", p.L.NewFunction(func(L *lua.LState) int {
		eventName := L.CheckString(1)
		callback := L.CheckFunction(2)

		var eventType event.Type
		switch eventName {
		case "buffer_modified":
			eventType = event.TypeBufferModified
		case "buffer_loaded":
			eventType = event.TypeBufferLoaded
		case "buffer_saved":
			eventType = event.TypeBufferSaved
		case "cursor_moved":
			eventType = event.TypeCursorMoved
		case "app_ready":
			eventType = event.TypeAppReady
		case "app_quit":
			eventType = event.TypeAppQuit
		default:
			L.Push(lua.LString(fmt.Sprintf("unknown event type: %s", eventName)))
			return 1
		}

		p.api.SubscribeEvent(eventType, func(e event.Event) bool {
			// Run asynchronously to avoid deadlocks if an event is triggered
			// from within a Lua API call (which already holds the lock).
			go func() {
				p.mu.Lock()
				defer p.mu.Unlock()

				p.L.Push(callback)

				if err := p.L.PCall(0, 0, nil); err != nil {
					logger.Errorf("Lua event '%s' callback error: %v", eventName, err)
				}
			}()
			return false
		})

		L.Push(lua.LNil)
		return 1
	}))

	// Register it globally
	p.L.SetGlobal("tide", tideTable)
}
